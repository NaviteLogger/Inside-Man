// Package promql runs the RED queries behind the services list.
//
// The metric and label names here were verified against a live cluster and are
// not the ones design doc 4.4 assumed. Alloy's spanmetrics connector labels by
// service_name and names its histogram duration_seconds, where Tempo's
// metrics-generator uses service and latency. See docs/join-key.md.
package promql

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	promapi "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

const (
	labelService   = "service_name"
	labelNamespace = "k8s_namespace_name"

	metricCalls        = "traces_spanmetrics_calls_total"
	metricDurationHist = "traces_spanmetrics_duration_seconds_bucket"
	errorSelector      = `status_code="STATUS_CODE_ERROR"`
)

type Client struct {
	api    promapi.API
	window time.Duration
}

func New(baseURL string, window time.Duration) (*Client, error) {
	c, err := api.NewClient(api.Config{Address: baseURL})
	if err != nil {
		return nil, fmt.Errorf("prometheus client: %w", err)
	}
	return &Client{api: promapi.NewAPI(c), window: window}, nil
}

// Key identifies a service. Namespace can be empty when a service reports no
// Kubernetes attributes, which the diagnostics page treats as a broken join.
type Key struct {
	Service   string
	Namespace string
}

// RED holds the three numbers the services list shows.
type RED struct {
	RequestRate float64
	ErrorRatio  float64
	P95         time.Duration
}

// ServiceRED returns RED metrics for every service with spans in the window.
func (c *Client) ServiceRED(ctx context.Context) (map[Key]*RED, error) {
	w := model.Duration(c.window).String()
	out := map[Key]*RED{}

	rate, err := c.vector(ctx, fmt.Sprintf(
		`sum by (%s, %s) (rate(%s[%s]))`, labelService, labelNamespace, metricCalls, w))
	if err != nil {
		return nil, fmt.Errorf("request rate: %w", err)
	}
	for k, v := range rate {
		out[k] = &RED{RequestRate: v}
	}

	errs, err := c.vector(ctx, fmt.Sprintf(
		`sum by (%s, %s) (rate(%s{%s}[%s]))`, labelService, labelNamespace, metricCalls, errorSelector, w))
	if err != nil {
		return nil, fmt.Errorf("error rate: %w", err)
	}
	for k, v := range errs {
		red, ok := out[k]
		if !ok {
			// Errors without a matching total would be a Prometheus
			// inconsistency, so treat the error rate as the whole of it.
			out[k] = &RED{RequestRate: v, ErrorRatio: 1}
			continue
		}
		if red.RequestRate > 0 {
			red.ErrorRatio = v / red.RequestRate
		}
	}

	p95, err := c.vector(ctx, fmt.Sprintf(
		`histogram_quantile(0.95, sum by (%s, %s, le) (rate(%s[%s])))`,
		labelService, labelNamespace, metricDurationHist, w))
	if err != nil {
		return nil, fmt.Errorf("p95: %w", err)
	}
	for k, v := range p95 {
		red, ok := out[k]
		if !ok {
			continue
		}
		// histogram_quantile yields NaN for buckets with no observations.
		if v == v {
			red.P95 = time.Duration(v * float64(time.Second))
		}
	}

	return out, nil
}

// Sparkline returns a request-rate series per service for the given range.
func (c *Client) Sparkline(ctx context.Context, span time.Duration, points int) (map[Key][]float64, error) {
	if points <= 0 {
		points = 30
	}
	end := time.Now()
	r := promapi.Range{
		Start: end.Add(-span),
		End:   end,
		Step:  span / time.Duration(points),
	}
	q := fmt.Sprintf(`sum by (%s, %s) (rate(%s[%s]))`,
		labelService, labelNamespace, metricCalls, model.Duration(c.window).String())

	val, _, err := c.api.QueryRange(ctx, q, r)
	if err != nil {
		return nil, fmt.Errorf("sparkline: %w", err)
	}
	matrix, ok := val.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("sparkline: expected a matrix, got %s", val.Type())
	}

	out := map[Key][]float64{}
	for _, stream := range matrix {
		k := keyOf(stream.Metric)
		series := make([]float64, 0, len(stream.Values))
		for _, p := range stream.Values {
			series = append(series, float64(p.Value))
		}
		out[k] = series
	}
	return out, nil
}

// Up reports whether Prometheus answers at all, for the diagnostics page.
func (c *Client) Up(ctx context.Context) error {
	_, _, err := c.api.Query(ctx, "vector(1)", time.Now())
	return err
}

func (c *Client) vector(ctx context.Context, query string) (map[Key]float64, error) {
	val, warnings, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	_ = warnings

	vec, ok := val.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected a vector, got %s", val.Type())
	}
	out := make(map[Key]float64, len(vec))
	for _, s := range vec {
		out[keyOf(s.Metric)] = float64(s.Value)
	}
	return out, nil
}

func keyOf(m model.Metric) Key {
	return Key{
		Service:   string(m[labelService]),
		Namespace: string(m[labelNamespace]),
	}
}
