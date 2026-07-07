package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"openpt/internal/torrent"
)

// loadRetryDelay 是加载失败后重试前的等待时间，用于规避 Docker bind-mount 下
// 文件仍在写入（partial read）导致的瞬时解析失败。
const loadRetryDelay = 500 * time.Millisecond

// watcherSettleDelay 是 fsnotify 触发 Create/Write 后、读取文件前的等待，
// 让 Docker bind-mount 等场景下仍在写入的文件落盘，避免 partial read。
const watcherSettleDelay = 300 * time.Millisecond

// archiveGracePeriod 是解析失败后归档前要求文件保持未修改的最短时间。
// 直接向 torrents_dir 慢速写入时，fsnotify 可能在文件完整落盘前触发；
// 对近期仍在变化的文件先保留并等待后续扫描，避免误归档有效种子。
const archiveGracePeriod = 2 * time.Second

// eventBufferSize 是 store 事件 channel 的缓冲容量。
// 调度器持续消费事件，缓冲主要吸收扫描/批量删除时的瞬时尖峰。
const eventBufferSize = 256

type EventType int

const (
	Added EventType = iota
	Removed
)

type Event struct {
	Type    EventType
	Torrent *torrent.Torrent
}

type Store struct {
	ctx          context.Context
	torrentsDir  string
	archiveDir   string // 加载失败的种子归档目录；为空则不归档（仅跳过）
	scanInterval time.Duration
	log          *slog.Logger
	mu           sync.RWMutex
	byPath       map[string]*torrent.Torrent
	events       chan Event
	debounceMu   sync.Mutex
	pendingLoads map[string]uint64
	nextLoadID   uint64
}

func New(ctx context.Context, torrentsDir, archiveDir string, log *slog.Logger) *Store {
	return NewWithScanInterval(ctx, torrentsDir, archiveDir, 5*time.Second, log)
}

func NewWithScanInterval(ctx context.Context, torrentsDir, archiveDir string, scanInterval time.Duration, log *slog.Logger) *Store {
	return &Store{
		ctx:          ctx,
		torrentsDir:  torrentsDir,
		archiveDir:   archiveDir,
		scanInterval: scanInterval,
		log:          log,
		byPath:       map[string]*torrent.Torrent{},
		events:       make(chan Event, eventBufferSize), // 增加缓冲区避免阻塞
		pendingLoads: map[string]uint64{},
	}
}

func (s *Store) Events() <-chan Event { return s.events }

// TorrentInfo holds basic torrent information for the web UI.
type TorrentInfo struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
}

// Status returns information about all torrents in the store.
func (s *Store) Status() []TorrentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TorrentInfo, 0, len(s.byPath))
	for _, t := range s.byPath {
		out = append(out, TorrentInfo{
			InfoHash: t.InfoHashHex(),
			Name:     t.Name,
			Size:     t.Size,
		})
	}
	return out
}

func (s *Store) List() []*torrent.Torrent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*torrent.Torrent, 0, len(s.byPath))
	for _, t := range s.byPath {
		out = append(out, t)
	}
	return out
}

func (s *Store) Start(ctx context.Context) error {
	if err := torrent.EnsureDirs(s.torrentsDir); err != nil {
		return err
	}
	if s.archiveDir != "" {
		if err := os.MkdirAll(s.archiveDir, 0o755); err != nil {
			return err
		}
	}
	if err := s.scan(); err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(s.torrentsDir); err != nil {
		_ = w.Close()
		return err
	}
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-w.Events:
				if torrent.IsTorrentPath(ev.Name) && (ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write)) {
					s.scheduleLoad(ctx, ev.Name)
				}
				if torrent.IsTorrentPath(ev.Name) && (ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename)) {
					s.cancelPendingLoad(ev.Name)
					s.removeFile(ev.Name)
				}
			case err := <-w.Errors:
				if err != nil {
					s.log.Warn("torrent watcher error", "error", err)
				}
			}
		}
	}()
	if s.scanInterval > 0 {
		go s.periodicScan(ctx)
	}
	return nil
}

