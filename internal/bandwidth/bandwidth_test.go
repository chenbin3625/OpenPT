package bandwidth

import (
	"testing"
	"time"
)

func TestDispatcherWeightsBandwidthByPeers(t *testing.T) {
	d := New(Config{
		Strategy:          "configured_rate",
		ConfiguredRateBps: 1000,
		MinRateBps:        1000,
		MaxRateBps:        1000,
	})
	d.Register("cold")
	d.Register("hot")
	d.UpdatePeers("cold", 10, 1)
	d.UpdatePeers("hot", 1, 10)
	forceElapsed(d, time.Second)
	d.tick()

	cold := d.Get("cold")
	hot := d.Get("hot")
	if hot.CurrentSpeedBps <= cold.CurrentSpeedBps {
		t.Fatalf("hot speed = %d, cold speed = %d", hot.CurrentSpeedBps, cold.CurrentSpeedBps)
	}
	if hot.Uploaded <= cold.Uploaded {
		t.Fatalf("hot uploaded = %d, cold uploaded = %d", hot.Uploaded, cold.Uploaded)
	}
}

func TestDispatcherRandomRateStaysInConfiguredRange(t *testing.T) {
	d := New(Config{
		Strategy:          "configured_rate",
		ConfiguredRateBps: 1000,
		MinRateBps:        700,
		MaxRateBps:        900,
	})
	for i := 0; i < 20; i++ {
		d.mu.Lock()
		d.refreshCurrentRateLocked(d.nextRefresh)
		got := d.currentRate
		d.mu.Unlock()
		if got < 700 || got > 900 {
			t.Fatalf("current rate = %d, want in 700..900", got)
		}
	}
}

func TestDispatcherRandomRateDoesNotRequireBaseRate(t *testing.T) {
	d := New(Config{
		Strategy:   "configured_rate",
		MinRateBps: 700,
		MaxRateBps: 900,
	})
	got := d.CurrentRate()
	if got < 700 || got > 900 {
		t.Fatalf("current rate = %d, want in 700..900", got)
	}
}

func TestStrategyNoneIgnoresResidualRatesAndJitter(t *testing.T) {
	d := New(Config{
		Strategy:            "none",
		ConfiguredRateBps:   1000,
		MinRateBps:          700,
		MaxRateBps:          900,
		RandomJitterPercent: 10,
	})
	d.Register("hash")
	forceElapsed(d, time.Second)
	d.tick()

	if got := d.CurrentRate(); got != 0 {
		t.Fatalf("current rate = %d, want 0 for strategy none", got)
	}
	st := d.Get("hash")
	if st.CurrentSpeedBps != 0 || st.Uploaded != 0 {
		t.Fatalf("stats with strategy none = %+v, want no uploaded/speed", st)
	}

	d.UpdateConfig(Config{
		Strategy:            "none",
		ConfiguredRateBps:   2000,
		RandomJitterPercent: 50,
	})
	if got := d.CurrentRate(); got != 0 {
		t.Fatalf("current rate after update = %d, want 0 for strategy none", got)
	}
}

func TestPeersWeightWithZeroPeers(t *testing.T) {
	// 验证 seeders=0, leechers=0 时返回默认权重而非 0
	weight := peersWeight(0, 0)
	if weight <= 0 {
		t.Fatalf("peersWeight(0, 0) = %f, want > 0", weight)
	}

	// 验证带宽分配在无 peers 信息时不会归零
	d := New(Config{
		Strategy:          "configured_rate",
		ConfiguredRateBps: 1000,
		MinRateBps:        1000,
		MaxRateBps:        1000,
	})
	d.Register("no-peers")
	forceElapsed(d, time.Second)
	d.tick()

	st := d.Get("no-peers")
	if st.CurrentSpeedBps <= 0 {
		t.Fatalf("CurrentSpeedBps = %d with no peers info, want > 0", st.CurrentSpeedBps)
	}
}

func TestDispatcherAccumulatesUploadedByElapsedTime(t *testing.T) {
	d := New(Config{
		Strategy:          "configured_rate",
		ConfiguredRateBps: 1000,
		MinRateBps:        1000,
		MaxRateBps:        1000,
	})
	d.Register("hash")
	forceElapsed(d, 2*time.Second)
	d.tick()

	if got := d.Get("hash").Uploaded; got < 1900 || got > 2100 {
		t.Fatalf("uploaded = %d, want about 2000", got)
	}
}

func forceElapsed(d *Dispatcher, elapsed time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastTick = time.Now().Add(-elapsed)
}

// TestTickCapsUploadedAfterSuspend 验证进程挂起恢复后不会一次性累加巨额上传量。
// elapsed=1h 应被截断到 maxTickElapsed(2s)，避免产生不自然的上传尖峰。
func TestTickCapsUploadedAfterSuspend(t *testing.T) {
	d := New(Config{
		Strategy:          "configured_rate",
		ConfiguredRateBps: 1000,
		MinRateBps:        1000,
		MaxRateBps:        1000,
	})
	d.Register("hash")
	// 模拟进程挂起 1 小时后恢复
	forceElapsed(d, time.Hour)
	d.tick()

	st := d.Get("hash")
	// 1000 Bps * 2s = 2000，而非 1000 * 3600 = 3,600,000
	if st.Uploaded > 2100 || st.Uploaded < 1900 {
		t.Fatalf("uploaded = %d after suspend, want about 2000 (capped at 2s)", st.Uploaded)
	}
}
