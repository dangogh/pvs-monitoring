package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	_ "embed"
)

//go:embed static/index.html
var indexHTML []byte

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
	var listenAddr, apiBase, tlsCert, tlsKey string
	var verbose bool
	fs.StringVar(&listenAddr, "addr", ":8080", "HTTP listen address")
	fs.StringVar(&apiBase, "api", "http://localhost:8081", "pvs-api base URL")
	fs.StringVar(&tlsCert, "tls-cert", "", "path to TLS certificate file (optional)")
	fs.StringVar(&tlsKey, "tls-key", "", "path to TLS key file (optional)")
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

	apiURL, err := url.Parse(apiBase)
	if err != nil {
		return fmt.Errorf("invalid -api URL: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if apiURL.Scheme == "https" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // self-signed cert on loopback
	}
	proxy := httputil.NewSingleHostReverseProxy(apiURL)
	proxy.Transport = transport

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.Handle("/api/", proxy)

	httpSrv := &http.Server{Addr: listenAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	logger.Info("pvs-ui listening", "addr", listenAddr, "api", apiBase)
	if tlsCert != "" {
		if err := httpSrv.ListenAndServeTLS(tlsCert, tlsKey); err != http.ErrServerClosed {
			return err
		}
	} else {
		if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
			return err
		}
	}
	return nil
}