func (s *Store) scan() error {
	return s.scanDir(false)
}

func (s *Store) scanAndNotify() error {
	return s.scanDir(true)
}

func (s *Store) scanDir(notify bool) error {
	entries, err := os.ReadDir(s.torrentsDir)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() || !torrent.IsTorrentPath(e.Name()) {
			continue
		}
		path := filepath.Join(s.torrentsDir, e.Name())
		seen[path] = true
		if notify {
			s.loadFile(path)
		} else {
			s.loadFileQuiet(path)
		}
	}
	var missing []string
	s.mu.RLock()
	for path := range s.byPath {
		if !seen[path] {
			missing = append(missing, path)
		}
	}
	s.mu.RUnlock()
	for _, path := range missing {
		if notify {
			s.removeFile(path)
		} else {
			s.removeFileQuiet(path)
		}
	}
	return nil
}

func (s *Store) periodicScan(ctx context.Context) {
	ticker := time.NewTicker(s.scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.scanAndNotify(); err != nil {
				s.log.Warn("torrent scan error", "error", err)
			}
		}
	}
}

func (s *Store) loadFileQuiet(path string) {
	t, err := loadTorrentWithRetry(path)
	if err != nil {
		s.handleLoadFailure(path, err)
		return
	}
	s.mu.Lock()
	s.byPath[path] = t
	s.mu.Unlock()
	s.log.Info("torrent loaded", "path", path, "name", t.Name, "size", t.Size, "info_hash", t.InfoHashHex())
}

func (s *Store) loadFile(path string) {
	t, err := loadTorrentWithRetry(path)
	if err != nil {
		s.handleLoadFailure(path, err)
		return
	}
	s.mu.Lock()
	old := s.byPath[path]
	s.byPath[path] = t
	s.mu.Unlock()
	if old != nil && old.InfoHash == t.InfoHash && slices.Equal(old.AnnounceList, t.AnnounceList) {
		return
	}
	if old != nil {
		s.log.Info("torrent replaced", "path", path, "old_info_hash", old.InfoHashHex(), "new_info_hash", t.InfoHashHex())
		s.emit(Event{Type: Removed, Torrent: old}, path)
	}
	s.log.Info("torrent loaded", "path", path, "name", t.Name, "size", t.Size, "info_hash", t.InfoHashHex())
	s.emit(Event{Type: Added, Torrent: t}, path)
}

func loadTorrentWithRetry(path string) (*torrent.Torrent, error) {
	t, err := torrent.Load(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Docker bind-mount 下文件可能仍在写入，短暂等待后重试一次，
		// 避免把 partial write 的种子误判为损坏而归档。
		time.Sleep(loadRetryDelay)
		t, err = torrent.Load(path)
	}
	return t, err
}

