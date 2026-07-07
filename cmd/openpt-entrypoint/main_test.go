package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitDataDirCopiesDefaultsWithoutOverwritingClients(t *testing.T) {
	appDir := t.TempDir()
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(appDir, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(appDir, "clients"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "examples", "config.docker.toml"), []byte("client = \"qb.client\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "clients", "qb.client"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "clients"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "clients", "qb.client"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := initDataDir(dataDir, appDir); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"torrents", "clients", "torrents_archive"} {
		if info, err := os.Stat(filepath.Join(dataDir, dir)); err != nil || !info.IsDir() {
			t.Fatalf("%s was not created as a directory", dir)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dataDir, "config.toml")); err != nil || string(got) != "client = \"qb.client\"\n" {
		t.Fatalf("config.toml = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dataDir, "clients", "qb.client")); err != nil || string(got) != "existing" {
		t.Fatalf("existing client was overwritten: %q, %v", got, err)
	}
}
