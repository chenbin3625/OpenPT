package scheduler

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"openpt/internal/bandwidth"
	"openpt/internal/clientemu"
	"openpt/internal/config"
	"openpt/internal/store"
	"openpt/internal/torrent"
	"openpt/internal/tracker"
)

// stopReason 表示种子停止的原因
type stopReason int

const (
	StopReasonRemoved     stopReason = iota // 种子文件被删除
	StopReasonManual                        // 用户手动停止/热重载
	StopReasonRatioTarget                   // 达到分享率目标
)

// maxConcurrentStopped 限制并发 stopped announce 的数量，避免批量删除种子时
// 瞬间打出大量 stopped 请求淹没 tracker。
const maxConcurrentStopped = 8

type Result struct {
	NextEvent clientemu.Event
	Delay     time.Duration
	Done      bool
}

// TorrentStatus holds the status of a torrent for the web UI.
type TorrentStatus struct {
	InfoHash        string  `json:"info_hash"`
	Name            string  `json:"name"`
	Size            int64   `json:"size"`
	Uploaded        int64   `json:"uploaded"`
	SpeedBps        int64   `json:"speed_bps"`
	Seeders         int     `json:"seeders"`
	Leechers        int     `json:"leechers"`
	Ratio           float64 `json:"ratio"`
	TrackerHost     string  `json:"tracker_host"`
	TrackerIndex    int     `json:"tracker_index"`
	TrackerCount    int     `json:"tracker_count"`
	Failures        int     `json:"failures"`
	HasIssue        bool    `json:"has_issue"`
	IssueReason     string  `json:"issue_reason"`
	LastError       string  `json:"last_error"`
	LastAnnounceAt  string  `json:"last_announce_at,omitempty"`
	NextAnnounceAt  string  `json:"next_announce_at,omitempty"`
	LastIntervalSec int64   `json:"last_interval_seconds"`
	RetryInSec      int64   `json:"retry_in_seconds"`
	NextEvent       string  `json:"next_event"`
}

func NextAfter(event clientemu.Event, interval time.Duration, err error) Result {
	if err != nil {
		return Result{NextEvent: event, Delay: interval}
	}
	if event == clientemu.EventStopped {
		return Result{Done: true}
	}
	return Result{NextEvent: clientemu.EventNone, Delay: interval}
}

type Scheduler struct {
	cfgMu   sync.RWMutex
	cfg     config.Config
	client  *clientemu.Client
	tracker *tracker.Client
	bw      *bandwidth.Dispatcher
	store   *store.Store
	log     *slog.Logger

	mu        sync.Mutex
	active    map[[20]byte]*announcer
	completed map[[20]byte]bool // 已完成的种子，不再重新添加

	// stoppedSem 限制并发 stopped announce 数量；stoppedWG 用于停机时等待在途的异步 stopped announce
	stoppedSem chan struct{}
	stoppedWG  sync.WaitGroup
}

type announcer struct {
	torrent      *torrent.Torrent
	mu           sync.Mutex // 保护 trackerIndex 和状态字段
	trackerIndex int
	lastInterval time.Duration
	failures     int
	lastError    string // 最后一次失败的错误消息
	lastAnnounce time.Time
	nextAnnounce time.Time
	nextEvent    clientemu.Event
	started      bool
	parent       context.Context
	cancel       context.CancelFunc
}

func (a *announcer) isStarted() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.started
}

func (a *announcer) markStarted() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.started = true
}

func New(cfg config.Config, emu *clientemu.Client, tc *tracker.Client, bw *bandwidth.Dispatcher, st *store.Store, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cfg: cfg, client: emu, tracker: tc, bw: bw, store: st, log: log,
		active:     map[[20]byte]*announcer{},
		completed:  map[[20]byte]bool{},
		stoppedSem: make(chan struct{}, maxConcurrentStopped),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.fillSlots(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-s.store.Events():
				switch ev.Type {
				case store.Added:
					s.fillSlots(ctx)
				case store.Removed:
					s.stopTorrent(ctx, ev.Torrent.InfoHash, StopReasonRemoved)
					// fillSlots 已在 stopTorrent 中调用，不需要重复
				}
			}
		}
	}()
}

