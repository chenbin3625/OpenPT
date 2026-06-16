package bandwidth

import "testing"

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
	d.tick()

	st := d.Get("no-peers")
	if st.CurrentSpeedBps <= 0 {
		t.Fatalf("CurrentSpeedBps = %d with no peers info, want > 0", st.CurrentSpeedBps)
	}
}
