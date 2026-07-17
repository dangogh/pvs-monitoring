package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dangogh/pvs-monitoring/config"
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(os.Args[1:], os.Stderr, ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "pvs-monitor: %v\n", err)
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

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	slog.Info("config loaded", "path", cfgPath)

	// Precedence: flag > env > config file > default.
	if addr != "" {
		cfg.Addr = addr
	} else if env := os.Getenv("PVS_ADDR"); env != "" {
		cfg.Addr = env
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))

	var store pvs.Store
	if dbPath != "" {
		s, err := sqlite.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer s.Close()
		store = s
		logger.Info("sqlite store opened", "path", dbPath)
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

	logger.Info("pvs-monitor starting", "addr", cfg.Addr)
	<-ctx.Done()
	return nil
}
