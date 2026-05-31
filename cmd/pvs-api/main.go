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
		fmt.Fprintf(os.Stderr, "pvs-api: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, ctx context.Context) error {
	fs := flag.NewFlagSet("pvs-api", flag.ContinueOnError)
	var dbPath, listenAddr, tlsCert, tlsKey string
	var verbose bool
	fs.StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	fs.StringVar(&listenAddr, "addr", ":8081", "HTTPS listen address")
	fs.StringVar(&tlsCert, "tls-cert", "", "path to TLS certificate file (required)")
	fs.StringVar(&tlsKey, "tls-key", "", "path to TLS key file (required)")
	fs.BoolVar(&verbose, "v", false, "enable debug logging")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if tlsCert == "" || tlsKey == "" {
		return fmt.Errorf("-tls-cert and -tls-key are required; generate with: openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -days 3650 -nodes -keyout server.key -out server.crt -subj '/CN=pvs-api'")
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

	srv := &apiServer{store: store, logger: logger}
	httpSrv := &http.Server{Addr: listenAddr, Handler: srv.routes()}

	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	// TODO: replace -tls-cert/-tls-key with a -domain flag that wires autocert.Manager here
	// for public deployments using Let's Encrypt.
	logger.Info("pvs-api listening", "addr", listenAddr)
	if err := httpSrv.ListenAndServeTLS(tlsCert, tlsKey); err != http.ErrServerClosed {
		return err
	}
	return nil
}