func (s *Scheduler) Stop(ctx context.Context) {
	s.mu.Lock()
	list := make([]*announcer, 0, len(s.active))
	for _, a := range s.active {
		if a.cancel != nil {
			a.cancel()
		}
		if a.isStarted() {
			list = append(list, a)
		}
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, a := range list {
		wg.Add(1)
		go func(a *announcer) {
			defer wg.Done()
			s.announce(ctx, a, clientemu.EventStopped)
		}(a)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
	case <-done:
	}
	// 等待因种子移除而触发的在途异步 stopped announce，避免停机时漏发
	stoppedDone := make(chan struct{})
	go func() { s.stoppedWG.Wait(); close(stoppedDone) }()
	select {
	case <-ctx.Done():
	case <-stoppedDone:
	}
}

func (s *Scheduler) UpdateConfig(cfg config.Config) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.cfg = cfg
}

func (s *Scheduler) Config() config.Config {
	return s.config()
}

func (s *Scheduler) FillSlots(ctx context.Context) {
	s.fillSlots(ctx)
}

func (s *Scheduler) Reconcile(ctx context.Context) {
	s.stopOverflow(ctx)
	s.fillSlots(ctx)
}

func (s *Scheduler) config() config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

func (s *Scheduler) stopOverflow(ctx context.Context) {
	cfg := s.config()
	if cfg.SimultaneousSeed <= 0 {
		return
	}
	for ctx.Err() == nil {
		cfg = s.config()
		s.mu.Lock()
		overflow := len(s.active) - cfg.SimultaneousSeed
		if overflow <= 0 {
			s.mu.Unlock()
			return
		}
		hashes := make([][20]byte, 0, len(s.active))
		for hash := range s.active {
			hashes = append(hashes, hash)
		}
		s.mu.Unlock()
		sort.Slice(hashes, func(i, j int) bool {
			return HashID(hashes[i]) < HashID(hashes[j])
		})
		for i := 0; i < overflow && i < len(hashes) && ctx.Err() == nil; i++ {
			s.stopTorrent(ctx, hashes[i], StopReasonManual)
		}
	}
}

// fillSlots fills available slots with torrents from the store.
// It is safe for concurrent calls: tryAdd checks active map under lock.
func (s *Scheduler) fillSlots(parent context.Context) {
	for parent.Err() == nil {
		cfg := s.config()
		s.mu.Lock()
		if cfg.SimultaneousSeed > 0 && len(s.active) >= cfg.SimultaneousSeed {
			s.mu.Unlock()
			return
		}
		active := make(map[[20]byte]bool, len(s.active))
		for hash := range s.active {
			active[hash] = true
		}
		// 将已完成的种子也加入排除列表
		for hash := range s.completed {
			active[hash] = true
		}
		s.mu.Unlock()
		t, err := s.store.PickNotIn(active)
		if err != nil {
			return
		}
		s.tryAdd(parent, t)
	}
}

func (s *Scheduler) tryAdd(parent context.Context, t *torrent.Torrent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.config()
	if _, ok := s.active[t.InfoHash]; ok || (cfg.SimultaneousSeed > 0 && len(s.active) >= cfg.SimultaneousSeed) {
		return false
	}
	ctx, cancel := context.WithCancel(parent)
	now := time.Now()
	a := &announcer{
		torrent:      t,
		lastInterval: 5 * time.Second,
		nextAnnounce: now,
		nextEvent:    clientemu.EventStarted,
		parent:       parent,
		cancel:       cancel,
	}
	s.active[t.InfoHash] = a
	s.log.Info("torrent scheduled", "name", t.Name, "info_hash", t.InfoHashHex())
	go s.loop(ctx, a, clientemu.EventStarted, 0)
	return true
}

