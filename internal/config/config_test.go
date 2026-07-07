package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBasicTOML(t *testing.T) {
	cfg := loadConfigTOML(t, `
client = "qbittorrent.client"
simultaneous_seed = 100

[uploaded]
strategy = "configured_rate"
configured_rate_bps = 170000
ratio_target = 0
`)
	if cfg.Client != "qbittorrent.client" {
		t.Fatalf("client = %q, want qbittorrent.client", cfg.Client)
	}
	if cfg.SimultaneousSeed != 100 {
		t.Fatalf("simultaneous_seed = %d, want 100", cfg.SimultaneousSeed)
	}
	if cfg.Uploaded.Strategy != "configured_rate" {
		t.Fatalf("strategy = %q, want configured_rate", cfg.Uploaded.Strategy)
	}
	if cfg.Uploaded.ConfiguredRateBps != 170000 {
		t.Fatalf("configured_rate_bps = %d, want 170000", cfg.Uploaded.ConfiguredRateBps)
	}
}

func TestLoadRatioTarget(t *testing.T) {
	cfg := loadConfigTOML(t, `
client = "qbittorrent.client"

[uploaded]
ratio_target = 1.5
`)
	if cfg.Uploaded.RatioTarget != 1.5 {
		t.Fatalf("ratio target = %v, want 1.5", cfg.Uploaded.RatioTarget)
	}
}

func TestInvalidRatioTargetFails(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[uploaded]
ratio_target = -2
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid ratio_target to fail")
	}
}

func TestUnknownFieldFails(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[uploaded]
strategy = "configured_rate"
configured_rate_bpss = 170000
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected unknown TOML field to fail")
	}
}

func TestConfiguredRateMustBePositive(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[uploaded]
strategy = "configured_rate"
configured_rate_bps = 0
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected configured_rate with zero rate to fail")
	}
}

func TestConservativeRateMustBePositiveWhenSelected(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[uploaded]
strategy = "conservative_rate"
conservative_rate_bps = -1
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected conservative_rate with negative rate to fail")
	}
}

func TestInvalidScanIntervalFails(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"
scan_interval_seconds = -1
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid scan_interval_seconds to fail")
	}
}

func TestNegativeSimultaneousSeedFails(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"
simultaneous_seed = -1
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected negative simultaneous_seed to fail")
	}
}

func TestArchiveDirInsideTorrentsDirFails(t *testing.T) {
	for _, archiveDir := range []string{"./torrents/archive", "./torrents/..bad_archive"} {
		path := writeConfigTOML(t, `
client = "qbittorrent.client"
torrents_dir = "./torrents"
archive_dir = "`+archiveDir+`"
`)
		if _, err := Load(path); err == nil {
			t.Fatalf("expected archive_dir %q inside torrents_dir to fail", archiveDir)
		}
	}
}

func TestInvalidProxyFails(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[tracker]
proxy = "ftp://127.0.0.1:7890"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected unsupported tracker.proxy scheme to fail")
	}
}

func TestInvalidMetricsPathFailsWhenMetricsEnabled(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[metrics]
enabled = true
path = "metrics"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected metrics.path without leading slash to fail")
	}
}

func TestInvalidMetricsListenFailsWhenMetricsEnabled(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"

[metrics]
enabled = true
listen = "not-a-listen-address"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid metrics.listen to fail")
	}
}

func TestMetricsPathCannotConflictWithWebUIRoutes(t *testing.T) {
	for _, metricsPath := range []string{"/", "/api/status", "/api/config", "/api/events"} {
		path := writeConfigTOML(t, `
client = "qbittorrent.client"

[metrics]
enabled = true
webui = true
path = "`+metricsPath+`"
`)
		if _, err := Load(path); err == nil {
			t.Fatalf("expected metrics.path %q to conflict with web UI routes", metricsPath)
		}
	}
}

func TestDefaultValues(t *testing.T) {
	cfg := loadConfigTOML(t, `client = "qbittorrent.client"`)

	// 未设置时默认为 0，不会自动设为 1
	if cfg.SimultaneousSeed != 0 {
		t.Fatalf("default simultaneous_seed = %d, want 0", cfg.SimultaneousSeed)
	}
	if cfg.Announce.Port < randomAnnouncePortMin || cfg.Announce.Port > randomAnnouncePortMax {
		t.Fatalf("default announce.port = %d, want %d..%d", cfg.Announce.Port, randomAnnouncePortMin, randomAnnouncePortMax)
	}
	if cfg.Uploaded.Strategy != "none" {
		t.Fatalf("default uploaded.strategy = %q, want none", cfg.Uploaded.Strategy)
	}
}

func TestRelativePathsResolveFromConfigDir(t *testing.T) {
	path := writeConfigTOML(t, `
client = "qbittorrent.client"
torrents_dir = "./torrents"
archive_dir = "../archive"
clients_dir = "./clients"
state_file = "./state.json"

[logging]
file = "./openpt.log"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(path)
	if cfg.TorrentsDir != filepath.Join(root, "torrents") {
		t.Fatalf("torrents_dir = %q, want config-relative path", cfg.TorrentsDir)
	}
	if cfg.ArchiveDir != filepath.Clean(filepath.Join(root, "../archive")) {
		t.Fatalf("archive_dir = %q, want config-relative path", cfg.ArchiveDir)
	}
	if cfg.ClientsDir != filepath.Join(root, "clients") {
		t.Fatalf("clients_dir = %q, want config-relative path", cfg.ClientsDir)
	}
	if cfg.StateFile != filepath.Join(root, "state.json") {
		t.Fatalf("state_file = %q, want config-relative path", cfg.StateFile)
	}
	if cfg.Logging.File != filepath.Join(root, "openpt.log") {
		t.Fatalf("logging.file = %q, want config-relative path", cfg.Logging.File)
	}
}

func TestConfiguredAnnouncePortIsPreserved(t *testing.T) {
	cfg := loadConfigTOML(t, `
client = "qbittorrent.client"

[announce]
port = 51413
`)
	if cfg.Announce.Port != 51413 {
		t.Fatalf("announce.port = %d, want 51413", cfg.Announce.Port)
	}
}

func TestJSONFormatDeprecated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"client":"qbittorrent.client"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected JSON format to be rejected")
	}
}

func loadConfigTOML(t *testing.T, data string) Config {
	t.Helper()
	cfg, err := Load(writeConfigTOML(t, data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeConfigTOML(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
