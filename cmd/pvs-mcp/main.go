package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
	if err := run(os.Args[1:], os.Stderr, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "pvs-mcp: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, logOut io.Writer, transport mcp.Transport) error {
	fs := flag.NewFlagSet("pvs-mcp", flag.ContinueOnError)
	var cfgPath, dbPath string
	var verbose bool
	fs.StringVar(&cfgPath, "config", config.DefaultPath(), "path to config file")
	fs.StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	fs.BoolVar(&verbose, "verbose", false, "enable debug logging")
	fs.BoolVar(&verbose, "v", false, "enable debug logging (shorthand)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level}))

	store, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()
	logger.Info("pvs-mcp starting", "db", dbPath)

	server := mcp.NewServer(&mcp.Implementation{Name: "pvs-mcp", Version: "0.1.0"}, nil)
	pvs.RegisterTools(server, store, cfg)

	return server.Run(context.Background(), transport)
}