func (s *Scheduler) stopTorrent(ctx context.Context, hash [20]byte, reason stopReason) {
	a := s.removeActive(hash)
	if a != nil && a.isStarted() {
		// 异步发送 stopped announce，避免阻塞事件循环（批量删除时尤其重要）
		s.sendStoppedAsync(ctx, a)
	}
	if a != nil {
		s.bw.Unregister(a.torrent.InfoHashHex())
	}

	// 只在非分享率目标原因时清除 completed 标记
	// 分享率达标的种子应保持 completed 状态，避免重新添加
	if reason != StopReasonRatioTarget {
		s.clearCompleted(hash)
	}

	s.fillSlots(ctx)
}

// sendStoppedAsync 在并发受限的 goroutine 中发送 stopped announce。
// 使用传入的 ctx（调度器根 ctx）：announcer 自身的 ctx 已被 removeActive 取消，
// 而根 ctx 在正常运行期间存活、停机时取消以中断在途请求。单次请求时长由 tracker
// HTTP 客户端的 Timeout 兜底，无需额外 context 超时（避免配置为 0 时立即超时）。
func (s *Scheduler) sendStoppedAsync(ctx context.Context, a *announcer) {
	s.stoppedWG.Add(1)
	go func() {
		defer s.stoppedWG.Done()
		select {
		case s.stoppedSem <- struct{}{}:
			defer func() { <-s.stoppedSem }()
		case <-ctx.Done():
			return
		}
		if _, err := s.announce(ctx, a, clientemu.EventStopped); err != nil {
			// ctx 已取消时的失败属于停机正常路径，不记录告警
			if ctx.Err() == nil {
				s.log.Warn("async stopped announce failed", "name", a.torrent.Name, "error", err)
			}
		}
	}()
}

// clearCompleted 清除 completed 标记，允许种子重新添加
func (s *Scheduler) clearCompleted(hash [20]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.completed, hash)
}

func (s *Scheduler) removeActive(hash [20]byte) *announcer {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.active[hash]
	if a != nil && a.cancel != nil {
		a.cancel()
	}
	if a != nil {
		delete(s.active, hash)
	}
	return a
}

func (s *Scheduler) loop(ctx context.Context, a *announcer, event clientemu.Event, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		resp, err := s.announce(ctx, a, event)
		now := time.Now()
		cfg := s.config()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.mu.Lock()
			a.failures++
			failures := a.failures
			a.lastError = err.Error()
			a.lastAnnounce = now
			a.mu.Unlock()
			delay := backoffDelay(failures, cfg.TrackerFailureBackoffMin(), cfg.TrackerFailureBackoffMax())
			a.mu.Lock()
			a.nextAnnounce = now.Add(delay)
			a.nextEvent = event
			a.mu.Unlock()
			s.log.Warn("announce failed", "event", eventName(event), "name", a.torrent.Name, "failures", failures, "retry_in", delay, "error", err)
			timer.Reset(delay)
			continue
		} else {
			// announce 成功返回后，ctx 可能已被并发的 stopTorrent 取消。
			// 此时 stopTorrent 已负责清理（removeActive/Unregister/Stopped），
			// 这里直接退出，避免再次 Register 造成 bw 条目泄漏。
			if ctx.Err() != nil {
				return
			}
			a.mu.Lock()
			a.failures = 0
			a.lastError = ""
			a.lastAnnounce = now
			interval := a.lastInterval
			a.mu.Unlock()
			// 遵守 tracker 的 min interval：取 interval 与 min interval 的较大值，
			// 避免过于频繁上报被站点 ban。
			intervalSeconds := resp.Interval
			if resp.MinInterval > intervalSeconds {
				intervalSeconds = resp.MinInterval
			}
			if intervalSeconds > 0 {
				interval = time.Duration(intervalSeconds) * time.Second
				a.mu.Lock()
				a.lastInterval = interval
				a.mu.Unlock()
			}
			if event == clientemu.EventStarted {
				a.markStarted()
				s.bw.Register(a.torrent.InfoHashHex())
			}
			s.bw.UpdatePeers(a.torrent.InfoHashHex(), resp.Seeders, resp.Leechers)
			if ratioReached(cfg.Uploaded.RatioTarget, a.torrent.Size, s.bw.Get(a.torrent.InfoHashHex()).Uploaded) {
				s.completeTorrent(ctx, a, cfg.Uploaded.RatioTarget)
				return
			}
		}
		event = clientemu.EventNone
		a.mu.Lock()
		interval := a.lastInterval
		a.nextAnnounce = now.Add(interval)
		a.nextEvent = event
		a.mu.Unlock()
		timer.Reset(interval)
	}
}

