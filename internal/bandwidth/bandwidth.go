package bandwidth

import (
	"sync"
	"time"
)

type Stats struct {
	Uploaded   int64
	Downloaded int64
	Left       int64
}

type Dispatcher struct {
	mu       sync.RWMutex
	strategy string
	rateBps  int64
	stats    map[string]*Stats
	stop     chan struct{}
}

func New(strategy string, conservativeRateBps, configuredRateBps int64) *Dispatcher {
	rate := int64(0)
	switch strategy {
	case "conservative_rate":
		rate = conservativeRateBps
	case "configured_rate":
		rate = configuredRateBps
	}
	if rate < 0 {
		rate = 0
	}
	return &Dispatcher{strategy: strategy, rateBps: rate, stats: map[string]*Stats{}, stop: make(chan struct{})}
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

func (d *Dispatcher) Stop() { close(d.stop) }

func (d *Dispatcher) Register(infoHash string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.stats[infoHash]; !ok {
		d.stats[infoHash] = &Stats{Left: 0}
	}
}

func (d *Dispatcher) Unregister(infoHash string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.stats, infoHash)
}

func (d *Dispatcher) Get(infoHash string) Stats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if st, ok := d.stats[infoHash]; ok {
		return *st
	}
	return Stats{Left: 0}
}

func (d *Dispatcher) tick() {
	if d.rateBps == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.stats) == 0 {
		return
	}
	perTorrent := d.rateBps / int64(len(d.stats))
	for _, st := range d.stats {
		st.Uploaded += perTorrent
	}
}
