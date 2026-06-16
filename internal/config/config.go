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
	TimeoutSeconds int    `json:"timeout_seconds"`
	Proxy          string `json:"proxy"`
}

type UploadedConfig struct {
	Strategy            string `json:"strategy"`
	ConservativeRateBps int64  `json:"conservative_rate_bps"`
	ConfiguredRateBps   int64  `json:"configured_rate_bps"`
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
		SimultaneousSeed            int   `json:"simultaneousSeed"`
		KeepTorrentWithZeroLeechers *bool `json:"keepTorrentWithZeroLeechers"`
		MinUploadRate               int64 `json:"minUploadRate"`
		MaxUploadRate               int64 `json:"maxUploadRate"`
	}
	_ = json.Unmarshal(data, &legacy)
	cfg.legacySimultaneousSeed = legacy.SimultaneousSeed
	cfg.legacyKeepTorrentZeroLeechers = legacy.KeepTorrentWithZeroLeechers
	if cfg.SimultaneousSeed == 0 {
		cfg.SimultaneousSeed = cfg.legacySimultaneousSeed
	}
	if cfg.legacyKeepTorrentZeroLeechers != nil {
		cfg.KeepTorrentWithZeroLeechers = *cfg.legacyKeepTorrentZeroLeechers
	}
	if cfg.Uploaded.Strategy == "" && legacy.MaxUploadRate > 0 {
		cfg.Uploaded.Strategy = "configured_rate"
		cfg.Uploaded.ConfiguredRateBps = legacy.MaxUploadRate * 1000
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
	if c.MaxConsecutiveFailures == 0 {
		c.MaxConsecutiveFailures = 5
	}
	if c.Uploaded.Strategy == "" {
		c.Uploaded.Strategy = "none"
	}
	if c.Uploaded.ConservativeRateBps == 0 {
		c.Uploaded.ConservativeRateBps = 1024
	}
	if c.ScanIntervalSeconds == 0 {
		c.ScanIntervalSeconds = 5
	}
	if c.ShutdownStopTimeoutSeconds == 0 {
		c.ShutdownStopTimeoutSeconds = 20
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
	return nil
}

func (c Config) TrackerTimeout() time.Duration {
	return time.Duration(c.Tracker.TimeoutSeconds) * time.Second
}

func (c Config) ShutdownStopTimeout() time.Duration {
	return time.Duration(c.ShutdownStopTimeoutSeconds) * time.Second
}
