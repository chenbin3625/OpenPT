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
	torrentsDir string
	archiveDir  string
	log         *slog.Logger
	mu          sync.RWMutex
	byPath      map[string]*torrent.Torrent
	events      chan Event
}

func New(torrentsDir, archiveDir string, log *slog.Logger) *Store {
	return &Store{
		torrentsDir: torrentsDir,
		archiveDir:  archiveDir,
		log:         log,
		byPath:      map[string]*torrent.Torrent{},
		events:      make(chan Event, 64),
	}
}

func (s *Store) Events() <-chan Event { return s.events }

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
	if err := torrent.EnsureDirs(s.torrentsDir, s.archiveDir); err != nil {
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
	return nil
}

func (s *Store) scan() error {
	entries, err := os.ReadDir(s.torrentsDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !torrent.IsTorrentPath(e.Name()) {
			continue
		}
		s.loadFile(filepath.Join(s.torrentsDir, e.Name()))
	}
	return nil
}

func (s *Store) loadFile(path string) {
	t, err := torrent.Load(path)
	if err != nil {
		s.log.Warn("invalid torrent archived", "path", path, "reason", err)
		s.archive(path, err.Error())
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
	s.events <- Event{Type: Added, Torrent: t}
}

func (s *Store) removeFile(path string) {
	s.mu.Lock()
	t := s.byPath[path]
	delete(s.byPath, path)
	s.mu.Unlock()
	if t != nil {
		s.log.Info("torrent removed", "path", path, "info_hash", t.InfoHashHex())
		s.events <- Event{Type: Removed, Torrent: t}
	}
}

func (s *Store) ArchiveByHash(infoHash [20]byte, reason string) {
	s.mu.RLock()
	var path string
	for p, t := range s.byPath {
		if t.InfoHash == infoHash {
			path = p
			break
		}
	}
	s.mu.RUnlock()
	if path != "" {
		s.archive(path, reason)
	}
}

func (s *Store) archive(path, reason string) {
	s.removeFile(path)
	target := filepath.Join(s.archiveDir, filepath.Base(path))
	if err := os.Rename(path, target); err != nil {
		s.log.Warn("failed to archive torrent", "path", path, "target", target, "reason", reason, "error", err)
		return
	}
	s.log.Info("torrent archived", "path", path, "target", target, "reason", reason)
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
