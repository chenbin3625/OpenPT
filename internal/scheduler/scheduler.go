package scheduler

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
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

type Result struct {
	NextEvent clientemu.Event
	Delay     time.Duration
	Done      bool
}

// TorrentStatus holds the status of a torrent for the web UI.
type TorrentStatus struct {
	InfoHash    string  `json:"info_hash"`
	Name        string  `json:"name"`
	Size        int64   `json:"size"`
	Uploaded    int64   `json:"uploaded"`
	SpeedBps    int64   `json:"speed_bps"`
	Seeders     int     `json:"seeders"`
	Leechers    int     `json:"leechers"`
	Ratio       float64 `json:"ratio"`
	TrackerHost string  `json:"tracker_host"`
	Failures    int     `json:"failures"`
	HasIssue    bool    `json:"has_issue"`
	IssueReason string  `json:"issue_reason"`
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

	mu     sync.Mutex
	active map[[20]byte]*announcer
}

type announcer struct {
	torrent      *torrent.Torrent
	mu           sync.Mutex // 保护 trackerIndex
	trackerIndex int
	lastInterval time.Duration
	failures     int
	started      bool
	parent       context.Context
	cancel       context.CancelFunc
}

func New(cfg config.Config, emu *clientemu.Client, tc *tracker.Client, bw *bandwidth.Dispatcher, st *store.Store, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cfg: cfg, client: emu, tracker: tc, bw: bw, store: st, log: log,
		active: map[[20]byte]*announcer{},
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
					s.stopTorrent(ev.Torrent.InfoHash)
					s.fillSlots(ctx)
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
		if a.started {
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
}

func (s *Scheduler) UpdateConfig(cfg config.Config) {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.cfg = cfg
}

func (s *Scheduler) FillSlots(ctx context.Context) {
	s.fillSlots(ctx)
}

func (s *Scheduler) config() config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

// fillSlots fills available slots with torrents from the store.
// It is safe for concurrent calls: tryAdd checks active map under lock.
func (s *Scheduler) fillSlots(parent context.Context) {
	for parent.Err() == nil {
		cfg := s.config()
		s.mu.Lock()
		if len(s.active) >= cfg.SimultaneousSeed {
			s.mu.Unlock()
			return
		}
		active := make(map[[20]byte]bool, len(s.active))
		for hash := range s.active {
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
	if _, ok := s.active[t.InfoHash]; ok || len(s.active) >= cfg.SimultaneousSeed {
		return false
	}
	ctx, cancel := context.WithCancel(parent)
	a := &announcer{torrent: t, lastInterval: 5 * time.Second, parent: parent, cancel: cancel}
	s.active[t.InfoHash] = a
	s.log.Info("torrent scheduled", "name", t.Name, "info_hash", t.InfoHashHex())
	go s.loop(ctx, a, clientemu.EventStarted, 0)
	return true
}

func (s *Scheduler) stopTorrent(hash [20]byte) {
	a := s.removeActive(hash)
	if a != nil {
		s.bw.Unregister(a.torrent.InfoHashHex())
	}
	if a != nil && a.started {
		go s.announce(context.Background(), a, clientemu.EventStopped)
	}
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
		cfg := s.config()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.failures++
			delay := backoffDelay(a.failures, cfg.TrackerFailureBackoffMin(), cfg.TrackerFailureBackoffMax())
			s.log.Warn("announce failed", "event", eventName(event), "name", a.torrent.Name, "failures", a.failures, "retry_in", delay, "error", err)
			if a.failures >= cfg.MaxConsecutiveFailures {
				s.log.Warn("max consecutive failures reached; archiving torrent", "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex())
				s.archiveTorrent(ctx, a, "max consecutive announce failures")
				return
			}
			timer.Reset(delay)
			continue
		} else {
			a.failures = 0
			if resp.Interval > 0 {
				a.lastInterval = time.Duration(resp.Interval) * time.Second
			}
			if event == clientemu.EventStarted {
				a.started = true
				s.bw.Register(a.torrent.InfoHashHex())
			}
			s.bw.UpdatePeers(a.torrent.InfoHashHex(), resp.Seeders, resp.Leechers)
			if event == clientemu.EventStopped {
				s.bw.Unregister(a.torrent.InfoHashHex())
				s.removeActive(a.torrent.InfoHash)
				s.fillSlots(a.fillContext(ctx))
				return
			}
			if ratioReached(cfg.Uploaded.RatioTarget, a.torrent.Size, s.bw.Get(a.torrent.InfoHashHex()).Uploaded) {
				s.completeTorrent(ctx, a, cfg.Uploaded.RatioTarget)
				return
			}
			if !cfg.KeepTorrentWithZeroLeechers && resp.Leechers < 1 {
				s.log.Info("archiving torrent with no peers", "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex(), "seeders", resp.Seeders, "leechers", resp.Leechers)
				if _, err := s.announce(ctx, a, clientemu.EventStopped); err != nil {
					s.log.Warn("failed to send stopped announce after no peers", "name", a.torrent.Name, "error", err)
				}
				s.archiveTorrent(ctx, a, "tracker reported zero seeders or leechers")
				return
			}
		}
		event = clientemu.EventNone
		timer.Reset(a.lastInterval)
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
	if _, err := s.announce(ctx, a, clientemu.EventStopped); err != nil {
		s.log.Warn("failed to send stopped announce after ratio target", "name", a.torrent.Name, "error", err)
	}
	s.archiveTorrent(ctx, a, "ratio target reached")
}

func (s *Scheduler) archiveTorrent(ctx context.Context, a *announcer, reason string) {
	s.bw.Unregister(a.torrent.InfoHashHex())
	s.removeActive(a.torrent.InfoHash)
	s.store.ArchiveByHash(a.torrent.InfoHash, reason)
	s.fillSlots(a.fillContext(ctx))
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
		raw = stringsTrimPrefix(raw, prefix)
	}
	for i, ch := range raw {
		if ch == '/' || ch == '?' {
			return raw[:i]
		}
	}
	return raw
}

func stringsTrimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func HashID(hash [20]byte) string { return hex.EncodeToString(hash[:]) }

// Status returns the status of all active torrents.
func (s *Scheduler) Status() []TorrentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TorrentStatus, 0, len(s.active))
	for _, a := range s.active {
		infoHashHex := a.torrent.InfoHashHex()
		stats := s.bw.Get(infoHashHex)
		ratio := float64(0)
		if a.torrent.Size > 0 && stats.Uploaded > 0 {
			ratio = float64(stats.Uploaded) / float64(a.torrent.Size)
		}
		trackerHostStr := ""
		if len(a.torrent.AnnounceList) > 0 {
			a.mu.Lock()
			trackerHostStr = trackerHost(a.torrent.AnnounceList[a.trackerIndex%len(a.torrent.AnnounceList)])
			a.mu.Unlock()
		}

		// Determine issue status and reason
		hasIssue := false
		issueReasons := []string{}
		if a.failures > 0 {
			hasIssue = true
			issueReasons = append(issueReasons, fmt.Sprintf("连续失败 %d 次", a.failures))
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
			InfoHash:    infoHashHex,
			Name:        a.torrent.Name,
			Size:        a.torrent.Size,
			Uploaded:    stats.Uploaded,
			SpeedBps:    stats.CurrentSpeedBps,
			Seeders:     stats.Seeders,
			Leechers:    stats.Leechers,
			Ratio:       ratio,
			TrackerHost: trackerHostStr,
			Failures:    a.failures,
			HasIssue:    hasIssue,
			IssueReason: issueReason,
		})
	}
	return out
}
