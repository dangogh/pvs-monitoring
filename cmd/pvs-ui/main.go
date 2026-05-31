package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
	if err := run(os.Args[1:], ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "pvs-ui: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, ctx context.Context) error {
	fs := flag.NewFlagSet("pvs-ui", flag.ContinueOnError)
	var dbPath, listenAddr string
	var verbose bool
	fs.StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	fs.StringVar(&listenAddr, "addr", ":8080", "HTTP listen address")
	fs.BoolVar(&verbose, "v", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	store, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	srv := &server{store: store, logger: logger}
	httpSrv := &http.Server{Addr: listenAddr, Handler: srv.routes()}

	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	logger.Info("pvs-ui listening", "addr", listenAddr)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
