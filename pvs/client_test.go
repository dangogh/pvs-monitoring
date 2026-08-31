package pvs

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient builds a Client, failing the test if construction errors.
func newTestClient(t *testing.T, baseURL string, opts ...ClientOption) *Client {
	t.Helper()
	c, err := NewClient(baseURL, opts...)
	require.NoError(t, err)
	return c
}

func TestClientCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/current", r.URL.Path)
		_, _ = w.Write([]byte(`{"solar_kw":4.2,"load_kw":1.8,"net_kw":2.4,"updated_at":"2026-08-30T14:02:11Z"}`))
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv.URL).Current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 4.2, got.SolarKW)
	assert.Equal(t, 2.4, got.NetKW)
	assert.Equal(t, 2026, got.UpdatedAt.Year())
}

// A 503 means the API is up but has no reading yet. That must not look like an
// outage, because the two call for different responses from the caller.
func TestClientNoDataIsNotUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no data", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).Current(context.Background())
	assert.ErrorIs(t, err, ErrNoData)
	assert.NotErrorIs(t, err, ErrUnreachable)
}

// A 404 means the server is healthy but predates the endpoint. Reporting that
// as an outage sends the reader looking at the network instead of the version.
func TestClientNotFoundIsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).PanelHealth(context.Background())
	assert.ErrorIs(t, err, ErrUnsupported)
	assert.NotErrorIs(t, err, ErrUnreachable)
}

func TestClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	srv.Close() // nothing is listening

	_, err := newTestClient(t, srv.URL).PanelHealth(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestClientServerErrorIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).PanelHealth(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestClientDataSendsUnixRange(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"summary":{"solar_kwh":31.5,"load_kwh":20.0}}`))
	}))
	defer srv.Close()

	since := time.Unix(1756500000, 0)
	until := time.Unix(1756600000, 0)
	got, err := newTestClient(t, srv.URL).Data(context.Background(), since, until)
	require.NoError(t, err)
	assert.Equal(t, "since=1756500000&until=1756600000", gotQuery)
	assert.Equal(t, 31.5, got.Summary.SolarKWh)
}

func TestClientDecodeErrorIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).Current(context.Background())
	assert.True(t, errors.Is(err, ErrUnreachable))
}

// The monitoring hosts serve pvs-api under self-signed certificates, so
// verification has to be defeatable — otherwise HTTPS access is impossible
// without distributing a CA.
func TestClientInsecureTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"solar_kw":3.2}`))
	}))
	defer srv.Close()

	// Without the option, the self-signed certificate is rejected.
	_, err := newTestClient(t, srv.URL).Current(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)

	got, err := newTestClient(t, srv.URL, WithInsecureTLS()).Current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3.2, got.SolarKW)
}

func TestClientWithCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"solar_kw":4.4}`))
	}))
	defer srv.Close()

	pemPath := filepath.Join(t.TempDir(), "ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	require.NoError(t, os.WriteFile(pemPath, pemBytes, 0o600))

	got, err := newTestClient(t, srv.URL, WithCACert(pemPath)).Current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 4.4, got.SolarKW)
}

func TestClientWithCACertErrors(t *testing.T) {
	_, err := NewClient("https://example.invalid", WithCACert("/nonexistent/ca.pem"))
	assert.Error(t, err)

	junk := filepath.Join(t.TempDir(), "junk.pem")
	require.NoError(t, os.WriteFile(junk, []byte("not a certificate"), 0o600))
	_, err = NewClient("https://example.invalid", WithCACert(junk))
	assert.ErrorContains(t, err, "no certificates found")
}

// A typo in -api should fail while someone is looking at the command line, not
// several minutes later as an "unreachable host" inside a tool call.
func TestNewClientRejectsBadURLs(t *testing.T) {
	for _, bad := range []string{"solar.local", "ftp://solar.local", "http://", ""} {
		t.Run(bad, func(t *testing.T) {
			_, err := NewClient(bad)
			assert.ErrorContains(t, err, "invalid api URL")
		})
	}
}

// A trailing slash would otherwise produce "//api/current".
func TestNewClientTrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL+"/")
	require.NoError(t, func() error { _, err := c.Current(context.Background()); return err }())
	assert.Equal(t, "/api/current", gotPath)
}

// An endpoint answering with an unbounded stream must not be decoded into
// unbounded memory.
func TestClientCapsResponseSize(t *testing.T) {
	orig := maxResponseBytes
	maxResponseBytes = 512
	t.Cleanup(func() { maxResponseBytes = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Valid JSON prefix, then an array far longer than the cap allows.
		_, _ = w.Write([]byte(`{"series":[`))
		chunk := []byte(`{"t":1,"s":1.0,"l":1.0},`)
		for range 200 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	// Decoding stops at the cap, so the truncated body fails to parse rather
	// than being consumed in full.
	_, err := newTestClient(t, srv.URL).Data(context.Background(), time.Unix(0, 0), time.Unix(1, 0))
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.ErrorContains(t, err, "decoding")
}
