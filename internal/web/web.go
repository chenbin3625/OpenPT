package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"openpt/internal/bandwidth"
	"openpt/internal/scheduler"
	"openpt/internal/store"
)

//go:embed index.html
var indexHTML []byte

//go:embed styles.css
var stylesCSS []byte

//go:embed openpt-icon.svg
var iconSVG []byte

const (
	// sseStatusPollInterval 是 SSE 端点检查状态是否变化并推送的间隔。
	sseStatusPollInterval = 2 * time.Second
	// defaultSSEHeartbeatInterval 是 SSE 心跳间隔。长时间无数据变化时，
	// 中间代理 / 浏览器可能断开空闲连接，定期发送注释行保持连接活跃。
	defaultSSEHeartbeatInterval = 15 * time.Second
)

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
	store             *store.Store
	scheduler         *scheduler.Scheduler
	bw                *bandwidth.Dispatcher
	heartbeatInterval time.Duration
}

// New creates a new web Handler.
func New(st *store.Store, s *scheduler.Scheduler, bw *bandwidth.Dispatcher) *Handler {
	return &Handler{
		store:             st,
		scheduler:         s,
		bw:                bw,
		heartbeatInterval: defaultSSEHeartbeatInterval,
	}
}

// RegisterRoutes registers the web UI routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/styles.css", h.handleStyles)
	mux.HandleFunc("/openpt-icon.svg", h.handleIcon)
	mux.HandleFunc("/api/status", h.handleStatus)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/events", h.handleEvents)
}

func (h *Handler) handleStyles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(stylesCSS)
}

func (h *Handler) handleIcon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(iconSVG)
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
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("web: failed to encode status response: %v", err)
	}
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
		{Key: "archive_dir", Label: "归档目录", Value: cfg.ArchiveDir},
		{Key: "clients_dir", Label: "客户端配置目录", Value: cfg.ClientsDir},
		{Key: "state_file", Label: "状态文件", Value: cfg.StateFile},
		{Key: "client", Label: "客户端伪装", Value: cfg.Client},
		{Key: "simultaneous_seed", Label: "同时保种数量", Value: formatSeedLimit(cfg.SimultaneousSeed)},
		{Key: "scan_interval_seconds", Label: "扫描间隔", Value: formatSeconds(cfg.ScanIntervalSeconds)},
		{Key: "shutdown_stop_timeout_seconds", Label: "关闭等待停止上报", Value: formatSeconds(cfg.ShutdownStopTimeoutSeconds)},
		{Key: "announce.port", Label: "Announce 端口", Value: fmt.Sprintf("%d", cfg.Announce.Port)},
		{Key: "announce.ip", Label: "上报 IPv4 地址", Value: defaultStr(cfg.Announce.IP, "自动检测")},
		{Key: "announce.ipv6", Label: "上报 IPv6 地址", Value: defaultStr(cfg.Announce.IPv6, "自动检测")},
		{Key: "tracker.timeout_seconds", Label: "Tracker 超时", Value: fmt.Sprintf("%d 秒", cfg.Tracker.TimeoutSeconds)},
		{Key: "tracker.proxy", Label: "代理地址", Value: defaultStr(redactProxy(cfg.Tracker.Proxy), "无")},
		{Key: "tracker.reuse_connections", Label: "复用连接", Value: boolToStr(cfg.TrackerReuseConnections())},
		{Key: "tracker.max_idle_conns", Label: "最大空闲连接数", Value: fmt.Sprintf("%d", cfg.Tracker.MaxIdleConns)},
		{Key: "tracker.max_idle_conns_per_host", Label: "单 Host 最大空闲连接数", Value: fmt.Sprintf("%d", cfg.Tracker.MaxIdleConnsPerHost)},
		{Key: "tracker.idle_conn_timeout_seconds", Label: "空闲连接超时", Value: formatSeconds(cfg.Tracker.IdleConnTimeoutSeconds)},
		{Key: "tracker.failure_backoff_min_seconds", Label: "失败最小退避", Value: formatSeconds(cfg.Tracker.FailureBackoffMinSeconds)},
		{Key: "tracker.failure_backoff_max_seconds", Label: "失败最大退避", Value: formatSeconds(cfg.Tracker.FailureBackoffMaxSeconds)},
		{Key: "uploaded.strategy", Label: "上传策略", Value: strategy},
		{Key: "uploaded.configured_rate_bps", Label: "配置速率", Value: formatBps(cfg.Uploaded.ConfiguredRateBps)},
		{Key: "uploaded.min_rate_bps", Label: "最小速率", Value: formatBps(cfg.Uploaded.MinRateBps)},
		{Key: "uploaded.max_rate_bps", Label: "最大速率", Value: formatBps(cfg.Uploaded.MaxRateBps)},
		{Key: "uploaded.conservative_rate_bps", Label: "保守速率", Value: formatBps(cfg.Uploaded.ConservativeRateBps)},
		{Key: "uploaded.random_jitter_percent", Label: "随机抖动", Value: fmt.Sprintf("%d%%", cfg.Uploaded.RandomJitterPercent)},
		{Key: "uploaded.random_refresh_seconds", Label: "随机速率刷新", Value: formatSeconds(cfg.Uploaded.RandomRefreshSeconds)},
		{Key: "uploaded.ratio_target", Label: "目标分享率", Value: formatRatioTarget(cfg.Uploaded.RatioTarget)},
		{Key: "metrics.enabled", Label: "监控服务", Value: boolToStr(cfg.Metrics.Enabled)},
		{Key: "metrics.listen", Label: "监控服务地址", Value: cfg.Metrics.Listen},
		{Key: "metrics.path", Label: "指标路径", Value: cfg.Metrics.Path},
		{Key: "metrics.webui", Label: "Web UI", Value: boolToStr(cfg.Metrics.WebUI)},
		{Key: "logging.file", Label: "日志文件", Value: defaultStr(cfg.Logging.File, "标准输出")},
	}

	if err := json.NewEncoder(w).Encode(ConfigResponse{Items: items}); err != nil {
		log.Printf("web: failed to encode config response: %v", err)
	}
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

func formatSeedLimit(n int) string {
	if n == 0 {
		return "不限制（0）"
	}
	return fmt.Sprintf("%d", n)
}

func formatSeconds(seconds int) string {
	return fmt.Sprintf("%d 秒", seconds)
}

// redactProxy 去除代理 URL 中的密码，避免在 Web UI 中泄漏凭据。
func redactProxy(proxy string) string {
	if proxy == "" {
		return ""
	}
	u, err := url.Parse(proxy)
	if err != nil {
		// 无法解析时返回不包含凭据的占位，避免泄漏
		return "(已配置)"
	}
	if u.User != nil {
		// 仅保留用户名，去除密码
		u.User = url.User(u.User.Username())
	}
	return u.String()
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

	ticker := time.NewTicker(sseStatusPollInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(h.heartbeatInterval)
	defer heartbeat.Stop()

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
		case <-heartbeat.C:
			// SSE 注释行（以冒号开头），客户端 EventSource 会忽略，仅用于保持连接活跃
			if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
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
