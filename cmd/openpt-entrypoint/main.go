package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	defaultOpenPTUID = 10001
	defaultOpenPTGID = 10001
)

func main() {
	dataDir := getenv("OPENPT_DATA_DIR", "/data")
	appDir := getenv("OPENPT_APP_DIR", "/app")
	runUID, runGID, err := runtimeIDs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "openpt-entrypoint: %v\n", err)
		os.Exit(1)
	}
	if err := initDataDir(dataDir, appDir); err != nil {
		fmt.Fprintf(os.Stderr, "openpt-entrypoint: %v\n", err)
		os.Exit(1)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"openpt", "--config", filepath.Join(dataDir, "config.toml")}
	} else if strings.HasPrefix(args[0], "-") {
		args = append([]string{"openpt"}, args...)
	}
	if os.Geteuid() == 0 {
		fixPermissions(dataDir, runUID, runGID)
		if err := syscall.Setgid(runGID); err != nil {
			fmt.Fprintf(os.Stderr, "openpt-entrypoint: setgid: %v\n", err)
			os.Exit(1)
		}
		if err := syscall.Setuid(runUID); err != nil {
			fmt.Fprintf(os.Stderr, "openpt-entrypoint: setuid: %v\n", err)
			os.Exit(1)
		}
	}
	if err := exec(args); err != nil {
		fmt.Fprintf(os.Stderr, "openpt-entrypoint: %v\n", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func runtimeIDs() (int, int, error) {
	uid, err := getenvInt([]string{"OPENPT_UID", "PUID"}, defaultOpenPTUID)
	if err != nil {
		return 0, 0, err
	}
	gid, err := getenvInt([]string{"OPENPT_GID", "PGID"}, defaultOpenPTGID)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func getenvInt(keys []string, fallback int) (int, error) {
	for _, key := range keys {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("%s must be a non-negative integer", key)
		}
		return n, nil
	}
	return fallback, nil
}

func initDataDir(dataDir, appDir string) error {
	for _, dir := range []string{
		filepath.Join(dataDir, "torrents"),
		filepath.Join(dataDir, "clients"),
		filepath.Join(dataDir, "torrents_archive"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	configPath := filepath.Join(dataDir, "config.toml")
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := copyFile(filepath.Join(appDir, "examples", "config.docker.toml"), configPath); err != nil {
			return fmt.Errorf("create default config: %w", err)
		}
		fmt.Printf("created default config: %s\n", configPath)
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", configPath, err)
	}
	matches, err := filepath.Glob(filepath.Join(appDir, "clients", "*.client"))
	if err != nil {
		return err
	}
	for _, client := range matches {
		target := filepath.Join(dataDir, "clients", filepath.Base(client))
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", target, err)
		}
		if err := copyFile(client, target); err != nil {
			return fmt.Errorf("copy %s: %w", filepath.Base(client), err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

func fixPermissions(dataDir string, uid, gid int) {
	_ = os.Chown(dataDir, uid, gid)
	_ = filepath.WalkDir(filepath.Join(dataDir, "clients"), func(path string, _ os.DirEntry, err error) error {
		if err == nil {
			_ = os.Chown(path, uid, gid)
		}
		return nil
	})
	for _, path := range []string{
		filepath.Join(dataDir, "torrents"),
		filepath.Join(dataDir, "torrents_archive"),
		filepath.Join(dataDir, "config.toml"),
		filepath.Join(dataDir, "openpt_state.json"),
	} {
		_ = os.Chown(path, uid, gid)
	}
}

func exec(args []string) error {
	path, err := execLookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}

var execLookPath = func(file string) (string, error) {
	if strings.Contains(file, "/") {
		return file, nil
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		path := filepath.Join(dir, file)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s not found in PATH", file)
}
