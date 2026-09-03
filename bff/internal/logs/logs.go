// Package logs queries Loki over its HTTP API.
//
// Loki publishes no official Go query client, and importing logcli's package
// drags the whole Loki module into the binary, so this is plain HTTP.
package logs

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

	// excludeNamespace is dropped from trace-wide queries. Inside Man's own
	// components log the trace id being searched for, so including them would
	// answer "logs for this request" with logs about the question.
	excludeNamespace string
}

func New(baseURL string, timeout time.Duration, excludeNamespace string) *Client {
	return &Client{
		base:             baseURL,
		http:             &http.Client{Timeout: timeout},
		excludeNamespace: excludeNamespace,
	}
}

// selector matches every application stream, minus our own namespace.
func (c *Client) selector() string {
	if c.excludeNamespace == "" {
		return `{service_name=~".+"}`
	}
	return fmt.Sprintf(`{service_name=~".+", k8s_namespace_name!=%q}`, c.excludeNamespace)
}

// Line is one log entry, with the stream labels that identify where it came
// from.
type Line struct {
	Timestamp time.Time `json:"timestamp"`
	Line      string    `json:"line"`
	Service   string    `json:"service"`
	Pod       string    `json:"pod,omitempty"`
}

type queryResponse struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// ForService returns recent lines for one service, keyed on the service_name
// index label that Loki derives from the OTLP service.name attribute.
func (c *Client) ForService(ctx context.Context, service string, span time.Duration, limit int) ([]Line, error) {
	return c.query(ctx, fmt.Sprintf(`{service_name=%q}`, service), span, limit)
}

// ForTrace returns every line carrying the given trace id, across services.
//
// The id lives in the log body, since Alloy collects container stdout and Loki
// never sees an OTLP TraceId field to promote into structured metadata. A line
// filter is therefore the right tool. See docs/join-key.md.
func (c *Client) ForTrace(ctx context.Context, traceID string, span time.Duration, limit int) ([]Line, error) {
	if !isHexID(traceID) {
		return nil, fmt.Errorf("not a trace id: %q", traceID)
	}
	return c.query(ctx, fmt.Sprintf(`%s |= %q`, c.selector(), traceID), span, limit)
}

// ForServiceAndTrace narrows to one service's view of a trace.
func (c *Client) ForServiceAndTrace(ctx context.Context, service, traceID string, span time.Duration, limit int) ([]Line, error) {
	if !isHexID(traceID) {
		return nil, fmt.Errorf("not a trace id: %q", traceID)
	}
	return c.query(ctx, fmt.Sprintf(`{service_name=%q} |= %q`, service, traceID), span, limit)
}

// Up reports whether Loki answers, for the diagnostics page.
func (c *Client) Up(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/ready", nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("loki: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("loki returned %s", res.Status)
	}
	return nil
}

func (c *Client) query(ctx context.Context, logQL string, span time.Duration, limit int) ([]Line, error) {
	if limit <= 0 {
		limit = 100
	}
	end := time.Now()
	q := url.Values{
		"query":     {logQL},
		"limit":     {strconv.Itoa(limit)},
		"direction": {"backward"},
		"start":     {strconv.FormatInt(end.Add(-span).UnixNano(), 10)},
		"end":       {strconv.FormatInt(end.UnixNano(), 10)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/loki/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki returned %s", res.Status)
	}

	var body queryResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("loki response: %w", err)
	}

	var out []Line
	for _, stream := range body.Data.Result {
		for _, v := range stream.Values {
			ns, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				continue
			}
			out = append(out, Line{
				Timestamp: time.Unix(0, ns).UTC(),
				Line:      v[1],
				Service:   stream.Stream["service_name"],
				Pod:       stream.Stream["k8s_pod_name"],
			})
		}
	}
	return out, nil
}

// isHexID guards the line filter. A trace id is 32 hex characters and a span id
// is 16, and refusing anything else keeps arbitrary text out of a LogQL filter.
func isHexID(s string) bool {
	if len(s) != 32 && len(s) != 16 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
