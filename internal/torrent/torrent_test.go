package torrent

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	infohashv2 "github.com/anacrolix/torrent/types/infohash-v2"
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

func TestTorrentV2UsesTruncatedSHA256InfoHash(t *testing.T) {
	root := "01234567890123456789012345678901"
	info := []byte("d9:file treed8:file.bind0:d6:lengthi123e11:pieces root32:" + root + "eee12:meta versioni2e4:name8:file.bin12:piece lengthi16384ee")
	announce := "udp://tracker.test:80/path"
	raw := append([]byte(fmt.Sprintf("d8:announce%d:%s4:info", len(announce), announce)), info...)
	raw = append(raw, 'e')
	path := filepath.Join(t.TempDir(), "v2.torrent")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	// 与 anacrolix 参考实现（原始 info 字典字节的 SHA-256 截断 20 字节）保持一致，
	// 避免自行推导的算法与生态实现漂移。
	v2Full := infohashv2.HashBytes(info)
	var want [20]byte
	copy(want[:], v2Full[:20])
	if got.InfoHash != want {
		t.Fatalf("info hash = %x, want %x", got.InfoHash, want)
	}
	if len(got.AnnounceList) != 1 || got.AnnounceList[0] != "udp://tracker.test:80/path" {
		t.Fatalf("announce list = %#v", got.AnnounceList)
	}
}

// TestTorrentHybridAnnounceUsesV1Hash 验证 hybrid（v1+v2）种子在 Load 时使用 v1
// infohash：tracker 上报以 v1 为准，v2 字段不影响调度器使用的稳定标识。
func TestTorrentHybridAnnounceUsesV1Hash(t *testing.T) {
	info := []byte("d6:lengthi123e4:name8:file.bin12:piece lengthi16384e6:pieces20:abcdefghijklmnopqrst" + "12:meta versioni2e" + "e")
	announce := "http://tracker.test/announce"
	raw := append([]byte(fmt.Sprintf("d8:announce%d:%s4:info", len(announce), announce)), info...)
	raw = append(raw, 'e')
	path := filepath.Join(t.TempDir(), "hybrid.torrent")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := metainfo.HashBytes(info)
	if got.InfoHash != want {
		t.Fatalf("hybrid info hash = %x, want v1 hash %x", got.InfoHash, want)
	}
}
