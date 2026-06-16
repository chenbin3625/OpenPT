package scheduler

import (
	"context"
	"encoding/hex"
	"log/slog"
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
	trackerIndex int
	lastInterval time.Duration
	failures     int
	started      bool
	cancel       context.CancelFunc
}

func New(cfg config.Config, emu *clientemu.Client, tc *tracker.Client, bw *bandwidth.Dispatcher, st *store.Store, log *slog.Logger) *Scheduler {
	return &Scheduler{
		cfg: cfg, client: emu, tracker: tc, bw: bw, store: st, log: log,
		active: map[[20]byte]*announcer{},
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	for _, t := range s.store.List() {
		s.tryAdd(ctx, t)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-s.store.Events():
				switch ev.Type {
				case store.Added:
					s.tryAdd(ctx, ev.Torrent)
				case store.Removed:
					s.stopTorrent(ev.Torrent.InfoHash)
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

func (s *Scheduler) tryAdd(parent context.Context, t *torrent.Torrent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.active[t.InfoHash]; ok || len(s.active) >= s.cfg.SimultaneousSeed {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	a := &announcer{torrent: t, lastInterval: 5 * time.Second, cancel: cancel}
	s.active[t.InfoHash] = a
	s.log.Info("torrent scheduled", "name", t.Name, "info_hash", t.InfoHashHex())
	go s.loop(ctx, a, clientemu.EventStarted, 0)
}

func (s *Scheduler) stopTorrent(hash [20]byte) {
	s.mu.Lock()
	a := s.active[hash]
	if a != nil && a.cancel != nil {
		a.cancel()
	}
	s.mu.Unlock()
	if a != nil && a.started {
		go s.announce(context.Background(), a, clientemu.EventStopped)
	}
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
		if err != nil {
			a.failures++
			s.log.Warn("announce failed", "event", eventName(event), "name", a.torrent.Name, "failures", a.failures, "error", err)
			if a.failures >= s.cfg.MaxConsecutiveFailures {
				s.log.Warn("max consecutive failures reached; archiving torrent", "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex())
				s.bw.Unregister(a.torrent.InfoHashHex())
				s.store.ArchiveByHash(a.torrent.InfoHash, "max consecutive announce failures")
				s.mu.Lock()
				delete(s.active, a.torrent.InfoHash)
				s.mu.Unlock()
				return
			}
		} else {
			a.failures = 0
			if resp.Interval > 0 {
				a.lastInterval = time.Duration(resp.Interval) * time.Second
			}
			if event == clientemu.EventStarted {
				a.started = true
				s.bw.Register(a.torrent.InfoHashHex())
			}
			if event == clientemu.EventStopped {
				s.bw.Unregister(a.torrent.InfoHashHex())
				s.mu.Lock()
				delete(s.active, a.torrent.InfoHash)
				s.mu.Unlock()
				return
			}
			if !s.cfg.KeepTorrentWithZeroLeechers && resp.Leechers < 1 {
				s.log.Info("archiving torrent with zero leechers", "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex())
				s.store.ArchiveByHash(a.torrent.InfoHash, "tracker reported zero leechers")
				return
			}
		}
		event = clientemu.EventNone
		timer.Reset(a.lastInterval)
	}
}

func (s *Scheduler) announce(ctx context.Context, a *announcer, event clientemu.Event) (tracker.Response, error) {
	stats := s.bw.Get(a.torrent.InfoHashHex())
	query, err := s.client.RenderQuery(clientemu.RenderInput{
		InfoHash:   a.torrent.InfoHashBytes(),
		InfoHashID: a.torrent.InfoHashHex(),
		Uploaded:   stats.Uploaded,
		Downloaded: stats.Downloaded,
		Left:       stats.Left,
		Port:       s.cfg.Announce.Port,
		Event:      event,
		IP:         s.cfg.Announce.IP,
		IPv6:       s.cfg.Announce.IPv6,
	})
	if err != nil {
		return tracker.Response{}, err
	}
	base := a.torrent.AnnounceList[a.trackerIndex%len(a.torrent.AnnounceList)]
	s.log.Info("announce", "event", eventName(event), "host", trackerHost(base), "name", a.torrent.Name, "info_hash", a.torrent.InfoHashHex())
	resp, err := s.tracker.Announce(ctx, base, query, s.client.HeadersForRequest())
	if err != nil {
		a.trackerIndex = (a.trackerIndex + 1) % len(a.torrent.AnnounceList)
		return tracker.Response{}, err
	}
	s.log.Info("tracker response", "event", eventName(event), "interval", resp.Interval, "seeders", resp.Seeders, "leechers", resp.Leechers, "name", a.torrent.Name)
	return resp, nil
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
