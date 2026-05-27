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

type noopTransport struct{}

func (noopTransport) Connect(_ context.Context) (mcp.Connection, error) {
	return noopConnection{}, nil
}

type noopConnection struct{}

func (noopConnection) Read(_ context.Context) (jsonrpc.Message, error)  { return nil, io.EOF }
func (noopConnection) Write(_ context.Context, _ jsonrpc.Message) error { return nil }
func (noopConnection) Close() error                                     { return nil }
func (noopConnection) SessionID() string                                { return "noop" }

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestRunSucceeds(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "readings.db")
	err := run([]string{"--db", dbPath}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunInvalidConfigReturnsError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "readings.db")
	p := writeConfig(t, `stale_threshold: "notaduration"`)
	err := run([]string{"--config", p, "--db", dbPath}, io.Discard, noopTransport{})
	assert.Error(t, err)
}

func TestRunBadDBPathReturnsError(t *testing.T) {
	err := run([]string{"--db", "/nonexistent/path/that/cannot/be/created/readings.db"}, io.Discard, noopTransport{})
	assert.Error(t, err)
}

func TestRunCreatesDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sub", "readings.db")
	err := run([]string{"--db", dbPath}, io.Discard, noopTransport{})
	assert.NoError(t, err)
	_, statErr := os.Stat(dbPath)
	assert.NoError(t, statErr, "db file should have been created")
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
