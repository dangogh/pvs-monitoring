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
	// An explicit config keeps the test off the developer's real config file,
	// which config.DefaultPath() would otherwise read.
	err := run([]string{"--config", writeConfig(t, "")}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunRejectsInvalidAPIURL(t *testing.T) {
	for _, bad := range []string{"solar.local", "ftp://solar.local", "http://"} {
		t.Run(bad, func(t *testing.T) {
			err := run([]string{"--config", writeConfig(t, ""), "--api", bad}, io.Discard, noopTransport{})
			assert.ErrorContains(t, err, "invalid api URL")
		})
	}
}

// Startup does not probe the API: a client launching while the monitoring host
// is down must still come up, so that the failure is reported per-call.
func TestRunStartsWithUnreachableAPI(t *testing.T) {
	err := run([]string{"--config", writeConfig(t, ""), "--api", "http://127.0.0.1:1"}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}

func TestRunInvalidConfigReturnsError(t *testing.T) {
	p := writeConfig(t, `stale_threshold: "notaduration"`)
	err := run([]string{"--config", p}, io.Discard, noopTransport{})
	assert.Error(t, err)
}

// -insecure and -ca both configure TLS trust; taking both would leave which one
// wins implicit, and it would be the unsafe one.
func TestRunRejectsInsecureWithCA(t *testing.T) {
	err := run([]string{"--config", writeConfig(t, ""), "--insecure", "--ca", "/tmp/ca.pem"}, io.Discard, noopTransport{})
	assert.ErrorContains(t, err, "mutually exclusive")
}

func TestRunRejectsUnreadableCA(t *testing.T) {
	err := run([]string{"--config", writeConfig(t, ""), "--ca", "/nonexistent/ca.pem"}, io.Discard, noopTransport{})
	assert.ErrorContains(t, err, "read CA cert")
}

func TestRunAcceptsNameAndInsecure(t *testing.T) {
	err := run([]string{"--config", writeConfig(t, ""), "--name", "pvs-solarwatch", "--api", "https://127.0.0.1:1", "--insecure"}, io.Discard, noopTransport{})
	assert.NoError(t, err)
}
