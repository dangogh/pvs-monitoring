package pvs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/current", r.URL.Path)
		_, _ = w.Write([]byte(`{"solar_kw":4.2,"load_kw":1.8,"net_kw":2.4,"updated_at":"2026-08-30T14:02:11Z"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).Current(context.Background())
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

	_, err := NewClient(srv.URL).Current(context.Background())
	assert.ErrorIs(t, err, ErrNoData)
	assert.NotErrorIs(t, err, ErrUnreachable)
}

func TestClientUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	srv.Close() // nothing is listening

	_, err := NewClient(srv.URL).PanelHealth(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
}

func TestClientServerErrorIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).PanelHealth(context.Background())
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
	got, err := NewClient(srv.URL).Data(context.Background(), since, until)
	require.NoError(t, err)
	assert.Equal(t, "since=1756500000&until=1756600000", gotQuery)
	assert.Equal(t, 31.5, got.Summary.SolarKWh)
}

func TestClientDecodeErrorIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).Current(context.Background())
	assert.True(t, errors.Is(err, ErrUnreachable))
}
