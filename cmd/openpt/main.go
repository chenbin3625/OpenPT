package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"openpt/internal/bandwidth"
	"openpt/internal/clientemu"
	"openpt/internal/config"
	"openpt/internal/scheduler"
	"openpt/internal/store"
	"openpt/internal/tracker"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "config.json", "path to OpenPT config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("openpt %s (%s, %s)\n", version, commit, date)
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}
	log.Info("config loaded", "path", *configPath, "torrents_dir", cfg.TorrentsDir, "clients_dir", cfg.ClientsDir, "client", cfg.Client)

	emu, err := clientemu.LoadClient(filepath.Join(cfg.ClientsDir, cfg.Client))
	if err != nil {
		log.Error("failed to load client emulation", "client", cfg.Client, "error", err)
		os.Exit(1)
	}
	log.Info("client emulation loaded", "client", cfg.Client, "headers", len(emu.HeadersForRequest()))

	trackerClient, err := tracker.New(cfg.TrackerTimeout(), cfg.Tracker.Proxy, log)
	if err != nil {
		log.Error("failed to configure tracker client", "error", err)
		os.Exit(1)
	}
	bw := bandwidth.New(cfg.Uploaded.Strategy, cfg.Uploaded.ConservativeRateBps, cfg.Uploaded.ConfiguredRateBps)
	bw.Start()
	defer bw.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := store.New(cfg.TorrentsDir, cfg.ArchiveDir, log)
	if err := st.Start(ctx); err != nil {
		log.Error("failed to start torrent store", "error", err)
		os.Exit(1)
	}

	s := scheduler.New(cfg, emu, trackerClient, bw, st, log)
	s.Start(ctx)

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutdown requested; sending stopped announces")
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), cfg.ShutdownStopTimeout())
	defer stopCancel()
	s.Stop(stopCtx)
	log.Info("OpenPT stopped")
}
