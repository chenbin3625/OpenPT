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
	initial := config.Config{
		Client:           "old.client",
		SimultaneousSeed: 1,
		Uploaded:         config.UploadedConfig{Strategy: "none"},
		Metrics:          config.MetricsConfig{Enabled: true, WebUI: true, Listen: "127.0.0.1:9090"},
	}
	s := scheduler.New(initial, nil, nil, nil, nil, nil)
	h := New(nil, s, nil)

	next := initial
	next.Client = "new.client"
	next.SimultaneousSeed = 7
	next.Uploaded.Strategy = "configured_rate"
	next.Uploaded.ConfiguredRateBps = 2048
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
	if items["uploaded.configured_rate_bps"] != "2.00 KB/s" {
		t.Fatalf("configured_rate item = %q, want 2.00 KB/s", items["uploaded.configured_rate_bps"])
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
