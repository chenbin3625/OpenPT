package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"openpt/internal/bandwidth"
	"openpt/internal/clientemu"
	"openpt/internal/config"
	"openpt/internal/scheduler"
	"openpt/internal/store"
	"openpt/internal/tracker"
	"openpt/internal/web"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.toml", "path to OpenPT config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("openpt %s (%s, %s)\n", version, commit, date)
		return
	}

	bootstrapLog := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(*configPath)
	if err != nil {
		bootstrapLog.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}
	log, closeLog, err := newLogger(cfg)
	if err != nil {
		bootstrapLog.Error("failed to configure logger", "error", err)
		os.Exit(1)
	}
	defer closeLog()
	log.Info("config loaded", "path", *configPath, "torrents_dir", cfg.TorrentsDir, "clients_dir", cfg.ClientsDir, "client", cfg.Client)
	log.Info("some config changes require restart", "restart_required", "client file, torrents_dir, clients_dir, archive_dir, state_file, scan_interval_seconds, logging.file, metrics.enabled, metrics.listen, metrics.path, metrics.webui")

	emu, err := clientemu.LoadClient(filepath.Join(cfg.ClientsDir, cfg.Client))
	if err != nil {
		log.Error("failed to load client emulation", "client", cfg.Client, "error", err)
		os.Exit(1)
	}
	log.Info("client emulation loaded", "client", cfg.Client, "headers", len(emu.HeadersForRequest()))

	trackerClient, err := tracker.New(trackerOptions(cfg), log)
	if err != nil {
		log.Error("failed to configure tracker client", "error", err)
		os.Exit(1)
	}
	bw := bandwidth.New(bandwidthConfig(cfg))
	bw.Start()
	defer bw.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := store.NewWithScanInterval(ctx, cfg.TorrentsDir, cfg.ArchiveDir, cfg.ScanInterval(), log)
	if err := st.Start(ctx); err != nil {
		log.Error("failed to start torrent store", "error", err)
		os.Exit(1)
	}

	s := scheduler.New(cfg, emu, trackerClient, bw, st, log)
	s.Start(ctx)
	metricsServer, err := startMetricsServer(cfg, bw, s, st, log)
	if err != nil {
		log.Error("failed to start metrics server", "listen", cfg.Metrics.Listen, "error", err)
		os.Exit(1)
	}
	if metricsServer != nil {
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownStopTimeout())
			defer shutdownCancel()
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				log.Warn("metrics server shutdown timed out or failed", "error", err)
			}
		}()
	}

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		received := <-sig
		if received != syscall.SIGHUP {
			break
		}
		nextCfg, err := config.Load(*configPath)
		if err != nil {
			log.Warn("failed to reload config", "path", *configPath, "error", err)
			continue
		}
		nextCfg = preserveRuntimeConfig(cfg, nextCfg)
		if err := trackerClient.Configure(trackerOptions(nextCfg)); err != nil {
			log.Warn("failed to reload tracker config", "error", err)
			continue
		}
		bw.UpdateConfig(bandwidthConfig(nextCfg))
		s.UpdateConfig(nextCfg)
		s.Reconcile(ctx)
		cfg = nextCfg
		log.Info("config reloaded", "path", *configPath, "hot_reloaded", "announce, tracker, bandwidth, scheduler, simultaneous_seed, ratio target", "restart_required", "client file, torrents_dir, clients_dir, archive_dir, state_file, scan_interval_seconds, logging.file, metrics.enabled, metrics.listen, metrics.path, metrics.webui")
	}
	log.Info("shutdown requested; sending stopped announces")
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), cfg.ShutdownStopTimeout())
	defer stopCancel()
	s.Stop(stopCtx)
	log.Info("OpenPT stopped")
}

func preserveRuntimeConfig(current, next config.Config) config.Config {
	if current.UsesRandomAnnouncePort() && next.UsesRandomAnnouncePort() {
		next.Announce.Port = current.Announce.Port
	}
	return next
}

func newLogger(cfg config.Config) (*slog.Logger, func(), error) {
	if cfg.Logging.File == "" {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})), func() {}, nil
	}
	f, err := os.OpenFile(cfg.Logging.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	writer := io.MultiWriter(os.Stdout, f)
	return slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})), func() { _ = f.Close() }, nil
}

