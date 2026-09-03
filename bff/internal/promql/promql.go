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
	"sort"
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

// Edge is one dependency between two services, from the service graph.
type Edge struct {
	Client      string  `json:"client"`
	Server      string  `json:"server"`
	RequestRate float64 `json:"requestRate"`
	ErrorRatio  float64 `json:"errorRatio"`
}

// Dependencies returns the service graph edges touching the given service, so
// the detail screen can show what calls it and what it calls.
func (c *Client) Dependencies(ctx context.Context, service string) (inbound, outbound []Edge, err error) {
	w := model.Duration(c.window).String()

	total, err := c.edges(ctx, fmt.Sprintf(
		`sum by (client, server) (rate(traces_service_graph_request_total[%s]))`, w))
	if err != nil {
		return nil, nil, fmt.Errorf("service graph: %w", err)
	}
	failed, err := c.edges(ctx, fmt.Sprintf(
		`sum by (client, server) (rate(traces_service_graph_request_failed_total[%s]))`, w))
	if err != nil {
		// Failure counts are a refinement, so a missing series should not cost
		// the caller its topology.
		failed = map[[2]string]float64{}
	}

	for pair, rate := range total {
		e := Edge{Client: pair[0], Server: pair[1], RequestRate: rate}
		if rate > 0 {
			e.ErrorRatio = failed[pair] / rate
		}
		switch service {
		case pair[1]:
			inbound = append(inbound, e)
		case pair[0]:
			outbound = append(outbound, e)
		}
	}
	sortEdges(inbound)
	sortEdges(outbound)
	return inbound, outbound, nil
}

// AllEdges returns the whole service graph, which the map screen needs in M4.
func (c *Client) AllEdges(ctx context.Context) ([]Edge, error) {
	total, err := c.edges(ctx, fmt.Sprintf(
		`sum by (client, server) (rate(traces_service_graph_request_total[%s]))`,
		model.Duration(c.window).String()))
	if err != nil {
		return nil, err
	}
	out := make([]Edge, 0, len(total))
	for pair, rate := range total {
		out = append(out, Edge{Client: pair[0], Server: pair[1], RequestRate: rate})
	}
	sortEdges(out)
	return out, nil
}

func sortEdges(e []Edge) {
	sort.Slice(e, func(i, j int) bool {
		if e[i].RequestRate != e[j].RequestRate {
			return e[i].RequestRate > e[j].RequestRate
		}
		return e[i].Client+e[i].Server < e[j].Client+e[j].Server
	})
}

func (c *Client) edges(ctx context.Context, query string) (map[[2]string]float64, error) {
	val, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	vec, ok := val.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected a vector, got %s", val.Type())
	}
	out := make(map[[2]string]float64, len(vec))
	for _, s := range vec {
		out[[2]string{string(s.Metric["client"]), string(s.Metric["server"])}] = float64(s.Value)
	}
	return out, nil
}

// PodUsage is the CPU and memory a pod is using right now.
type PodUsage struct {
	CPUMillis float64 `json:"cpuMillis"`
	MemBytes  float64 `json:"memBytes"`
}

// PodResources returns per-pod usage for a namespace.
//
// cAdvisor labels these `namespace` and `pod`, where span metrics use
// k8s_namespace_name and k8s_pod_name. The names differ by origin, and the
// join happens here so no recording rule has to exist for it.
func (c *Client) PodResources(ctx context.Context, namespace string) (map[string]*PodUsage, error) {
	w := model.Duration(c.window).String()
	out := map[string]*PodUsage{}

	cpu, err := c.byPod(ctx, fmt.Sprintf(
		`sum by (pod) (rate(container_cpu_usage_seconds_total{namespace=%q,container!=""}[%s]))`,
		namespace, w))
	if err != nil {
		return nil, fmt.Errorf("pod cpu: %w", err)
	}
	for pod, v := range cpu {
		out[pod] = &PodUsage{CPUMillis: v * 1000}
	}

	mem, err := c.byPod(ctx, fmt.Sprintf(
		`sum by (pod) (container_memory_working_set_bytes{namespace=%q,container!=""})`, namespace))
	if err != nil {
		return nil, fmt.Errorf("pod memory: %w", err)
	}
	for pod, v := range mem {
		if u, ok := out[pod]; ok {
			u.MemBytes = v
		} else {
			out[pod] = &PodUsage{MemBytes: v}
		}
	}
	return out, nil
}

func (c *Client) byPod(ctx context.Context, query string) (map[string]float64, error) {
	val, _, err := c.api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}
	vec, ok := val.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("expected a vector, got %s", val.Type())
	}
	out := make(map[string]float64, len(vec))
	for _, s := range vec {
		out[string(s.Metric["pod"])] = float64(s.Value)
	}
	return out, nil
}
