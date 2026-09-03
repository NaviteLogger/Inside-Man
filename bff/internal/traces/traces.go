// Package traces queries Tempo over its HTTP API.
//
// Tempo publishes no Go client library, so this is plain HTTP against the
// documented endpoints. See docs/join-key.md for how a trace reaches its logs.
package traces

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	base string
	http *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: timeout}}
}

// Summary is one row in the "recent traces" list.
type Summary struct {
	TraceID         string `json:"traceId"`
	RootServiceName string `json:"rootServiceName"`
	RootTraceName   string `json:"rootTraceName"`
	DurationMillis  int    `json:"durationMillis"`
	StartUnixNano   string `json:"startTimeUnixNano"`
}

type searchResponse struct {
	Traces []Summary `json:"traces"`
}

// Search runs TraceQL. The caller supplies the query so the UI can ask for
// errors, slow traces, or everything, without this package growing an option
// for each.
func (c *Client) Search(ctx context.Context, traceQL string, span time.Duration, limit int) ([]Summary, error) {
	if limit <= 0 {
		limit = 20
	}
	end := time.Now()
	q := url.Values{
		"q":     {traceQL},
		"limit": {strconv.Itoa(limit)},
		"start": {strconv.FormatInt(end.Add(-span).Unix(), 10)},
		"end":   {strconv.FormatInt(end.Unix(), 10)},
	}

	var out searchResponse
	if err := c.get(ctx, "/api/search?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return out.Traces, nil
}

// ErrorTraces returns recent failing traces for a service, which is the list
// the detail screen leads with.
func (c *Client) ErrorTraces(ctx context.Context, service string, span time.Duration, limit int) ([]Summary, error) {
	return c.Search(ctx, fmt.Sprintf(`{resource.service.name=%q && status=error}`, service), span, limit)
}

// SlowTraces returns the slowest recent traces for a service.
func (c *Client) SlowTraces(ctx context.Context, service string, minDuration time.Duration, span time.Duration, limit int) ([]Summary, error) {
	return c.Search(ctx,
		fmt.Sprintf(`{resource.service.name=%q && duration > %s}`, service, minDuration.String()),
		span, limit)
}

// Up reports whether Tempo answers, for the diagnostics page.
func (c *Client) Up(ctx context.Context) error {
	var discard map[string]any
	return c.get(ctx, "/api/echo", &discard)
}

func (c *Client) get(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tempo: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("tempo returned %s for %s", res.Status, path)
	}
	// /api/echo answers with plain text, so a decode failure there is expected
	// and the status code is the real signal.
	if err := json.NewDecoder(res.Body).Decode(into); err != nil && path != "/api/echo" {
		return fmt.Errorf("tempo response: %w", err)
	}
	return nil
}
