package torrent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent/metainfo"
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
	hash := sha1.Sum(mi.InfoBytes)
	trackers := trackerList(mi)
	if len(trackers) == 0 {
		return nil, fmt.Errorf("no http/https tracker in torrent")
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

func EnsureDirs(torrentsDir, archiveDir string) error {
	if err := os.MkdirAll(torrentsDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(archiveDir, 0o755)
}

func IsTorrentPath(path string) bool {
	return filepath.Ext(path) == ".torrent"
}

func trackerList(mi *metainfo.MetaInfo) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if (len(s) >= 7 && s[:7] == "http://") || (len(s) >= 8 && s[:8] == "https://") {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
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