func (s *Scheduler) announce(ctx context.Context, a *announcer, event clientemu.Event) (tracker.Response, error) {
	cfg := s.config()
	stats := s.bw.Get(a.torrent.InfoHashHex())
	query, err := s.client.RenderQuery(clientemu.RenderInput{
		InfoHash:   a.torrent.InfoHashBytes(),
		InfoHashID: a.torrent.InfoHashHex(),
		Uploaded:   stats.Uploaded,
		Downloaded: stats.Downloaded,
		Left:       stats.Left,
		Port:       cfg.Announce.Port,
		Event:      event,
		IP:         cfg.Announce.IP,
		IPv6:       cfg.Announce.IPv6,
	})
	if err != nil {
		return tracker.Response{}, err
	}
	a.mu.Lock()
	base := a.torrent.AnnounceList[a.trackerIndex%len(a.torrent.AnnounceList)]
	a.mu.Unlock()
	s.log.Info("announce", "event", eventName(event), "host", trackerHost(base), "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex())
	resp, err := s.tracker.Announce(ctx, base, query, s.client.HeadersForRequest())
	if err != nil {
		a.mu.Lock()
		a.trackerIndex = (a.trackerIndex + 1) % len(a.torrent.AnnounceList)
		a.mu.Unlock()
		return tracker.Response{}, err
	}
	s.log.Info("tracker response", "event", eventName(event), "interval", resp.Interval, "seeders", resp.Seeders, "leechers", resp.Leechers, "name", a.torrent.Name)
	return resp, nil
}

func (s *Scheduler) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active)
}

