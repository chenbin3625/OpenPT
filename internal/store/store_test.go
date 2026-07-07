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

func TestScanAndNotifyDetectsReplacedTorrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := NewWithScanInterval(ctx, dir, "", 0, discardLogger())

	path := filepath.Join(dir, "test.torrent")
	writeTestTorrent(t, path, "http://tracker.example/announce", "old.bin", 100)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}
	added := receiveEvent(t, s)
	if added.Type != Added || added.Torrent.Name != "old.bin" {
		t.Fatalf("event after add = %+v, want Added old.bin", added)
	}

	writeTestTorrent(t, path, "http://tracker.example/announce", "new.bin", 200)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}
	removed := receiveEvent(t, s)
	if removed.Type != Removed || removed.Torrent.Name != "old.bin" {
		t.Fatalf("event after replace = %+v, want Removed old.bin", removed)
	}
	added = receiveEvent(t, s)
	if added.Type != Added || added.Torrent.Name != "new.bin" {
		t.Fatalf("event after replace = %+v, want Added new.bin", added)
	}
}

func TestInvalidReplacementKeepsOldTorrentAndDoesNotDeleteFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := NewWithScanInterval(ctx, dir, "", 0, discardLogger())

	path := filepath.Join(dir, "test.torrent")
	writeTestTorrent(t, path, "http://tracker.example/announce", "old.bin", 100)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}
	_ = receiveEvent(t, s)

	// 用无效内容覆盖（模拟 Docker bind-mount 下的 partial write / 损坏文件）
	if err := os.WriteFile(path, []byte("not bencode"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}

	// 不应产生 Removed 事件：旧种子保持加载，等待下次扫描重试（避免误删/误停）
	select {
	case ev := <-s.Events():
		t.Fatalf("unexpected event after invalid replace = %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}

	// 文件不应被删除
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("invalid torrent file was deleted: %v", err)
	}

	// 旧种子仍在 store 中
	if got := s.Status(); len(got) != 1 || got[0].Name != "old.bin" {
		t.Fatalf("store status after invalid replace = %+v, want [old.bin]", got)
	}
}

func TestScanDoesNotDeletePartialTorrentFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := NewWithScanInterval(ctx, dir, "", 0, discardLogger())

	// 模拟正在写入的半成品种子文件
	path := filepath.Join(dir, "partial.torrent")
	if err := os.WriteFile(path, []byte("d8:announce"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}

	// partial read 不应导致文件被删除
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("partial torrent file was deleted: %v", err)
	}
	// 不应产生 Added 事件
	select {
	case ev := <-s.Events():
		t.Fatalf("unexpected event for partial torrent = %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestFailedTorrentArchived(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, "archive")
	s := NewWithScanInterval(ctx, dir, archiveDir, 0, discardLogger())

	path := filepath.Join(dir, "broken.torrent")
	if err := os.WriteFile(path, []byte("not bencode"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, path)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}

	// 文件应被移动到归档目录，原位置不再存在
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected broken torrent moved out of torrents dir, got err=%v", err)
	}
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("archive dir not created: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "broken.torrent" {
		t.Fatalf("archive entries = %+v, want [broken.torrent]", entries)
	}
	// 不应产生 Added 事件
	select {
	case ev := <-s.Events():
		t.Fatalf("unexpected event for broken torrent = %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestInvalidReplacementArchivedAndOldRemoved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, "archive")
	s := NewWithScanInterval(ctx, dir, archiveDir, 0, discardLogger())

	path := filepath.Join(dir, "test.torrent")
	writeTestTorrent(t, path, "http://tracker.example/announce", "old.bin", 100)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}
	_ = receiveEvent(t, s) // Added old.bin

	if err := os.WriteFile(path, []byte("not bencode"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, path)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}

	// 旧种子应被移除并发出 Removed 事件
	removed := receiveEvent(t, s)
	if removed.Type != Removed || removed.Torrent.Name != "old.bin" {
		t.Fatalf("event after invalid replace = %+v, want Removed old.bin", removed)
	}
	// 无效文件应被归档
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected invalid torrent archived (moved out), got err=%v", err)
	}
	if got := s.Status(); len(got) != 0 {
		t.Fatalf("store status after invalid replace = %+v, want empty", got)
	}
}

func TestArchiveNameCollisionDoesNotOverwrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 归档目录已有同名文件
	if err := os.WriteFile(filepath.Join(archiveDir, "broken.torrent"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewWithScanInterval(ctx, dir, archiveDir, 0, discardLogger())

	path := filepath.Join(dir, "broken.torrent")
	if err := os.WriteFile(path, []byte("not bencode"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, path)
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}

	// 新归档文件应使用带后缀的名字，原归档文件保留
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["broken.torrent"] || !names["broken.1.torrent"] {
		t.Fatalf("archive entries = %+v, want both broken.torrent and broken.1.torrent", names)
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

func TestLoadFileQuietRetriesBeforeArchiving(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, "archive")
	s := NewWithScanInterval(ctx, dir, archiveDir, 0, discardLogger())

	path := filepath.Join(dir, "retry.torrent")
	if err := os.WriteFile(path, []byte("not complete yet"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		s.loadFileQuiet(path)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	writeTestTorrent(t, path, "http://tracker.example/announce", "retry.bin", 100)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loadFileQuiet did not return")
	}
	if got := s.Status(); len(got) != 1 || got[0].Name != "retry.bin" {
		t.Fatalf("store status after retry = %+v, want retry.bin", got)
	}
	if entries, err := os.ReadDir(archiveDir); err == nil && len(entries) != 0 {
		t.Fatalf("archive entries = %+v, want empty", entries)
	}
}

func TestScheduleLoadDebouncesSamePath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := NewWithScanInterval(ctx, dir, "", 0, discardLogger())

	path := filepath.Join(dir, "debounce.torrent")
	writeTestTorrent(t, path, "http://tracker.example/announce", "old.bin", 100)
	s.scheduleLoad(ctx, path)
	writeTestTorrent(t, path, "http://tracker.example/announce", "new.bin", 200)
	s.scheduleLoad(ctx, path)

	ev := receiveEvent(t, s)
	if ev.Type != Added || ev.Torrent.Name != "new.bin" {
		t.Fatalf("debounced event = %+v, want Added new.bin", ev)
	}
	select {
	case ev := <-s.Events():
		t.Fatalf("unexpected extra event after debounce = %+v", ev)
	case <-time.After(watcherSettleDelay + 100*time.Millisecond):
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestMoveFileRollsBackOnRemoveFailure 验证：当 Rename 失败、copyFile 成功、
// 但删除 src 失败时（如源目录只读），dst 会被回滚，避免文件重复。
// 在 src 所在目录设为只读时：Rename 需要写源目录 → 失败；
// Remove(src) 需要写源目录 → 失败；copyFile 打开 src 读 + 在可写 dst 目录创建 → 成功。
func TestMoveFileRollsBackOnRemoveFailure(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	src := filepath.Join(srcDir, "src.torrent")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 源目录设为只读：禁止 unlink/rename，但允许读取已有文件
	if err := os.Chmod(srcDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(srcDir, 0o755) // 恢复可写，让 tempdir 清理正常
	})

	dst := filepath.Join(dstDir, "dst.torrent")
	err := moveFile(src, dst)
	if err == nil {
		t.Fatal("expected moveFile to fail when src directory is read-only")
	}

	// src 应仍存在（Remove 失败）
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("src should still exist after rollback, got err=%v", err)
	}
	// dst 应已被回滚清理（避免重复）
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("dst should be rolled back (removed), got err=%v", err)
	}
}

func TestRecentInvalidTorrentIsNotArchived(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, "archive")
	s := NewWithScanInterval(ctx, dir, archiveDir, 0, discardLogger())

	path := filepath.Join(dir, "partial.torrent")
	if err := os.WriteFile(path, []byte("not bencode yet"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.scanAndNotify(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recent invalid torrent should stay in place, got err=%v", err)
	}
	entries, err := os.ReadDir(archiveDir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("archive entries = %+v, want empty", entries)
	}
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

func ageFile(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-(archiveGracePeriod + time.Second))
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}
