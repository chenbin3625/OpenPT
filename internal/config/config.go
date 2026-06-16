package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	TorrentsDir                   string         `json:"torrents_dir"`
	ArchiveDir                    string         `json:"archive_dir"`
	ClientsDir                    string         `json:"clients_dir"`
	Client                        string         `json:"client"`
	SimultaneousSeed              int            `json:"simultaneous_seed"`
	KeepTorrentWithZeroLeechers   bool           `json:"keep_torrent_with_zero_leechers"`
	Announce                      AnnounceConfig `json:"announce"`
	Tracker                       TrackerConfig  `json:"tracker"`
	Logging                       LoggingConfig  `json:"logging"`
	Metrics                       MetricsConfig  `json:"metrics"`
	MaxConsecutiveFailures        int            `json:"max_consecutive_failures"`
	Uploaded                      UploadedConfig `json:"uploaded"`
	ScanIntervalSeconds           int            `json:"scan_interval_seconds"`
	ShutdownStopTimeoutSeconds    int            `json:"shutdown_stop_timeout_seconds"`
	legacySimultaneousSeed        int
	legacyKeepTorrentZeroLeechers *bool
}

type AnnounceConfig struct {
	Port int    `json:"port"`
	IP   string `json:"ip"`
	IPv6 string `json:"ipv6"`
}

type TrackerConfig struct {
	TimeoutSeconds           int    `json:"timeout_seconds"`
	Proxy                    string `json:"proxy"`
	ReuseConnections         *bool  `json:"reuse_connections"`
	MaxIdleConns             int    `json:"max_idle_conns"`
	MaxIdleConnsPerHost      int    `json:"max_idle_conns_per_host"`
	IdleConnTimeoutSeconds   int    `json:"idle_conn_timeout_seconds"`
	FailureBackoffMinSeconds int    `json:"failure_backoff_min_seconds"`
	FailureBackoffMaxSeconds int    `json:"failure_backoff_max_seconds"`
}

type UploadedConfig struct {
	Strategy             string  `json:"strategy"`
	ConservativeRateBps  int64   `json:"conservative_rate_bps"`
	ConfiguredRateBps    int64   `json:"configured_rate_bps"`
	MinRateBps           int64   `json:"min_rate_bps"`
	MaxRateBps           int64   `json:"max_rate_bps"`
	RandomJitterPercent  int     `json:"random_jitter_percent"`
	RandomRefreshSeconds int     `json:"random_refresh_seconds"`
	RatioTarget          float64 `json:"ratio_target"`
}

type LoggingConfig struct {
	File string `json:"file"`
}

