package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"openpt/internal/bandwidth"
	"openpt/internal/scheduler"
	"openpt/internal/store"
)

//go:embed index.html
var indexHTML []byte

// StatusResponse represents the full status response.
type StatusResponse struct {
	Torrents []scheduler.TorrentStatus `json:"torrents"`
}

// ConfigItem represents a configuration item with Chinese label.
type ConfigItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// ConfigResponse represents the configuration response.
type ConfigResponse struct {
	Items []ConfigItem `json:"items"`
}

// Handler provides HTTP handlers for the web UI.
type Handler struct {
	store     *store.Store
	scheduler *scheduler.Scheduler
	bw        *bandwidth.Dispatcher
}

// New creates a new web Handler.
func New(st *store.Store, s *scheduler.Scheduler, bw *bandwidth.Dispatcher) *Handler {
	return &Handler{
		store:     st,
		scheduler: s,
		bw:        bw,
	}
}

// RegisterRoutes registers the web UI routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/api/status", h.handleStatus)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/events", h.handleEvents)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := StatusResponse{
		Torrents: h.scheduler.Status(),
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cfg := h.scheduler.Config()

	strategy := "不累计上传量"
	switch cfg.Uploaded.Strategy {
	case "conservative_rate":
		strategy = "保守速率"
	case "configured_rate":
		strategy = "配置速率"
	}

	items := []ConfigItem{
		{Key: "torrents_dir", Label: "种子目录", Value: cfg.TorrentsDir},
		{Key: "clients_dir", Label: "客户端配置目录", Value: cfg.ClientsDir},
		{Key: "client", Label: "客户端伪装", Value: cfg.Client},
		{Key: "simultaneous_seed", Label: "同时保种数量", Value: fmt.Sprintf("%d", cfg.SimultaneousSeed)},
		{Key: "announce.port", Label: "Announce 端口", Value: fmt.Sprintf("%d", cfg.Announce.Port)},
		{Key: "announce.ip", Label: "上报 IPv4 地址", Value: defaultStr(cfg.Announce.IP, "自动检测")},
		{Key: "announce.ipv6", Label: "上报 IPv6 地址", Value: defaultStr(cfg.Announce.IPv6, "自动检测")},
		{Key: "tracker.timeout_seconds", Label: "Tracker 超时", Value: fmt.Sprintf("%d 秒", cfg.Tracker.TimeoutSeconds)},
		{Key: "tracker.proxy", Label: "代理地址", Value: defaultStr(cfg.Tracker.Proxy, "无")},
		{Key: "tracker.reuse_connections", Label: "复用连接", Value: boolToStr(cfg.TrackerReuseConnections())},
		{Key: "uploaded.strategy", Label: "上传策略", Value: strategy},
		{Key: "uploaded.configured_rate_bps", Label: "配置速率", Value: formatBps(cfg.Uploaded.ConfiguredRateBps)},
		{Key: "uploaded.min_rate_bps", Label: "最小速率", Value: formatBps(cfg.Uploaded.MinRateBps)},
		{Key: "uploaded.max_rate_bps", Label: "最大速率", Value: formatBps(cfg.Uploaded.MaxRateBps)},
		{Key: "uploaded.ratio_target", Label: "目标分享率", Value: formatRatioTarget(cfg.Uploaded.RatioTarget)},
		{Key: "metrics.listen", Label: "监控服务地址", Value: cfg.Metrics.Listen},
		{Key: "metrics.webui", Label: "Web UI", Value: boolToStr(cfg.Metrics.WebUI)},
	}

	json.NewEncoder(w).Encode(ConfigResponse{Items: items})
}

func boolToStr(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

func defaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func formatBps(bps int64) string {
	if bps == 0 {
		return "0"
	}
	return fmt.Sprintf("%.2f KB/s", float64(bps)/1024)
}

func formatRatioTarget(ratio float64) string {
	if ratio <= 0 {
		return "禁用"
	}
	return fmt.Sprintf("%.2f", ratio)
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastHash uint64

	// Send initial status
	if !h.sendStatusIfChanged(w, flusher, &lastHash) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !h.sendStatusIfChanged(w, flusher, &lastHash) {
				return
			}
		}
	}
}

func (h *Handler) sendStatusIfChanged(w http.ResponseWriter, flusher http.Flusher, lastHash *uint64) bool {
	resp := StatusResponse{
		Torrents: h.scheduler.Status(),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return true
	}
	// 仅数据变更时才推送
	hash := hashBytes(data)
	if hash == *lastHash {
		return true
	}
	*lastHash = hash
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// hashBytes computes a simple FNV-1a hash for change detection.
func hashBytes(data []byte) uint64 {
	var h uint64 = 14695981039346656037
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}
