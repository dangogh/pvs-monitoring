package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/dangogh/pvs-monitoring/pvs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pvs-monitor: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "ws://192.168.191.155:9002", "PVS6 WebSocket address")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	flag.BoolVar(verbose, "v", false, "enable debug logging (shorthand)")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	monitor := pvs.NewMonitor(*addr, logger)

	ctx := context.Background()

	// Run the WebSocket monitor in the background.
	monCtx, cancelMon := context.WithCancel(ctx)
	defer cancelMon()
	go func() {
		if err := monitor.Run(monCtx); err != nil && monCtx.Err() == nil {
			logger.Error("monitor stopped", "err", err)
		}
	}()

	server := mcp.NewServer(&mcp.Implementation{Name: "pvs-monitor", Version: "0.1.0"}, nil)
	pvs.RegisterTools(server, monitor)

	logger.Info("pvs-monitor starting", "addr", *addr)
	return server.Run(ctx, &mcp.StdioTransport{})
}
