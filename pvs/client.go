package pvs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

// NewClient returns a Client for the pvs-api at baseURL (e.g.
// "http://solar.local"). The timeout is short: every call backs a synchronous
// tool invocation, and a caller waiting on a dead host wants to be told so
// rather than made to wait.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
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

	switch {
	case resp.StatusCode == http.StatusServiceUnavailable:
		// The API answered; it simply has nothing yet. Not an outage.
		return ErrNoData
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: %s returned %s", ErrUnreachable, path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
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
