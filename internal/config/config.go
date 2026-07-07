package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	TorrentsDir                string         `toml:"torrents_dir"`
	ArchiveDir                 string         `toml:"archive_dir"`
	ClientsDir                 string         `toml:"clients_dir"`
	StateFile                  string         `toml:"state_file"`
	Client                     string         `toml:"client"`
	SimultaneousSeed           int            `toml:"simultaneous_seed"`
	Announce                   AnnounceConfig `toml:"announce"`
	Tracker                    TrackerConfig  `toml:"tracker"`
	Logging                    LoggingConfig  `toml:"logging"`
	Metrics                    MetricsConfig  `toml:"metrics"`
	Uploaded                   UploadedConfig `toml:"uploaded"`
	ScanIntervalSeconds        int            `toml:"scan_interval_seconds"`
	ShutdownStopTimeoutSeconds int            `toml:"shutdown_stop_timeout_seconds"`
}

type AnnounceConfig struct {
	Port int    `toml:"port"`
	IP   string `toml:"ip"`
	IPv6 string `toml:"ipv6"`
}

const (
	randomAnnouncePortMin = 49152
	randomAnnouncePortMax = 65535
)

type TrackerConfig struct {
	TimeoutSeconds           int    `toml:"timeout_seconds"`
	Proxy                    string `toml:"proxy"`
	ReuseConnections         *bool  `toml:"reuse_connections"`
	MaxIdleConns             int    `toml:"max_idle_conns"`
	MaxIdleConnsPerHost      int    `toml:"max_idle_conns_per_host"`
	IdleConnTimeoutSeconds   int    `toml:"idle_conn_timeout_seconds"`
	FailureBackoffMinSeconds int    `toml:"failure_backoff_min_seconds"`
	FailureBackoffMaxSeconds int    `toml:"failure_backoff_max_seconds"`
}

type UploadedConfig struct {
	Strategy             string  `toml:"strategy"`
	ConservativeRateBps  int64   `toml:"conservative_rate_bps"`
	ConfiguredRateBps    int64   `toml:"configured_rate_bps"`
	MinRateBps           int64   `toml:"min_rate_bps"`
	MaxRateBps           int64   `toml:"max_rate_bps"`
	RandomJitterPercent  int     `toml:"random_jitter_percent"`
	RandomRefreshSeconds int     `toml:"random_refresh_seconds"`
	RatioTarget          float64 `toml:"ratio_target"`
}

type LoggingConfig struct {
	File string `toml:"file"`
}

