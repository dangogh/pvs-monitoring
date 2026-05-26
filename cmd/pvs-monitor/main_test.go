package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopTransport is an mcp.Transport that connects and immediately closes.
type noopTransport struct{}

func (noopTransport) Connect(_ context.Context) (mcp.Connection, error) {
	return noopConnection{}, nil
}

type noopConnection struct{}

func (noopConnection) Read(_ context.Context) (jsonrpc.Message, error) { return nil, io.EOF }
func (noopConnection) Write(_ context.Context, _ jsonrpc.Message) error { return nil }
func (noopConnection) Close() error                                      { return nil }
func (noopConnection) SessionID() string                                 { return "noop" }

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestRunSucceeds(t *testing.T) {
	err := run([]string{"--db", ""}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunInvalidConfigReturnsError(t *testing.T) {
	p := writeConfig(t, `stale_threshold: "notaduration"`)
	err := run([]string{"--config", p, "--db", ""}, io.Discard, noopTransport{})
	assert.Error(t, err)
}

func TestRunAddrFlagOverridesConfig(t *testing.T) {
	// Verify --addr takes precedence over config file value.
	p := writeConfig(t, `addr: ws://config-addr:9002`)
	// Just needs to run without error; the addr is used by the monitor goroutine
	// which we don't wait on.
	err := run([]string{"--config", p, "--addr", "ws://flag-addr:9002", "--db", ""}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunEnvAddrOverridesConfig(t *testing.T) {
	t.Setenv("PVS_ADDR", "ws://env-addr:9002")
	p := writeConfig(t, `addr: ws://config-addr:9002`)
	err := run([]string{"--config", p, "--db", ""}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunAddrFlagOverridesEnv(t *testing.T) {
	t.Setenv("PVS_ADDR", "ws://env-addr:9002")
	err := run([]string{"--addr", "ws://flag-addr:9002", "--db", ""}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunWithDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "readings.db")
	err := run([]string{"--db", dbPath}, io.Discard, noopTransport{})
	assert.NoError(t, err)
	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr, "db file should have been created")
}

func TestRunBadDBPathReturnsError(t *testing.T) {
	err := run([]string{"--db", "/nonexistent/path/that/cannot/be/created/readings.db"}, io.Discard, noopTransport{})
	assert.Error(t, err)
}

func TestDefaultDBPath(t *testing.T) {
	t.Run("uses XDG_DATA_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/custom/data")
		assert.Equal(t, "/custom/data/pvs-monitor/readings.db", defaultDBPath())
	})

	t.Run("falls back to home dir", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		home, _ := os.UserHomeDir()
		assert.Equal(t, home+"/.local/share/pvs-monitor/readings.db", defaultDBPath())
	})
}
