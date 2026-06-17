package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"openpt/internal/torrent"
)

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
		scanInterval: scanInterval,
		log:          log,
		byPath:       map[string]*torrent.Torrent{},
		events:       make(chan Event, 256), // 增加缓冲区避免阻塞
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
	if err := torrent.EnsureDirs(s.torrentsDir, ""); err != nil {
		return err
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
					time.Sleep(300 * time.Millisecond)
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
	if notify {
		var missing []string
		s.mu.RLock()
		for path := range s.byPath {
			if !seen[path] {
				missing = append(missing, path)
			}
		}
		s.mu.RUnlock()
		for _, path := range missing {
			s.removeFile(path)
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
		s.log.Warn("invalid torrent, removing", "path", path, "reason", err)
		if err := os.Remove(path); err != nil {
			s.log.Warn("failed to remove invalid torrent", "path", path, "error", err)
		}
		return
	}
	s.mu.Lock()
	s.byPath[path] = t
	s.mu.Unlock()
	s.log.Info("torrent loaded", "path", path, "name", t.Name, "size", t.Size, "info_hash", t.InfoHashHex())
}

func (s *Store) loadFile(path string) {
	t, err := torrent.Load(path)
	if err != nil {
		s.log.Warn("invalid torrent, removing", "path", path, "reason", err)
		if err := os.Remove(path); err != nil {
			s.log.Warn("failed to remove invalid torrent", "path", path, "error", err)
		}
		return
	}
	s.mu.Lock()
	old := s.byPath[path]
	s.byPath[path] = t
	s.mu.Unlock()
	if old != nil && old.InfoHash == t.InfoHash {
		return
	}
	s.log.Info("torrent loaded", "path", path, "name", t.Name, "size", t.Size, "info_hash", t.InfoHashHex())
	s.emit(Event{Type: Added, Torrent: t}, path)
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