func (s *Store) scheduleLoad(ctx context.Context, path string) {
	s.debounceMu.Lock()
	s.nextLoadID++
	id := s.nextLoadID
	s.pendingLoads[path] = id
	s.debounceMu.Unlock()

	go func() {
		timer := time.NewTimer(watcherSettleDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.debounceMu.Lock()
		if s.pendingLoads[path] != id {
			s.debounceMu.Unlock()
			return
		}
		delete(s.pendingLoads, path)
		s.debounceMu.Unlock()
		s.loadFile(path)
	}()
}

func (s *Store) cancelPendingLoad(path string) {
	s.debounceMu.Lock()
	defer s.debounceMu.Unlock()
	delete(s.pendingLoads, path)
}

// handleLoadFailure 处理种子加载失败：不删除文件。
//   - 文件不存在（已移除）：若之前已加载，移除并通知调度器停止
//     （兼容部分 bind-mount 下 fsnotify Remove 事件不触发的情况）。
//   - 未配置 archive_dir：保留旧条目与文件，等待下次扫描重试，不产生事件。
//   - 其它错误（解析/权限/partial read）：移动到归档目录便于整理问题种子；
//     归档失败则保留原文件与旧条目，等待下次扫描重试。
func (s *Store) handleLoadFailure(path string, loadErr error) {
	if errors.Is(loadErr, os.ErrNotExist) {
		if old := s.removeFileQuiet(path); old != nil {
			s.log.Info("torrent removed", "path", path, "info_hash", old.InfoHashHex())
			s.emit(Event{Type: Removed, Torrent: old}, path)
		}
		return
	}
	if s.archiveDir == "" {
		s.log.Warn("torrent load failed, skipping", "path", path, "reason", loadErr)
		return
	}
	if s.isRecentlyModified(path) {
		s.log.Warn("torrent load failed, waiting for file to settle", "path", path, "reason", loadErr)
		return
	}
	dest, ok := s.archiveFile(path)
	if !ok {
		s.log.Warn("torrent load failed, kept in place (archive failed)", "path", path, "reason", loadErr)
		return
	}
	s.log.Warn("torrent load failed, archived", "path", path, "archive", dest, "reason", loadErr)
	// 文件已移走：移除旧条目并通知调度器停止
	if old := s.removeFileQuiet(path); old != nil {
		s.log.Info("torrent removed", "path", path, "info_hash", old.InfoHashHex())
		s.emit(Event{Type: Removed, Torrent: old}, path)
	}
}

func (s *Store) isRecentlyModified(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < archiveGracePeriod
}

// archiveFile 将加载失败的种子移动到归档目录，返回归档后的路径。
// 同名文件已存在时自动追加数字后缀避免覆盖。
func (s *Store) archiveFile(path string) (string, bool) {
	if err := os.MkdirAll(s.archiveDir, 0o755); err != nil {
		s.log.Warn("failed to create archive dir", "dir", s.archiveDir, "error", err)
		return "", false
	}
	for {
		dest := uniquePath(s.archiveDir, filepath.Base(path))
		if err := moveFile(path, dest); err != nil {
			if os.IsExist(err) {
				continue
			}
			s.log.Warn("failed to archive torrent", "path", path, "archive", dest, "error", err)
			return "", false
		}
		return dest, true
	}
}

func uniquePath(dir, name string) string {
	base := filepath.Join(dir, name)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s.%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// moveFile 跨文件系统安全地移动文件，不覆盖已有目标。
func moveFile(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		if err := os.Remove(src); err != nil {
			_ = os.Remove(dst)
			return err
		}
		return nil
	} else if copyErr := copyFile(src, dst); copyErr != nil {
		return copyErr
	}
	if err := os.Remove(src); err != nil {
		// copy 已成功但 src 删除失败：回滚 dst，避免 src 与 dst 同时存在导致下次扫描重复归档
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		// 清理半成品 dst，避免归档目录残留损坏文件
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

func (s *Store) removeFile(path string) {
	s.mu.Lock()
	t := s.byPath[path]
	delete(s.byPath, path)
	s.mu.Unlock()
	if t != nil {
		s.log.Info("torrent removed", "path", path, "info_hash", t.InfoHashHex())
		s.emit(Event{Type: Removed, Torrent: t}, path)
	}
}

func (s *Store) removeFileQuiet(path string) *torrent.Torrent {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.byPath[path]
	delete(s.byPath, path)
	return t
}

func (s *Store) emit(ev Event, path string) {
	select {
	case s.events <- ev:
	default:
		s.log.Warn("torrent event queue full, dropping event", "path", path, "type", ev.Type)
	}
}

func (s *Store) PickNotIn(active map[[20]byte]bool) (*torrent.Torrent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.byPath {
		if !active[t.InfoHash] {
			return t, nil
		}
	}
	return nil, fmt.Errorf("no more torrent files available")
}
