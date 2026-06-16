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

func TestDefaultValues(t *testing.T) {
	cfg := loadConfigTOML(t, `client = "qbittorrent.client"`)

	// 未设置时默认为 0，不会自动设为 1
	if cfg.SimultaneousSeed != 0 {
		t.Fatalf("default simultaneous_seed = %d, want 0", cfg.SimultaneousSeed)
	}
	if cfg.Announce.Port != 6881 {
		t.Fatalf("default announce.port = %d, want 6881", cfg.Announce.Port)
	}
	if cfg.Uploaded.Strategy != "none" {
		t.Fatalf("default uploaded.strategy = %q, want none", cfg.Uploaded.Strategy)
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

