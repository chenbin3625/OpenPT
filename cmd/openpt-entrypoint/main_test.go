package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeIDsDefaultToUIDAndGID1000(t *testing.T) {
	t.Setenv("OPENPT_UID", "")
	t.Setenv("OPENPT_GID", "")
	t.Setenv("PUID", "")
	t.Setenv("PGID", "")

	uid, gid, err := runtimeIDs()
	if err != nil {
		t.Fatal(err)
	}
	if uid != 1000 || gid != 1000 {
		t.Fatalf("runtimeIDs = %d:%d, want 1000:1000", uid, gid)
	}
}

func TestRuntimeIDsUsePUIDAndPGID(t *testing.T) {
	t.Setenv("OPENPT_UID", "")
	t.Setenv("OPENPT_GID", "")
	t.Setenv("PUID", "501")
	t.Setenv("PGID", "20")

	uid, gid, err := runtimeIDs()
	if err != nil {
		t.Fatal(err)
	}
	if uid != 501 || gid != 20 {
		t.Fatalf("runtimeIDs = %d:%d, want 501:20", uid, gid)
	}
}

func TestRuntimeIDsPreferOpenPTUIDAndGID(t *testing.T) {
	t.Setenv("OPENPT_UID", "1000")
	t.Setenv("OPENPT_GID", "1001")
	t.Setenv("PUID", "501")
	t.Setenv("PGID", "20")

	uid, gid, err := runtimeIDs()
	if err != nil {
		t.Fatal(err)
	}
	if uid != 1000 || gid != 1001 {
		t.Fatalf("runtimeIDs = %d:%d, want 1000:1001", uid, gid)
	}
}

func TestRuntimeIDsRejectInvalidValues(t *testing.T) {
	t.Setenv("OPENPT_UID", "")
	t.Setenv("OPENPT_GID", "")
	t.Setenv("PUID", "abc")

	_, _, err := runtimeIDs()
	if err == nil || !strings.Contains(err.Error(), "PUID") {
		t.Fatalf("runtimeIDs error = %v, want PUID validation error", err)
	}
}

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

	createdPaths, err := initDataDir(dataDir, appDir)
	if err != nil {
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

	created := pathSet(createdPaths)
	for _, path := range []string{
		dataDir,
		filepath.Join(dataDir, "clients"),
		filepath.Join(dataDir, "clients", "qb.client"),
	} {
		if created[path] {
			t.Fatalf("existing path %s was reported as newly created", path)
		}
	}
	for _, path := range []string{
		filepath.Join(dataDir, "torrents"),
		filepath.Join(dataDir, "torrents_archive"),
		filepath.Join(dataDir, "config.toml"),
	} {
		if !created[path] {
			t.Fatalf("created path %s was not reported", path)
		}
	}
}

func TestInitDataDirReportsCopiedClientAsCreated(t *testing.T) {
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

	createdPaths, err := initDataDir(dataDir, appDir)
	if err != nil {
		t.Fatal(err)
	}

	created := pathSet(createdPaths)
	clientPath := filepath.Join(dataDir, "clients", "qb.client")
	if !created[clientPath] {
		t.Fatalf("copied client %s was not reported as newly created", clientPath)
	}
}

func pathSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[path] = true
	}
	return set
}
