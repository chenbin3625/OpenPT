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
