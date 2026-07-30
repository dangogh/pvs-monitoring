package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dangogh/pvs-monitoring/config"
	"github.com/dangogh/pvs-monitoring/internal/version"
	"github.com/dangogh/pvs-monitoring/pvs"
	"github.com/dangogh/pvs-monitoring/store/sqlite"
)

func defaultDBPath() string {
	home, _ := os.UserHomeDir()
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = home + "/.local/share"
	}
	return base + "/pvs-monitor/readings.db"
}

type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(os.Args[1:], os.Stderr, ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "pvs-monitor: %v\n", err)
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}
		os.Exit(1)
	}
}

func run(args []string, logOut io.Writer, ctx context.Context) error {
	fs := flag.NewFlagSet("pvs-monitor", flag.ContinueOnError)
	var cfgPath, addr, dbPath string
	var verbose bool
	fs.StringVar(&cfgPath, "config", config.DefaultPath(), "path to config file")
	fs.StringVar(&addr, "addr", "", "PVS6 WebSocket address (overrides config and PVS_ADDR)")
	fs.StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database (empty to disable)")
	fs.BoolVar(&verbose, "verbose", false, "enable debug logging")
	fs.BoolVar(&verbose, "v", false, "enable debug logging (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))

	var store pvs.Store
	var settingsStore config.SettingsStore
	if dbPath != "" {
		s, err := sqlite.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer func() { _ = s.Close() }()
		store = s
		settingsStore = s
		logger.Info("sqlite store opened", "path", dbPath)
	}
	logger.Info("pvs-monitor starting", "version", version.Version)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	logger.Info("config loaded", "path", cfgPath)

	// Overlay DB-backed settings on top of the file config. On first run the
	// settings table is seeded from the file so existing installs migrate
	// transparently; thereafter the DB is authoritative.
	if settingsStore != nil {
		// Startup config resolution uses its own context, independent of the
		// run/shutdown signal context, so a fast local DB read isn't aborted.
		startupCtx := context.Background()
		seeded, err := config.SeedSettingsIfEmpty(startupCtx, settingsStore, cfg)
		if err != nil {
			return err
		}
		if seeded {
			logger.Info("seeded settings table from config file")
		}
		cfg, err = config.LoadWithStore(startupCtx, cfgPath, settingsStore)
		if err != nil {
			return err
		}
	}

	// Precedence: flag > env > DB > config file > default. Flag and env win,
	// so they are applied last, on top of the DB overlay above.
	if addr != "" {
		cfg.Addr = addr
	} else if env := os.Getenv("PVS_ADDR"); env != "" {
		cfg.Addr = env
	}

	monitor := pvs.NewMonitor(cfg.Addr, cfg, store, logger)
	go func() {
		if err := monitor.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("monitor stopped", "err", err)
		}
	}()

	if cfg.DeviceList.Password != "" {
		poller := pvs.NewDevicePoller(cfg.DeviceList, store, logger)
		go func() {
			if err := poller.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("device poller stopped", "err", err)
			}
		}()
		logger.Info("device list poller starting", "url", cfg.DeviceList.URL, "interval", cfg.DeviceList.Interval.Duration())
	}

	if store != nil {
		go func() {
			checkpoint := func() {
				if err := store.Checkpoint(ctx); err != nil && ctx.Err() == nil {
					logger.Warn("wal checkpoint failed", "err", err)
				} else {
					logger.Info("wal checkpoint complete")
				}
			}
			checkpoint()
			t := time.NewTicker(2 * time.Hour)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					checkpoint()
				}
			}
		}()
	}

	if cfg.IsUnconfigured() {
		logger.Error("pvs-monitor is not configured — edit the config file and restart", "config", cfgPath)
		return &exitError{code: 2, msg: fmt.Sprintf("unconfigured: edit %s and restart", cfgPath)}
	}
	logger.Info("pvs-monitor starting", "addr", cfg.Addr)
	<-ctx.Done()
	return nil
}
