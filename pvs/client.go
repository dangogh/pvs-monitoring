package pvs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrUnreachable wraps any failure to get an answer out of pvs-api: a dead
// host, a broken network, a 5xx. It is deliberately distinct from a successful
// response that happens to carry stale or absent data — "the server is down"
// and "the server says the monitor stopped reporting an hour ago" are different
// diagnoses, and collapsing them into one error loses the more useful half.
var ErrUnreachable = errors.New("pvs-api unreachable")

// ErrNoData reports that pvs-api answered but has no reading to give yet.
var ErrNoData = errors.New("no data available yet")

// ErrUnsupported reports that pvs-api answered but does not serve the endpoint,
// meaning it is older than the feature. Distinct from ErrUnreachable for the
// same reason ErrNoData is: the host is healthy and the fix is to upgrade it,
// not to go looking at the network.
var ErrUnsupported = errors.New("endpoint not supported by this pvs-api")

// maxResponseBytes caps how much of a response body is decoded. A full-detail
// series over a long range is the largest legitimate payload and stays far
// under this.
// It is a var, not a const, only so tests can shrink it.
var maxResponseBytes int64 = 64 << 20 // 64 MiB

// API is the read surface the MCP tools need. Client implements it against a
// running pvs-api; tests supply a fake.
type API interface {
	Current(ctx context.Context) (CurrentReading, error)
	Data(ctx context.Context, since, until time.Time) (DataResponse, error)
	PanelHealth(ctx context.Context) (PanelHealth, error)
}

// Client reads from a pvs-api instance over HTTP.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*clientConfig) error

type clientConfig struct {
	tls *tls.Config
}

// WithInsecureTLS disables certificate verification. The monitoring hosts serve
// pvs-api under self-signed certificates, so this is how you reach them over
// HTTPS without distributing a CA. It removes any guarantee that the host is
// who it claims to be; prefer WithCACert where the certificate is available.
func WithInsecureTLS() ClientOption {
	return func(c *clientConfig) error {
		if c.tls == nil {
			c.tls = &tls.Config{} //nolint:gosec // MinVersion set below.
		}
		c.tls.MinVersion = tls.VersionTLS12
		c.tls.InsecureSkipVerify = true //nolint:gosec // deliberate; see doc comment.
		return nil
	}
}

// WithCACert trusts the PEM certificate(s) at path in addition to nothing else,
// which is how to reach a self-signed pvs-api while still verifying it is the
// host you meant.
//
// This requires a certificate carrying a subjectAltName. Go has rejected
// CN-only certificates since 1.15 no matter what is in the trust store, so a
// certificate generated without SANs fails here even when supplied as its own
// CA — as the ones on the monitoring hosts do today. Until those are reissued
// with SANs, HTTPS access needs WithInsecureTLS.
func WithCACert(path string) ClientOption {
	return func(c *clientConfig) error {
		pem, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("no certificates found in %s", path)
		}
		if c.tls == nil {
			c.tls = &tls.Config{} //nolint:gosec // MinVersion set below.
		}
		c.tls.MinVersion = tls.VersionTLS12
		c.tls.RootCAs = pool
		return nil
	}
}

// NewClient returns a Client for the pvs-api at baseURL (e.g.
// "http://solar.local"). The timeout is short: every call backs a synchronous
// tool invocation, and a caller waiting on a dead host wants to be told so
// rather than made to wait.
func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	// Validate here rather than on first use: a typo in -api should fail while
	// someone is looking at the command line, not several minutes later inside
	// a tool call that reports it as an unreachable host.
	baseURL = strings.TrimSuffix(baseURL, "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api URL %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid api URL %q: need an http:// or https:// base URL", baseURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid api URL %q: no host", baseURL)
	}

	var cfg clientConfig
	for _, opt := range opts {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	if cfg.tls != nil {
		// NextProtos pins HTTP/1.1. Go's HTTP/2 transport hangs against some
		// TLS configurations here, the same hazard DevicePoller works around.
		cfg.tls.NextProtos = []string{"http/1.1"}
		httpClient.Transport = &http.Transport{TLSClientConfig: cfg.tls}
	}

	return &Client{BaseURL: baseURL, HTTP: httpClient}, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A long series is large but bounded; anything past this is a wrong or
	// hostile endpoint, and decoding it would consume memory unboundedly.
	body := io.LimitReader(resp.Body, maxResponseBytes)

	switch {
	case resp.StatusCode == http.StatusServiceUnavailable:
		// The API answered; it simply has nothing yet. Not an outage.
		return ErrNoData
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrUnsupported, path)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: %s returned %s", ErrUnreachable, path, resp.Status)
	}

	if err := json.NewDecoder(body).Decode(out); err != nil {
		return fmt.Errorf("%w: decoding %s: %v", ErrUnreachable, path, err)
	}
	return nil
}

// Current returns the latest instantaneous reading. It returns ErrNoData if the
// monitor has not recorded anything yet. A reading that is merely old is
// returned normally — judging staleness is the caller's job.
func (c *Client) Current(ctx context.Context) (CurrentReading, error) {
	var out CurrentReading
	err := c.get(ctx, "/api/current", nil, &out)
	return out, err
}

// Data returns energy totals, average power, and a series for the given range.
func (c *Client) Data(ctx context.Context, since, until time.Time) (DataResponse, error) {
	q := url.Values{
		"since": {strconv.FormatInt(since.Unix(), 10)},
		"until": {strconv.FormatInt(until.Unix(), 10)},
	}
	var out DataResponse
	err := c.get(ctx, "/api/data", q, &out)
	return out, err
}

// PanelHealth returns a point-in-time assessment of every inverter. The verdict
// has no memory of previous calls; see EvaluatePanelHealth.
func (c *Client) PanelHealth(ctx context.Context) (PanelHealth, error) {
	var out PanelHealth
	err := c.get(ctx, "/api/panel-health", nil, &out)
	return out, err
}