func (s *Scheduler) completeTorrent(ctx context.Context, a *announcer, ratioTarget float64) {
	s.log.Info("ratio target reached; completing torrent", "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex(), "ratio_target", ratioTarget)
	// 标记为已完成，防止重新添加
	s.markCompleted(a.torrent.InfoHash)
	s.bw.Unregister(a.torrent.InfoHashHex())
	s.removeActive(a.torrent.InfoHash)
	// removeActive 后 announcer 自身 ctx 已取消，异步发送 stopped announce
	if a.isStarted() {
		s.sendStoppedAsync(ctx, a)
	}
	s.fillSlots(a.fillContext(ctx))
}

// markCompleted 标记种子为已完成状态，防止重新调度
func (s *Scheduler) markCompleted(hash [20]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed[hash] = true
}

func (a *announcer) fillContext(fallback context.Context) context.Context {
	if a.parent != nil {
		return a.parent
	}
	return fallback
}

func ratioReached(target float64, size, uploaded int64) bool {
	if target <= 0 || size <= 0 || uploaded < 0 {
		return false
	}
	return float64(uploaded)/float64(size) >= target
}

func backoffDelay(failures int, minDelay, maxDelay time.Duration) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := minDelay
	for i := 1; i < failures; i++ {
		if delay >= maxDelay/2 {
			return maxDelay
		}
		delay *= 2
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func eventName(e clientemu.Event) string {
	if e == clientemu.EventNone {
		return "regular"
	}
	return string(e)
}

func trackerHost(raw string) string {
	for _, prefix := range []string{"http://", "https://"} {
		raw = strings.TrimPrefix(raw, prefix)
	}
	for i, ch := range raw {
		if ch == '/' || ch == '?' {
			return raw[:i]
		}
	}
	return raw
}

func HashID(hash [20]byte) string { return hex.EncodeToString(hash[:]) }

// Status returns the status of all active torrents.
func (s *Scheduler) Status() []TorrentStatus {
	// 快照 active map 后释放调度器锁，避免阻塞新增/删除操作
	type snapshot struct {
		a           *announcer
		infoHashHex string
	}
	s.mu.Lock()
	snapshots := make([]snapshot, 0, len(s.active))
	for _, a := range s.active {
		snapshots = append(snapshots, snapshot{a: a, infoHashHex: a.torrent.InfoHashHex()})
	}
	s.mu.Unlock()

	out := make([]TorrentStatus, 0, len(snapshots))
	for _, snap := range snapshots {
		a := snap.a
		infoHashHex := snap.infoHashHex
		stats := s.bw.Get(infoHashHex)
		ratio := float64(0)
		if a.torrent.Size > 0 && stats.Uploaded > 0 {
			ratio = float64(stats.Uploaded) / float64(a.torrent.Size)
		}
		trackerHostStr := ""
		trackerIndex := 0
		trackerCount := len(a.torrent.AnnounceList)
		failures := 0
		lastErr := ""
		lastAnnounce := time.Time{}
		nextAnnounce := time.Time{}
		nextEvent := clientemu.EventNone
		lastInterval := time.Duration(0)
		if len(a.torrent.AnnounceList) > 0 {
			a.mu.Lock()
			trackerIndex = a.trackerIndex % len(a.torrent.AnnounceList)
			trackerHostStr = trackerHost(a.torrent.AnnounceList[trackerIndex])
			failures = a.failures
			lastErr = a.lastError
			lastAnnounce = a.lastAnnounce
			nextAnnounce = a.nextAnnounce
			nextEvent = a.nextEvent
			lastInterval = a.lastInterval
			a.mu.Unlock()
		}

		hasIssue := false
		issueReasons := []string{}

		if failures > 0 {
			hasIssue = true
			if lastErr != "" {
				issueReasons = append(issueReasons, fmt.Sprintf("失败 %d 次: %s", failures, lastErr))
			} else {
				issueReasons = append(issueReasons, fmt.Sprintf("连续失败 %d 次", failures))
			}
		}
		if stats.Seeders == 0 && stats.Leechers == 0 {
			hasIssue = true
			issueReasons = append(issueReasons, "无 peers 连接")
		} else if stats.Leechers == 0 {
			issueReasons = append(issueReasons, "无下载者")
		}
		issueReason := ""
		if len(issueReasons) > 0 {
			issueReason = strings.Join(issueReasons, "; ")
		}

		out = append(out, TorrentStatus{
			InfoHash:        infoHashHex,
			Name:            a.torrent.Name,
			Size:            a.torrent.Size,
			Uploaded:        stats.Uploaded,
			SpeedBps:        stats.CurrentSpeedBps,
			Seeders:         stats.Seeders,
			Leechers:        stats.Leechers,
			Ratio:           ratio,
			TrackerHost:     trackerHostStr,
			TrackerIndex:    trackerIndex,
			TrackerCount:    trackerCount,
			Failures:        failures,
			HasIssue:        hasIssue,
			IssueReason:     issueReason,
			LastError:       lastErr,
			LastAnnounceAt:  formatStatusTime(lastAnnounce),
			NextAnnounceAt:  formatStatusTime(nextAnnounce),
			LastIntervalSec: int64(lastInterval.Seconds()),
			RetryInSec:      secondsUntil(nextAnnounce),
			NextEvent:       eventName(nextEvent),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].InfoHash < out[j].InfoHash
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func formatStatusTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func secondsUntil(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	seconds := int64(time.Until(t).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}