type MetricsConfig struct {
	Enabled bool   `toml:"enabled"`
	Listen  string `toml:"listen"`
	Path    string `toml:"path"`
	WebUI   bool   `toml:"webui"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	// 根据文件扩展名决定解析方式
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".toml" {
		md, err := toml.Decode(string(data), &cfg)
		if err != nil {
			return Config{}, err
		}
		if undecoded := md.Undecoded(); len(undecoded) > 0 {
			return Config{}, fmt.Errorf("unknown config field %q", undecoded[0].String())
		}
	} else {
		// 为了向后兼容，仍然支持 JSON 格式（但会输出警告）
		return Config{}, fmt.Errorf("JSON config format is deprecated, please migrate to TOML format (config.toml)")
	}

	cfg.applyDefaults(path)
	return cfg, cfg.Validate()
}

func (c *Config) applyDefaults(configPath string) {
	root, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		root = filepath.Dir(configPath)
	}
	if c.TorrentsDir == "" {
		c.TorrentsDir = filepath.Join(root, "torrents")
	} else {
		c.TorrentsDir = resolveConfigPath(root, c.TorrentsDir)
	}
	if c.ArchiveDir == "" {
		// 默认归档到种子目录的同级目录，便于整理加载失败的种子
		c.ArchiveDir = filepath.Join(filepath.Dir(c.TorrentsDir), "torrents_archive")
	} else {
		c.ArchiveDir = resolveConfigPath(root, c.ArchiveDir)
	}
	if c.ClientsDir == "" {
		c.ClientsDir = filepath.Join(root, "clients")
	} else {
		c.ClientsDir = resolveConfigPath(root, c.ClientsDir)
	}
	if c.StateFile == "" {
		c.StateFile = filepath.Join(root, "openpt_state.json")
	} else {
		c.StateFile = resolveConfigPath(root, c.StateFile)
	}
	if c.Logging.File != "" {
		c.Logging.File = resolveConfigPath(root, c.Logging.File)
	}
	// simultaneous_seed 可以为 0（无限制，全量加载），负数交给 Validate 报错。
	if c.Announce.Port == 0 {
		c.Announce.Port = randomAnnouncePort()
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

func resolveConfigPath(root, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func randomAnnouncePort() int {
	span := randomAnnouncePortMax - randomAnnouncePortMin + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(span)))
	if err != nil {
		// 应急回退：混合时间戳和进程 ID 提升不可预测性
		seed := time.Now().UnixNano() ^ int64(os.Getpid())<<16
		return randomAnnouncePortMin + int(seed%int64(span))
	}
	return randomAnnouncePortMin + int(n.Int64())
}

func (c Config) Validate() error {
	if c.Client == "" {
		return errors.New("client is required")
	}
	if c.SimultaneousSeed < 0 {
		return errors.New("simultaneous_seed must not be negative")
	}
	if err := validateArchiveDir(c.TorrentsDir, c.ArchiveDir); err != nil {
		return err
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
	switch c.Uploaded.Strategy {
	case "none", "conservative_rate", "configured_rate":
	default:
		return fmt.Errorf("uploaded.strategy must be none, conservative_rate, or configured_rate")
	}
	if c.Uploaded.ConservativeRateBps < 0 {
		return errors.New("uploaded.conservative_rate_bps must not be negative")
	}
	if c.Uploaded.Strategy == "conservative_rate" && c.Uploaded.ConservativeRateBps <= 0 {
		return errors.New("uploaded.conservative_rate_bps must be greater than 0 when uploaded.strategy is conservative_rate")
	}
	if c.Uploaded.Strategy == "configured_rate" && c.Uploaded.ConfiguredRateBps <= 0 {
		return errors.New("uploaded.configured_rate_bps must be greater than 0 when uploaded.strategy is configured_rate")
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
	if c.ScanIntervalSeconds < 1 {
		return errors.New("scan_interval_seconds must be at least 1")
	}
	if err := validateProxy(c.Tracker.Proxy); err != nil {
		return err
	}
	if c.Metrics.Enabled {
		if err := validateListenAddress(c.Metrics.Listen); err != nil {
			return err
		}
		if !strings.HasPrefix(c.Metrics.Path, "/") {
			return errors.New("metrics.path must start with /")
		}
		if c.Metrics.WebUI {
			switch c.Metrics.Path {
			case "/", "/api/status", "/api/config", "/api/events":
				return fmt.Errorf("metrics.path %q conflicts with web UI routes", c.Metrics.Path)
			}
		}
	}
	return nil
}

func validateProxy(proxy string) error {
	if proxy == "" {
		return nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("tracker.proxy must be a valid URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5":
		return nil
	default:
		return errors.New("tracker.proxy scheme must be http, https, or socks5")
	}
}

func validateListenAddress(listen string) error {
	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return errors.New("metrics.listen must be in host:port form")
	}
	if strings.ContainsAny(host, "/ \t\r\n") {
		return errors.New("metrics.listen host must not contain whitespace or slash")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("metrics.listen port must be in 1..65535")
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
	// 防御性 nil 检查：未经 applyDefaults 直接构造的配置默认为 true
	if c.Tracker.ReuseConnections == nil {
		return true
	}
	return *c.Tracker.ReuseConnections
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

func (c Config) ScanInterval() time.Duration {
	return time.Duration(c.ScanIntervalSeconds) * time.Second
}

func (c Config) ShutdownStopTimeout() time.Duration {
	return time.Duration(c.ShutdownStopTimeoutSeconds) * time.Second
}

// validateArchiveDir 确保 archive_dir 不与 torrents_dir 相同或位于其内部，
// 避免归档的种子被重新扫描导致循环。
func validateArchiveDir(torrentsDir, archiveDir string) error {
	if archiveDir == "" {
		return nil
	}
	td, err := filepath.Abs(filepath.Clean(torrentsDir))
	if err != nil {
		return err
	}
	ad, err := filepath.Abs(filepath.Clean(archiveDir))
	if err != nil {
		return err
	}
	if ad == td {
		return errors.New("archive_dir must not be the same as torrents_dir")
	}
	rel, err := filepath.Rel(td, ad)
	if err != nil {
		return err
	}
	// rel 是 "." 或不以 ".." 路径段开头时，archive 位于 torrents 内部。
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("archive_dir must not be inside torrents_dir")
	}
	return nil
}
