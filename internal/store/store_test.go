package store

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanAndNotifyDetectsAddedAndRemovedTorrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := NewWithScanInterval(ctx, dir, "", 0, discardLogger())

	path := filepath.Join(dir, "test.torrent")
	writeTestTorrent(t, path, "http://tracker.example/announce", "file.bin", 100)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}
	ev := receiveEvent(t, s)
	if ev.Type != Added || ev.Torrent.Name != "file.bin" {
		t.Fatalf("event after add = %+v, want Added file.bin", ev)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}
	ev = receiveEvent(t, s)
	if ev.Type != Removed || ev.Torrent.Name != "file.bin" {
		t.Fatalf("event after remove = %+v, want Removed file.bin", ev)
	}
}

func TestPeriodicScanUsesConfiguredInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := NewWithScanInterval(ctx, dir, "", 20*time.Millisecond, discardLogger())
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	writeTestTorrent(t, filepath.Join(dir, "periodic.torrent"), "http://tracker.example/announce", "periodic.bin", 100)
	ev := receiveEventBefore(t, s, 250*time.Millisecond)
	if ev.Type != Added || ev.Torrent.Name != "periodic.bin" {
		t.Fatalf("event after periodic scan = %+v, want Added periodic.bin", ev)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func receiveEvent(t *testing.T, s *Store) Event {
	t.Helper()
	return receiveEventBefore(t, s, time.Second)
}

func receiveEventBefore(t *testing.T, s *Store, timeout time.Duration) Event {
	t.Helper()
	select {
	case ev := <-s.Events():
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for store event")
		return Event{}
	}
}

func writeTestTorrent(t *testing.T, path, announce, name string, size int64) {
	t.Helper()
	info := fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces20:abcdefghijklmnopqrste", size, len(name), name)
	raw := fmt.Sprintf("d8:announce%d:%s4:info%se", len(announce), announce, info)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}
