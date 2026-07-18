package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openpt/internal/bandwidth"
	"openpt/internal/config"
	"openpt/internal/scheduler"
)

func TestPreserveRuntimeConfigKeepsRandomPortAcrossReload(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "current.toml")
	nextPath := filepath.Join(dir, "next.toml")
	for _, path := range []string{currentPath, nextPath} {
		if err := os.WriteFile(path, []byte(`client = "test.client"`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	current, err := config.Load(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	next, err := config.Load(nextPath)
	if err != nil {
		t.Fatal(err)
	}
	got := preserveRuntimeConfig(current, next)
	if got.Announce.Port != current.Announce.Port {
		t.Fatalf("announce port after reload = %d, want %d", got.Announce.Port, current.Announce.Port)
	}
}

func TestStartMetricsServerReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, err = startMetricsServer(config.Config{
		Metrics: config.MetricsConfig{
			Enabled: true,
			Listen:  ln.Addr().String(),
			Path:    "/metrics",
		},
	}, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected listen error for occupied metrics port")
	}
}

func TestMetricsServerIncludesPrometheusMetadata(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bw := bandwidth.New(bandwidth.Config{})
	s := scheduler.New(config.Config{}, nil, nil, bw, nil, log)
	server, err := startMetricsServer(config.Config{
		Metrics: config.MetricsConfig{
			Enabled: true,
			Listen:  "127.0.0.1:0",
			Path:    "/metrics",
		},
	}, bw, s, nil, log)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())
	if server.ReadHeaderTimeout == 0 || server.IdleTimeout == 0 || server.MaxHeaderBytes == 0 {
		t.Fatalf("HTTP server timeouts/header limit are not configured: %+v", server)
	}

	resp, err := http.Get("http://" + server.Addr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"# HELP openpt_bandwidth_current_rate_bps",
		"# TYPE openpt_bandwidth_current_rate_bps gauge",
		"# HELP openpt_torrent_uploaded_bytes",
		"# TYPE openpt_torrent_uploaded_bytes counter",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, text)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bw := bandwidth.New(bandwidth.Config{})
	s := scheduler.New(config.Config{}, nil, nil, bw, nil, log)
	server, err := startMetricsServer(config.Config{
		Metrics: config.MetricsConfig{Enabled: true, Listen: "127.0.0.1:0", Path: "/metrics"},
	}, bw, s, nil, log)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Shutdown(context.Background())

	resp, err := http.Get("http://" + server.Addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("health response = status %d body %q", resp.StatusCode, body)
	}
}
