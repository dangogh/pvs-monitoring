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

	"github.com/dangogh/pvs-monitoring/pvs"
	"github.com/dangogh/pvs-monitoring/store/sqlite"
)

func defaultDBPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = home + "/.local/share"
	}
	return base + "/pvs-monitor/readings.db"
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(os.Args[1:], ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "pvs-api: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, ctx context.Context) error {
	fs := flag.NewFlagSet("pvs-api", flag.ContinueOnError)
	var dbPath, listenAddr, tlsCert, tlsKey string
	var verbose bool
	fs.StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	fs.StringVar(&listenAddr, "addr", ":8081", "listen address")
	fs.StringVar(&tlsCert, "tls-cert", "", "path to TLS certificate file (optional; plain HTTP if omitted)")
	fs.StringVar(&tlsKey, "tls-key", "", "path to TLS key file (optional; plain HTTP if omitted)")
	fs.BoolVar(&verbose, "v", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (tlsCert == "") != (tlsKey == "") {
		return fmt.Errorf("-tls-cert and -tls-key must both be provided or both be omitted")
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

	return serve(ctx, store, listenAddr, tlsCert, tlsKey, logger)
}

func serve(ctx context.Context, store pvs.Store, addr, tlsCert, tlsKey string, logger *slog.Logger) error {
	srv := &apiServer{store: store, logger: logger}
	httpSrv := &http.Server{Addr: addr, Handler: srv.routes()}

	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	logger.Info("pvs-api listening", "addr", addr, "tls", tlsCert != "")
	var err error
	if tlsCert != "" {
		err = httpSrv.ListenAndServeTLS(tlsCert, tlsKey)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if err != http.ErrServerClosed {
		return err
	}
	return nil
}
