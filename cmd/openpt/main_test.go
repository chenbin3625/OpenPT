package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"

	"openpt/internal/bandwidth"
	"openpt/internal/config"
	"openpt/internal/scheduler"
)

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
