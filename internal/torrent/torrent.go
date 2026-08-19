package torrent

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
	infohashv2 "github.com/anacrolix/torrent/types/infohash-v2"
)

type Torrent struct {
	Path         string
	Name         string
	Size         int64
	Announce     string
	AnnounceList []string
	InfoHash     [20]byte
}

func Load(path string) (*Torrent, error) {
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, err
	}
	var hash [20]byte
	switch {
	case info.HasV1():
		// v1 / hybrid：tracker 一律以 20 字节 SHA-1 infohash 为准。
		hash = metainfo.HashBytes(mi.InfoBytes)
	case info.HasV2():
		// BEP-52：v2-only 种子的 20 字节 trackable infohash 是 info 字典
		// “原始 bencode 字节”的 SHA-256 截断前 20 字节。
		// 直接复用 anacrolix 参考实现的 infohash_v2.HashBytes，
		// 避免两处算法漂移（padding 处理等细节与参考实现保持一致）。
		v2Hash := infohashv2.HashBytes(mi.InfoBytes)
		copy(hash[:], v2Hash[:20])
	default:
		return nil, fmt.Errorf("torrent info dictionary is neither v1 nor v2")
	}
	trackers := trackerList(mi)
	if len(trackers) == 0 {
		return nil, fmt.Errorf("no http/https/udp tracker in torrent")
	}
	return &Torrent{
		Path:         path,
		Name:         info.Name,
		Size:         info.TotalLength(),
		Announce:     mi.Announce,
		AnnounceList: trackers,
		InfoHash:     hash,
	}, nil
}

func (t Torrent) InfoHashBytes() []byte {
	out := make([]byte, len(t.InfoHash))
	copy(out, t.InfoHash[:])
	return out
}

func (t Torrent) InfoHashHex() string {
	return hex.EncodeToString(t.InfoHash[:])
}

func EnsureDirs(torrentsDir string) error {
	if err := os.MkdirAll(torrentsDir, 0o755); err != nil {
		return err
	}
	return nil
}

func IsTorrentPath(path string) bool {
	return filepath.Ext(path) == ".torrent"
}

func trackerList(mi *metainfo.MetaInfo) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return
		}
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "udp":
		default:
			return
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(mi.Announce)
	for _, tier := range mi.AnnounceList {
		for _, tr := range tier {
			add(tr)
		}
	}
	return out
}
