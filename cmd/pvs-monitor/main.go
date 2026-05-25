package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dangogh/pvs-monitoring/config"
	"github.com/dangogh/pvs-monitoring/pvs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pvs-monitor: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var cfgPath, addr string
	var verbose bool
	flag.StringVar(&cfgPath, "config", config.DefaultPath(), "path to config file")
	flag.StringVar(&addr, "addr", "", "PVS6 WebSocket address (overrides config and PVS_ADDR)")
	flag.BoolVar(&verbose, "verbose", false, "enable debug logging")
	flag.BoolVar(&verbose, "v", false, "enable debug logging (shorthand)")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	monitor := pvs.NewMonitor(cfg.Addr, cfg, logger)

	ctx := context.Background()

	monCtx, cancelMon := context.WithCancel(ctx)
	defer cancelMon()
	go func() {
		if err := monitor.Run(monCtx); err != nil && monCtx.Err() == nil {
			logger.Error("monitor stopped", "err", err)
		}
	}()

	server := mcp.NewServer(&mcp.Implementation{Name: "pvs-monitor", Version: "0.1.0"}, nil)
	pvs.RegisterTools(server, monitor)

	logger.Info("pvs-monitor starting", "addr", cfg.Addr)
	return server.Run(ctx, &mcp.StdioTransport{})
}
