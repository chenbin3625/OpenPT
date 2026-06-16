package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLegacyUploadRatioTargetDisabled(t *testing.T) {
	cfg := loadConfigJSON(t, `{"client":"qbittorrent.client","uploadRatioTarget":-1}`)
	if cfg.Uploaded.RatioTarget != 0 {
		t.Fatalf("ratio target = %v, want disabled 0", cfg.Uploaded.RatioTarget)
	}
}

func TestLoadLegacyUploadRatioTarget(t *testing.T) {
	cfg := loadConfigJSON(t, `{"client":"qbittorrent.client","uploadRatioTarget":1.5}`)
	if cfg.Uploaded.RatioTarget != 1.5 {
		t.Fatalf("ratio target = %v, want 1.5", cfg.Uploaded.RatioTarget)
	}
}

func TestModernRatioTargetOverridesLegacy(t *testing.T) {
	cfg := loadConfigJSON(t, `{"client":"qbittorrent.client","uploadRatioTarget":1.5,"uploaded":{"ratio_target":0}}`)
	if cfg.Uploaded.RatioTarget != 0 {
		t.Fatalf("ratio target = %v, want modern value 0", cfg.Uploaded.RatioTarget)
	}
}

func TestInvalidLegacyUploadRatioTargetFails(t *testing.T) {
	path := writeConfigJSON(t, `{"client":"qbittorrent.client","uploadRatioTarget":-2}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid legacy uploadRatioTarget to fail")
	}
}

func TestLegacyMinUploadRateOnlyMigration(t *testing.T) {
	// 验证旧配置仅设 minUploadRate 未设 maxUploadRate 时，ConfiguredRateBps 不为 0
	cfg := loadConfigJSON(t, `{"client":"qbittorrent.client","minUploadRate":500}`)
	if cfg.Uploaded.Strategy != "configured_rate" {
		t.Fatalf("strategy = %q, want configured_rate", cfg.Uploaded.Strategy)
	}
	if cfg.Uploaded.ConfiguredRateBps != 500*1000 {
		t.Fatalf("ConfiguredRateBps = %d, want %d", cfg.Uploaded.ConfiguredRateBps, 500*1000)
	}
	if cfg.Uploaded.MinRateBps != 500*1000 {
		t.Fatalf("MinRateBps = %d, want %d", cfg.Uploaded.MinRateBps, 500*1000)
	}
}

func loadConfigJSON(t *testing.T, data string) Config {
	t.Helper()
	cfg, err := Load(writeConfigJSON(t, data))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeConfigJSON(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
