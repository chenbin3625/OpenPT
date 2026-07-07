package bandwidth

import (
	"math/rand"
	"sync"
	"time"
)

type Stats struct {
	Uploaded        int64
	Downloaded      int64
	Left            int64
	CurrentSpeedBps int64
	Seeders         int
	Leechers        int
}

// maxTickElapsed 限制单次 tick 累计上传量的时间上限。
// 进程被挂起（笔记本休眠 / 容器 pause）后恢复时，elapsed 可能长达数小时；
// 若按实际 elapsed 累加，会一次性产生数 GB 的不自然上传尖峰，对伪造上传的 PT 保种场景
// 极易被站点判定为作弊。超过该上限时按上限计算，等价于"挂起期间不伪造上传"。
const maxTickElapsed = 2 * time.Second

type Config struct {
	Strategy             string
	ConservativeRateBps  int64
	ConfiguredRateBps    int64
	MinRateBps           int64
	MaxRateBps           int64
	RandomJitterPercent  int
	RandomRefreshSeconds int
}

type Dispatcher struct {
	mu            sync.RWMutex
	strategy      string
	baseRateBps   int64
	minRateBps    int64
	maxRateBps    int64
	currentRate   int64
	randomRefresh time.Duration
	nextRefresh   time.Time
	lastTick      time.Time
	stats         map[string]*Stats
	stop          chan struct{}
	stopOnce      sync.Once
	rng           *rand.Rand
}

func New(cfg Config) *Dispatcher {
	cfg = normalizeConfig(cfg)
	rate := int64(0)
	switch cfg.Strategy {
	case "conservative_rate":
		rate = cfg.ConservativeRateBps
	case "configured_rate":
		rate = cfg.ConfiguredRateBps
	}
	if rate < 0 {
		rate = 0
	}
	d := &Dispatcher{
		strategy:      cfg.Strategy,
		baseRateBps:   rate,
		minRateBps:    cfg.MinRateBps,
		maxRateBps:    cfg.MaxRateBps,
		randomRefresh: time.Duration(cfg.RandomRefreshSeconds) * time.Second,
		lastTick:      time.Now(),
		stats:         map[string]*Stats{},
		stop:          make(chan struct{}),
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.refreshCurrentRateLocked(time.Now())
	return d
}

func (d *Dispatcher) Start() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-d.stop:
				return
			case <-ticker.C:
				d.tick()
			}
		}
	}()
}

func (d *Dispatcher) Stop() { d.stopOnce.Do(func() { close(d.stop) }) }

func (d *Dispatcher) UpdateConfig(cfg Config) {
	cfg = normalizeConfig(cfg)
	rate := int64(0)
	switch cfg.Strategy {
	case "conservative_rate":
		rate = cfg.ConservativeRateBps
	case "configured_rate":
		rate = cfg.ConfiguredRateBps
	}
	if rate < 0 {
		rate = 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.strategy = cfg.Strategy
	d.baseRateBps = rate
	d.minRateBps = cfg.MinRateBps
	d.maxRateBps = cfg.MaxRateBps
	d.randomRefresh = time.Duration(cfg.RandomRefreshSeconds) * time.Second
	d.refreshCurrentRateLocked(time.Now())
	d.recomputeSpeedsLocked()
}

func (d *Dispatcher) Register(infoHash string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.stats[infoHash]; !ok {
		d.stats[infoHash] = &Stats{Left: 0}
	}
	d.recomputeSpeedsLocked()
}

func (d *Dispatcher) Restore(infoHash string, stats Stats) {
	if stats.Uploaded < 0 {
		stats.Uploaded = 0
	}
	if stats.Downloaded < 0 {
		stats.Downloaded = 0
	}
	if stats.Left < 0 {
		stats.Left = 0
	}
	if stats.Seeders < 0 {
		stats.Seeders = 0
	}
	if stats.Leechers < 0 {
		stats.Leechers = 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats[infoHash] = &Stats{
		Uploaded:   stats.Uploaded,
		Downloaded: stats.Downloaded,
		Left:       stats.Left,
		Seeders:    stats.Seeders,
		Leechers:   stats.Leechers,
	}
	d.recomputeSpeedsLocked()
}

func (d *Dispatcher) Unregister(infoHash string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.stats, infoHash)
	d.recomputeSpeedsLocked()
}

func (d *Dispatcher) UpdatePeers(infoHash string, seeders, leechers int) {
	if seeders < 0 {
		seeders = 0
	}
	if leechers < 0 {
		leechers = 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	st, ok := d.stats[infoHash]
	if !ok {
		st = &Stats{Left: 0}
		d.stats[infoHash] = st
	}
	st.Seeders = seeders
	st.Leechers = leechers
	d.recomputeSpeedsLocked()
}

func (d *Dispatcher) Get(infoHash string) Stats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if st, ok := d.stats[infoHash]; ok {
		return *st
	}
	return Stats{Left: 0}
}

func (d *Dispatcher) Snapshot() map[string]Stats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]Stats, len(d.stats))
	for infoHash, st := range d.stats {
		out[infoHash] = *st
	}
	return out
}

