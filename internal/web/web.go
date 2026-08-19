package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"openpt/internal/bandwidth"
	"openpt/internal/scheduler"
	"openpt/internal/store"
)

//go:embed all:dist
var distFS embed.FS

//go:embed openpt-icon.svg
var iconSVG []byte

const (
	// sseStatusPollInterval 是 SSE 端点检查状态是否变化并推送的初始轮询间隔。
	sseStatusPollInterval = 2 * time.Second
	// maxSSEPollInterval 是 SSE 自适应退避的轮询间隔上限：数据持续无变化时
	// 会指数退避到此值，降低空闲期间全量 Status() 遍历与 JSON 编码开销。
	maxSSEPollInterval = 15 * time.Second
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
	log               *slog.Logger
	heartbeatInterval time.Duration
	basePoll          time.Duration
	maxPoll           time.Duration
	// shutdown 在服务停机时被关闭，SSE 处理器据此主动退出，
	// 使 http.Server.Shutdown 不必等待长连接自然结束。nil 表示未注入（select 恒阻塞）。
	shutdown <-chan struct{}
}

// SetShutdownSignal 注入停机信号：服务 Shutdown 开始时关闭 ch，SSE 连接随即断开。
func (h *Handler) SetShutdownSignal(ch <-chan struct{}) {
	h.shutdown = ch
}

// New creates a new web Handler. log 为 nil 时使用 slog.Default()。
func New(st *store.Store, s *scheduler.Scheduler, bw *bandwidth.Dispatcher, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		store:             st,
		scheduler:         s,
		bw:                bw,
		log:               log,
		heartbeatInterval: defaultSSEHeartbeatInterval,
		basePoll:          sseStatusPollInterval,
		maxPoll:           maxSSEPollInterval,
	}
}

// RegisterRoutes registers the web UI routes on the given mux.
// 注意：这里注册的路由需与 config.ReservedWebUIRoutes 保持同步（单源清单）。
func (h *Handler) RegisterRoutes(mux *http.ServeMux) error {
	assets, err := fs.Sub(distFS, "dist")
	if err != nil {
		// go:embed 保证 dist 存在；此处仅防御，但以错误返回而非 panic。
		return fmt.Errorf("web: embedded dist unavailable: %w", err)
	}
	mux.Handle("/assets/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("/", h.handleIndex(assets))
	mux.HandleFunc("/openpt-icon.svg", h.handleIcon)
	mux.HandleFunc("/api/status", h.handleStatus)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/events", h.handleEvents)
	return nil
}

// handleIndex 返回一个处理函数，仅当路径为 / 时返回内嵌的 index.html，
// 其余路径返回 404，由 /assets/ 与 API 路由自行接管。
func (h *Handler) handleIndex(assets fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(data)
	}
}

func (h *Handler) handleIcon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(iconSVG)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := StatusResponse{
		Torrents: h.scheduler.Status(),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Warn("web: failed to encode status response", "error", err)
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
		h.log.Warn("web: failed to encode config response", "error", err)
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

	heartbeat := time.NewTicker(h.heartbeatInterval)
	defer heartbeat.Stop()

	var lastHash uint64

	// Send initial status
	if _, ok := h.sendStatusIfChanged(w, flusher, &lastHash); !ok {
		return
	}

	// 自适应轮询：数据无变化时把轮询间隔向 maxPoll 退避，数据变化时恢复初始值。
	// 活跃做种期间 uploaded 每秒变化，间隔维持在初始档；空闲/无种子时降低开销。
	poll := h.basePoll
	pollTimer := time.NewTimer(poll)
	defer pollTimer.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.shutdown: // nil channel 恒阻塞，未注入时不影响正常流程
			return
		case <-pollTimer.C:
			changed, ok := h.sendStatusIfChanged(w, flusher, &lastHash)
			if !ok {
				return
			}
			if changed {
				poll = h.basePoll
			} else if poll < h.maxPoll {
				poll = min(poll*3/2, h.maxPoll)
			}
			pollTimer.Reset(poll)
		case <-heartbeat.C:
			// SSE 注释行（以冒号开头），客户端 EventSource 会忽略，仅用于保持连接活跃
			if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// sendStatusIfChanged 计算当前状态并仅在变化时推送一行 data。
// 返回 (changed, ok)：changed 表示本次是否有数据变化；ok=false 表示写入连接失败，调用方应结束。
func (h *Handler) sendStatusIfChanged(w http.ResponseWriter, flusher http.Flusher, lastHash *uint64) (bool, bool) {
	resp := StatusResponse{
		Torrents: h.scheduler.Status(),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return false, true
	}
	// 仅数据变更时才推送
	hash := hashBytes(data)
	if hash == *lastHash {
		return false, true
	}
	*lastHash = hash
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return true, false
	}
	flusher.Flush()
	return true, true
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
