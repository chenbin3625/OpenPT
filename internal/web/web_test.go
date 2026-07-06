package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openpt/internal/config"
	"openpt/internal/scheduler"
)

func TestHandleConfigUsesCurrentSchedulerConfig(t *testing.T) {
	reuseConnections := false
	initial := config.Config{
		TorrentsDir:                "/old/torrents",
		ArchiveDir:                 "/old/archive",
		ClientsDir:                 "/old/clients",
		Client:                     "old.client",
		SimultaneousSeed:           1,
		ScanIntervalSeconds:        5,
		ShutdownStopTimeoutSeconds: 20,
		Tracker:                    config.TrackerConfig{ReuseConnections: &reuseConnections},
		Uploaded:                   config.UploadedConfig{Strategy: "none"},
		Metrics:                    config.MetricsConfig{Enabled: true, WebUI: true, Listen: "127.0.0.1:9090", Path: "/metrics"},
		Logging:                    config.LoggingConfig{File: ""},
	}
	s := scheduler.New(initial, nil, nil, nil, nil, nil)
	h := New(nil, s, nil)

	next := initial
	next.ArchiveDir = "/new/archive"
	next.Client = "new.client"
	next.SimultaneousSeed = 7
	next.ScanIntervalSeconds = 11
	next.Uploaded.Strategy = "configured_rate"
	next.Uploaded.ConfiguredRateBps = 2048
	next.Tracker.FailureBackoffMaxSeconds = 300
	next.Metrics.Path = "/custom-metrics"
	next.Logging.File = "/var/log/openpt.log"
	s.UpdateConfig(next)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	h.handleConfig(rec, req)

	var resp ConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	items := map[string]string{}
	for _, item := range resp.Items {
		items[item.Key] = item.Value
	}
	if items["client"] != "new.client" {
		t.Fatalf("client item = %q, want new.client", items["client"])
	}
	if items["simultaneous_seed"] != "7" {
		t.Fatalf("simultaneous_seed item = %q, want 7", items["simultaneous_seed"])
	}
	if items["archive_dir"] != "/new/archive" {
		t.Fatalf("archive_dir item = %q, want /new/archive", items["archive_dir"])
	}
	if items["scan_interval_seconds"] != "11 秒" {
		t.Fatalf("scan_interval item = %q, want 11 秒", items["scan_interval_seconds"])
	}
	if items["uploaded.configured_rate_bps"] != "2.00 KB/s" {
		t.Fatalf("configured_rate item = %q, want 2.00 KB/s", items["uploaded.configured_rate_bps"])
	}
	if items["tracker.failure_backoff_max_seconds"] != "300 秒" {
		t.Fatalf("tracker backoff item = %q, want 300 秒", items["tracker.failure_backoff_max_seconds"])
	}
	if items["metrics.path"] != "/custom-metrics" {
		t.Fatalf("metrics.path item = %q, want /custom-metrics", items["metrics.path"])
	}
	if items["logging.file"] != "/var/log/openpt.log" {
		t.Fatalf("logging.file item = %q, want /var/log/openpt.log", items["logging.file"])
	}
}

// TestSSEEmitsHeartbeat 验证 SSE 端点在无数据变化时仍定期发送心跳注释行，
// 防止中间代理 / 浏览器因空闲断开连接。
func TestSSEEmitsHeartbeat(t *testing.T) {
	s := scheduler.New(config.Config{
		Client:   "x",
		Uploaded: config.UploadedConfig{Strategy: "none"},
		Metrics:  config.MetricsConfig{Enabled: true, WebUI: true},
	}, nil, nil, nil, nil, nil)
	h := New(nil, s, nil)
	h.heartbeatInterval = 30 * time.Millisecond

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), ": keep-alive") {
			return // 收到心跳，测试通过
		}
	}
	t.Fatal("did not receive SSE heartbeat within timeout")
}