func bandwidthConfig(cfg config.Config) bandwidth.Config {
	return bandwidth.Config{
		Strategy:             cfg.Uploaded.Strategy,
		ConservativeRateBps:  cfg.Uploaded.ConservativeRateBps,
		ConfiguredRateBps:    cfg.Uploaded.ConfiguredRateBps,
		MinRateBps:           cfg.Uploaded.MinRateBps,
		MaxRateBps:           cfg.Uploaded.MaxRateBps,
		RandomJitterPercent:  cfg.Uploaded.RandomJitterPercent,
		RandomRefreshSeconds: cfg.Uploaded.RandomRefreshSeconds,
	}
}

func trackerOptions(cfg config.Config) tracker.Options {
	return tracker.Options{
		Timeout:             cfg.TrackerTimeout(),
		Proxy:               cfg.Tracker.Proxy,
		ReuseConnections:    cfg.TrackerReuseConnections(),
		MaxIdleConns:        cfg.Tracker.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.Tracker.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.TrackerIdleConnTimeout(),
	}
}

func startMetricsServer(cfg config.Config, bw *bandwidth.Dispatcher, s *scheduler.Scheduler, st *store.Store, log *slog.Logger) (*http.Server, error) {
	if !cfg.Metrics.Enabled {
		return nil, nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, "ok\n")
		}
	})
	mux.HandleFunc(cfg.Metrics.Path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintln(w, "# HELP openpt_bandwidth_current_rate_bps Current configured synthetic upload bandwidth in bytes per second.")
		fmt.Fprintln(w, "# TYPE openpt_bandwidth_current_rate_bps gauge")
		fmt.Fprintf(w, "openpt_bandwidth_current_rate_bps %d\n", bw.CurrentRate())
		fmt.Fprintln(w, "# HELP openpt_active_torrents Number of currently active torrents.")
		fmt.Fprintln(w, "# TYPE openpt_active_torrents gauge")
		fmt.Fprintf(w, "openpt_active_torrents %d\n", s.ActiveCount())
		fmt.Fprintln(w, "# HELP openpt_torrent_uploaded_bytes Total synthetic uploaded bytes reported per torrent.")
		fmt.Fprintln(w, "# TYPE openpt_torrent_uploaded_bytes counter")
		fmt.Fprintln(w, "# HELP openpt_torrent_speed_bps Current synthetic upload speed in bytes per second per torrent.")
		fmt.Fprintln(w, "# TYPE openpt_torrent_speed_bps gauge")
		fmt.Fprintln(w, "# HELP openpt_torrent_seeders Last tracker seeder count per torrent.")
		fmt.Fprintln(w, "# TYPE openpt_torrent_seeders gauge")
		fmt.Fprintln(w, "# HELP openpt_torrent_leechers Last tracker leecher count per torrent.")
		fmt.Fprintln(w, "# TYPE openpt_torrent_leechers gauge")
		for infoHash, st := range bw.Snapshot() {
			fmt.Fprintf(w, "openpt_torrent_uploaded_bytes{info_hash=%q} %d\n", infoHash, st.Uploaded)
			fmt.Fprintf(w, "openpt_torrent_speed_bps{info_hash=%q} %d\n", infoHash, st.CurrentSpeedBps)
			fmt.Fprintf(w, "openpt_torrent_seeders{info_hash=%q} %d\n", infoHash, st.Seeders)
			fmt.Fprintf(w, "openpt_torrent_leechers{info_hash=%q} %d\n", infoHash, st.Leechers)
		}
	})

	if cfg.Metrics.WebUI {
		webHandler := web.New(st, s, bw)
		webHandler.RegisterRoutes(mux)
		log.Info("web UI enabled", "url", "http://"+cfg.Metrics.Listen+"/")
	}

	ln, err := net.Listen("tcp", cfg.Metrics.Listen)
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Addr:              ln.Addr().String(),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		log.Info("metrics server started", "listen", server.Addr, "path", cfg.Metrics.Path)
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Warn("metrics server stopped with error", "error", err)
		}
	}()
	return server, nil
}
