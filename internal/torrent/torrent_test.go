package torrent

import (
	"crypto/sha1"
	"os"
	"path/filepath"
	"testing"
)

func TestTorrentInfoHashUsesRawInfoDictionary(t *testing.T) {
	info := []byte("d6:lengthi123e4:name8:file.bin12:piece lengthi16384e6:pieces20:abcdefghijklmnopqrste")
	raw := append([]byte("d8:announce28:http://tracker.test/announce4:info"), info...)
	raw = append(raw, 'e')
	path := filepath.Join(t.TempDir(), "sample.torrent")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := sha1.Sum(info)
	if got.InfoHash != want {
		t.Fatalf("info hash = %x, want %x", got.InfoHash, want)
	}
	if got.Name != "file.bin" || got.Size != 123 {
		t.Fatalf("unexpected metadata: name=%q size=%d", got.Name, got.Size)
	}
	if len(got.AnnounceList) != 1 || got.AnnounceList[0] != "http://tracker.test/announce" {
		t.Fatalf("announce list = %#v", got.AnnounceList)
	}
}
