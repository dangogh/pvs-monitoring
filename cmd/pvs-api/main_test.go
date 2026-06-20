package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestDefaultDBPath_XDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	got := defaultDBPath()
	if got != "/custom/data/pvs-monitor/readings.db" {
		t.Errorf("unexpected path: %s", got)
	}
}

func TestDefaultDBPath_HomeDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	got := defaultDBPath()
	home, _ := os.UserHomeDir()
	want := home + "/.local/share/pvs-monitor/readings.db"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	err := run([]string{"-unknown"}, context.Background())
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestRun_TLSMismatch(t *testing.T) {
	err := run([]string{"-tls-cert", "cert.pem"}, context.Background())
	if err == nil || !strings.Contains(err.Error(), "tls-cert") {
		t.Fatalf("expected tls mismatch error, got %v", err)
	}
}

func TestRun_BadDB(t *testing.T) {
	err := run([]string{"-db", "/no/such/dir/readings.db"}, context.Background())
	if err == nil {
		t.Fatal("expected error opening nonexistent db path")
	}
}

func TestServe_StartsAndStops(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	store := &fakeStore{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, store, ln, "", "", logger)
	}()

	// Listener is already bound so the server is ready to accept immediately.
	resp, err := http.Get("http://" + addr + "/api/current")
	if err != nil {
		cancel()
		t.Fatalf("server not ready: %v", err)
	}
	resp.Body.Close()

	cancel()
	if err := <-done; err != nil {
		t.Errorf("serve returned error: %v", err)
	}
}