func (d *Dispatcher) CurrentRate() int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentRate
}

func (d *Dispatcher) tick() {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if d.lastTick.IsZero() {
		d.lastTick = now
	}
	elapsed := now.Sub(d.lastTick)
	d.lastTick = now
	if elapsed > maxTickElapsed {
		elapsed = maxTickElapsed
	}
	if d.currentRate == 0 || now.After(d.nextRefresh) {
		d.refreshCurrentRateLocked(now)
		d.recomputeSpeedsLocked()
	}
	if len(d.stats) == 0 || elapsed <= 0 {
		return
	}
	for _, st := range d.stats {
		st.Uploaded += int64(float64(st.CurrentSpeedBps) * elapsed.Seconds())
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.RandomRefreshSeconds == 0 {
		cfg.RandomRefreshSeconds = 20 * 60
	}
	if cfg.Strategy == "none" {
		cfg.MinRateBps = 0
		cfg.MaxRateBps = 0
		return cfg
	}
	if cfg.MinRateBps == 0 && cfg.MaxRateBps == 0 && cfg.RandomJitterPercent > 0 {
		base := int64(0)
		switch cfg.Strategy {
		case "conservative_rate":
			base = cfg.ConservativeRateBps
		case "configured_rate":
			base = cfg.ConfiguredRateBps
		}
		if base > 0 {
			delta := base * int64(cfg.RandomJitterPercent) / 100
			cfg.MinRateBps = base - delta
			cfg.MaxRateBps = base + delta
		}
	}
	if cfg.MinRateBps < 0 {
		cfg.MinRateBps = 0
	}
	if cfg.MaxRateBps < 0 {
		cfg.MaxRateBps = 0
	}
	if cfg.MaxRateBps > 0 && cfg.MinRateBps > cfg.MaxRateBps {
		cfg.MinRateBps = cfg.MaxRateBps
	}
	return cfg
}

func (d *Dispatcher) refreshCurrentRateLocked(now time.Time) {
	d.nextRefresh = now.Add(d.randomRefresh)
	if d.maxRateBps > 0 {
		minRate := d.minRateBps
		maxRate := d.maxRateBps
		if minRate == 0 {
			minRate = maxRate
		}
		if minRate == maxRate {
			d.currentRate = maxRate
			return
		}
		d.currentRate = minRate + d.rng.Int63n(maxRate-minRate+1)
		return
	}
	if d.baseRateBps == 0 {
		d.currentRate = 0
		return
	}
	d.currentRate = d.baseRateBps
}

func (d *Dispatcher) recomputeSpeedsLocked() {
	if len(d.stats) == 0 || d.currentRate == 0 {
		for _, st := range d.stats {
			st.CurrentSpeedBps = 0
		}
		return
	}
	totalWeight := 0.0
	weights := make(map[string]float64, len(d.stats))
	for infoHash, st := range d.stats {
		weight := peersWeight(st.Seeders, st.Leechers)
		weights[infoHash] = weight
		totalWeight += weight
	}
	if totalWeight == 0 {
		for _, st := range d.stats {
			st.CurrentSpeedBps = 0
		}
		return
	}
	for infoHash, st := range d.stats {
		st.CurrentSpeedBps = int64(float64(d.currentRate) * weights[infoHash] / totalWeight)
	}
}

func peersWeight(seeders, leechers int) float64 {
	if seeders <= 0 && leechers <= 0 {
		return 0.1 // 无 peers 信息时使用较小权重，待实际数据到达后调整
	}

	// 使用局部变量，避免修改输入参数
	s := seeders
	l := leechers
	if s <= 0 {
		s = 1
	}
	if l <= 0 {
		l = 1
	}

	total := s + l
	leechersRatio := float64(l) / float64(total)
	return leechersRatio * 100 * leechersRatio * float64(l)
}