type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	Listen  string `json:"listen"`
	Path    string `json:"path"`
	WebUI   bool   `json:"webui"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	var legacy struct {
		SimultaneousSeed            int      `json:"simultaneousSeed"`
		KeepTorrentWithZeroLeechers *bool    `json:"keepTorrentWithZeroLeechers"`
		MinUploadRate               int64    `json:"minUploadRate"`
		MaxUploadRate               int64    `json:"maxUploadRate"`
		UploadRatioTarget           *float64 `json:"uploadRatioTarget"`
	}
	var modern struct {
		Uploaded struct {
			RatioTarget *float64 `json:"ratio_target"`
		} `json:"uploaded"`
	}
	_ = json.Unmarshal(data, &legacy)
	_ = json.Unmarshal(data, &modern)
	cfg.legacySimultaneousSeed = legacy.SimultaneousSeed
	cfg.legacyKeepTorrentZeroLeechers = legacy.KeepTorrentWithZeroLeechers
	if cfg.SimultaneousSeed == 0 {
		cfg.SimultaneousSeed = cfg.legacySimultaneousSeed
	}
	if cfg.legacyKeepTorrentZeroLeechers != nil {
		cfg.KeepTorrentWithZeroLeechers = *cfg.legacyKeepTorrentZeroLeechers
	}
	if legacy.MinUploadRate > 0 && cfg.Uploaded.MinRateBps == 0 {
		cfg.Uploaded.MinRateBps = legacy.MinUploadRate * 1000
	}
	if legacy.MaxUploadRate > 0 && cfg.Uploaded.MaxRateBps == 0 {
		cfg.Uploaded.MaxRateBps = legacy.MaxUploadRate * 1000
	}
	if cfg.Uploaded.Strategy == "" && (legacy.MaxUploadRate > 0 || legacy.MinUploadRate > 0) {
		cfg.Uploaded.Strategy = "configured_rate"
		if legacy.MaxUploadRate > 0 {
			cfg.Uploaded.ConfiguredRateBps = legacy.MaxUploadRate * 1000
		} else {
			cfg.Uploaded.ConfiguredRateBps = legacy.MinUploadRate * 1000
		}
	}
	if legacy.UploadRatioTarget != nil && modern.Uploaded.RatioTarget == nil {
		cfg.Uploaded.RatioTarget = *legacy.UploadRatioTarget
		if cfg.Uploaded.RatioTarget == -1 {
			cfg.Uploaded.RatioTarget = 0
		}
	}
	cfg.applyDefaults(path)
	return cfg, cfg.Validate()
}

func (c *Config) applyDefaults(configPath string) {
	root := filepath.Dir(configPath)
	if c.TorrentsDir == "" {
		c.TorrentsDir = filepath.Join(root, "torrents")
	}
	if c.ArchiveDir == "" {
		c.ArchiveDir = filepath.Join(c.TorrentsDir, "archived")
	}
	if c.ClientsDir == "" {
		c.ClientsDir = filepath.Join(root, "clients")
	}
	if c.SimultaneousSeed == 0 {
		c.SimultaneousSeed = 1
	}
	if c.Announce.Port == 0 {
		c.Announce.Port = 6881
	}
	if c.Tracker.TimeoutSeconds == 0 {
		c.Tracker.TimeoutSeconds = 15
	}
	if c.Tracker.ReuseConnections == nil {
		reuseConnections := true
		c.Tracker.ReuseConnections = &reuseConnections
	}
	if c.Tracker.MaxIdleConns == 0 {
		c.Tracker.MaxIdleConns = 100
	}
	if c.Tracker.MaxIdleConnsPerHost == 0 {
		c.Tracker.MaxIdleConnsPerHost = 10
	}
	if c.Tracker.IdleConnTimeoutSeconds == 0 {
		c.Tracker.IdleConnTimeoutSeconds = 90
	}
	if c.Tracker.FailureBackoffMinSeconds == 0 {
		c.Tracker.FailureBackoffMinSeconds = 5
	}
	if c.Tracker.FailureBackoffMaxSeconds == 0 {
		c.Tracker.FailureBackoffMaxSeconds = 300
	}
	if c.MaxConsecutiveFailures == 0 {
		c.MaxConsecutiveFailures = 5
	}
	if c.Uploaded.Strategy == "" {
		c.Uploaded.Strategy = "none"
	}
	if c.Uploaded.ConservativeRateBps == 0 {
		c.Uploaded.ConservativeRateBps = 1024
	}
	if c.Uploaded.RandomRefreshSeconds == 0 {
		c.Uploaded.RandomRefreshSeconds = 20 * 60
	}
	if c.ScanIntervalSeconds == 0 {
		c.ScanIntervalSeconds = 5
	}
	if c.ShutdownStopTimeoutSeconds == 0 {
		c.ShutdownStopTimeoutSeconds = 20
	}
	if c.Metrics.Listen == "" {
		c.Metrics.Listen = "127.0.0.1:9090"
	}
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
	if !c.Metrics.Enabled {
		c.Metrics.WebUI = false
	}
}

func (c Config) Validate() error {
	if c.Client == "" {
		return errors.New("client is required")
	}
	if c.SimultaneousSeed < 1 {
		return errors.New("simultaneous_seed must be at least 1")
	}
	if c.Announce.Port < 1 || c.Announce.Port > 65535 {
		return errors.New("announce.port must be in 1..65535")
	}
	if c.Tracker.TimeoutSeconds < 1 {
		return errors.New("tracker.timeout_seconds must be at least 1")
	}
	if c.Tracker.MaxIdleConns < 1 {
		return errors.New("tracker.max_idle_conns must be at least 1")
	}
	if c.Tracker.MaxIdleConnsPerHost < 1 {
		return errors.New("tracker.max_idle_conns_per_host must be at least 1")
	}
	if c.Tracker.IdleConnTimeoutSeconds < 1 {
		return errors.New("tracker.idle_conn_timeout_seconds must be at least 1")
	}
	if c.Tracker.FailureBackoffMinSeconds < 1 {
		return errors.New("tracker.failure_backoff_min_seconds must be at least 1")
	}
	if c.Tracker.FailureBackoffMaxSeconds < c.Tracker.FailureBackoffMinSeconds {
		return errors.New("tracker.failure_backoff_max_seconds must be greater than or equal to tracker.failure_backoff_min_seconds")
	}
	if c.MaxConsecutiveFailures < 1 {
		return errors.New("max_consecutive_failures must be at least 1")
	}
	switch c.Uploaded.Strategy {
	case "none", "conservative_rate", "configured_rate":
	default:
		return fmt.Errorf("uploaded.strategy must be none, conservative_rate, or configured_rate")
	}
	if c.Uploaded.Strategy == "configured_rate" && c.Uploaded.ConfiguredRateBps < 0 {
		return errors.New("uploaded.configured_rate_bps must not be negative")
	}
	if c.Uploaded.MinRateBps < 0 || c.Uploaded.MaxRateBps < 0 {
		return errors.New("uploaded.min_rate_bps and uploaded.max_rate_bps must not be negative")
	}
	if c.Uploaded.MaxRateBps > 0 && c.Uploaded.MinRateBps > c.Uploaded.MaxRateBps {
		return errors.New("uploaded.min_rate_bps must be less than or equal to uploaded.max_rate_bps")
	}
	if c.Uploaded.RandomJitterPercent < 0 || c.Uploaded.RandomJitterPercent > 100 {
		return errors.New("uploaded.random_jitter_percent must be in 0..100")
	}
	if c.Uploaded.RandomRefreshSeconds < 1 {
		return errors.New("uploaded.random_refresh_seconds must be at least 1")
	}
	if c.Uploaded.RatioTarget < 0 {
		return errors.New("uploaded.ratio_target must not be negative")
	}
	return nil
}

func (c Config) TrackerTimeout() time.Duration {
	return time.Duration(c.Tracker.TimeoutSeconds) * time.Second
}

func (c Config) TrackerIdleConnTimeout() time.Duration {
	return time.Duration(c.Tracker.IdleConnTimeoutSeconds) * time.Second
}

func (c Config) TrackerReuseConnections() bool {
	return c.Tracker.ReuseConnections == nil || *c.Tracker.ReuseConnections
}

func (c Config) TrackerFailureBackoffMin() time.Duration {
	return time.Duration(c.Tracker.FailureBackoffMinSeconds) * time.Second
}

func (c Config) TrackerFailureBackoffMax() time.Duration {
	return time.Duration(c.Tracker.FailureBackoffMaxSeconds) * time.Second
}

func (c Config) UploadedRandomRefresh() time.Duration {
	return time.Duration(c.Uploaded.RandomRefreshSeconds) * time.Second
}

func (c Config) ShutdownStopTimeout() time.Duration {
	return time.Duration(c.ShutdownStopTimeoutSeconds) * time.Second
}
