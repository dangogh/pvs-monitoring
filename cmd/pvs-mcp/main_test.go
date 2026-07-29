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

	"github.com/dangogh/pvs-monitoring/store/sqlite"
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

// seedDB creates and migrates a database file (as pvs-monitor would), so the
// read-only pvs-mcp can open it.
func seedDB(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "readings.db")
	s, err := sqlite.Open(p)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	return p
}

func TestRunSucceeds(t *testing.T) {
	// pvs-mcp opens the DB read-only and does not create it, so the writer must
	// have created it first (in production, pvs-monitor).
	dbPath := seedDB(t)
	err := run([]string{"--db", dbPath}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunInvalidConfigReturnsError(t *testing.T) {
	dbPath := seedDB(t)
	p := writeConfig(t, `stale_threshold: "notaduration"`)
	err := run([]string{"--config", p, "--db", dbPath}, io.Discard, noopTransport{})
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
