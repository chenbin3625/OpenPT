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
					time.Sleep(watcherSettleDelay)
					s.loadFile(ev.Name)
				}
				if torrent.IsTorrentPath(ev.Name) && (ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename)) {
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
	t, err := torrent.Load(path)
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
	t, err := torrent.Load(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Docker bind-mount 下文件可能仍在写入，短暂等待后重试一次，
		// 避免把 partial write 的种子误判为损坏而归档。
		time.Sleep(loadRetryDelay)
		t, err = torrent.Load(path)
	}
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

// archiveFile 将加载失败的种子移动到归档目录，返回归档后的路径。
// 同名文件已存在时自动追加数字后缀避免覆盖。
func (s *Store) archiveFile(path string) (string, bool) {
	if err := os.MkdirAll(s.archiveDir, 0o755); err != nil {
		s.log.Warn("failed to create archive dir", "dir", s.archiveDir, "error", err)
		return "", false
	}
	dest := uniquePath(s.archiveDir, filepath.Base(path))
	if err := moveFile(path, dest); err != nil {
		s.log.Warn("failed to archive torrent", "path", path, "archive", dest, "error", err)
		return "", false
	}
	return dest, true
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

// moveFile 跨文件系统安全地移动文件：先尝试 rename，失败（如跨设备）则回退到 copy+remove。
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if copyErr := copyFile(src, dst); copyErr != nil {
		// 回退也失败时返回 rename 的原始错误
		return err
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
	out, err := os.Create(dst)
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
		// channel 满，在 goroutine 中异步发送，并监听 context 避免泄漏
		go func() {
			select {
			case s.events <- ev:
			case <-s.ctx.Done():
				s.log.Debug("context cancelled, dropping torrent event", "path", path)
				return
			}
		}()
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
