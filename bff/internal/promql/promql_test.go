package promql

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stub answers Prometheus queries by matching on the query text, and records
// what it was asked so the queries themselves can be asserted.
type stub struct {
	queries  []string
	response func(query string) string
}

func newClient(t *testing.T, respond func(string) string) (*Client, *stub) {
	t.Helper()
	s := &stub{response: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		q := r.FormValue("query")
		s.queries = append(s.queries, q)
		w.Header().Set("content-type", "application/json")
		io.WriteString(w, s.response(q))
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, 5*time.Minute)
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return c, s
}

func vector(samples ...[3]string) string {
	var b strings.Builder
	b.WriteString(`{"status":"success","data":{"resultType":"vector","result":[`)
	for i, s := range samples {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"metric":{"service_name":"` + s[0] + `","k8s_namespace_name":"` + s[1] +
			`"},"value":[1700000000,"` + s[2] + `"]}`)
	}
	b.WriteString(`]}}`)
	return b.String()
}

func edgeVector(rows ...[3]string) string {
	var b strings.Builder
	b.WriteString(`{"status":"success","data":{"resultType":"vector","result":[`)
	for i, r := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"metric":{"client":"` + r[0] + `","server":"` + r[1] +
			`"},"value":[1700000000,"` + r[2] + `"]}`)
	}
	b.WriteString(`]}}`)
	return b.String()
}

func podVector(rows ...[2]string) string {
	var b strings.Builder
	b.WriteString(`{"status":"success","data":{"resultType":"vector","result":[`)
	for i, r := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"metric":{"pod":"` + r[0] + `"},"value":[1700000000,"` + r[1] + `"]}`)
	}
	b.WriteString(`]}}`)
	return b.String()
}

func TestServiceREDComputesErrorRatio(t *testing.T) {
	c, _ := newClient(t, func(q string) string {
		switch {
		case strings.Contains(q, "STATUS_CODE_ERROR"):
			return vector([3]string{"checkout", "shop", "0.5"})
		case strings.Contains(q, "histogram_quantile"):
			return vector([3]string{"checkout", "shop", "0.25"})
		default:
			return vector(
				[3]string{"checkout", "shop", "2"},
				[3]string{"catalog", "shop", "8"})
		}
	})

	red, err := c.ServiceRED(context.Background())
	if err != nil {
		t.Fatalf("ServiceRED: %v", err)
	}

	checkout := red[Key{Service: "checkout", Namespace: "shop"}]
	if checkout == nil {
		t.Fatal("checkout missing")
	}
	// 0.5 failing of 2 total is a quarter.
	if checkout.ErrorRatio != 0.25 {
		t.Errorf("error ratio: got %v, want 0.25", checkout.ErrorRatio)
	}
	if checkout.P95 != 250*time.Millisecond {
		t.Errorf("p95: got %v, want 250ms", checkout.P95)
	}

	// A service with traffic and no error series has a zero ratio, which is
	// different from having no data.
	catalog := red[Key{Service: "catalog", Namespace: "shop"}]
	if catalog == nil || catalog.ErrorRatio != 0 {
		t.Errorf("catalog error ratio: got %+v, want 0", catalog)
	}
}

func TestServiceREDIgnoresNaNQuantile(t *testing.T) {
	// histogram_quantile yields NaN for a histogram with no observations, and a
	// NaN duration would render as a nonsense p95.
	c, _ := newClient(t, func(q string) string {
		if strings.Contains(q, "histogram_quantile") {
			return vector([3]string{"checkout", "shop", "NaN"})
		}
		if strings.Contains(q, "STATUS_CODE_ERROR") {
			return vector()
		}
		return vector([3]string{"checkout", "shop", "1"})
	})

	red, err := c.ServiceRED(context.Background())
	if err != nil {
		t.Fatalf("ServiceRED: %v", err)
	}
	got := red[Key{Service: "checkout", Namespace: "shop"}]
	if got.P95 != 0 {
		t.Errorf("p95 from NaN: got %v, want 0", got.P95)
	}
	if math.IsNaN(float64(got.P95)) {
		t.Error("p95 is NaN, which would reach the UI as a broken number")
	}
}

func TestServiceREDUsesTheVerifiedNames(t *testing.T) {
	// These were verified against a live cluster and differ from design doc 4.4,
	// which assumed Tempo's conventions. See docs/join-key.md.
	c, s := newClient(t, func(string) string { return vector() })
	if _, err := c.ServiceRED(context.Background()); err != nil {
		t.Fatalf("ServiceRED: %v", err)
	}
	all := strings.Join(s.queries, "\n")

	for _, want := range []string{
		"traces_spanmetrics_calls_total",
		"traces_spanmetrics_duration_seconds_bucket",
		"by (service_name, k8s_namespace_name)",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no query contained %q\n%s", want, all)
		}
	}
	for _, unwanted := range []string{"traces_spanmetrics_latency", "by (service,"} {
		if strings.Contains(all, unwanted) {
			t.Errorf("query used Tempo's convention %q", unwanted)
		}
	}
}

func TestDependenciesSplitsInboundFromOutbound(t *testing.T) {
	c, _ := newClient(t, func(q string) string {
		if strings.Contains(q, "request_failed_total") {
			return edgeVector([3]string{"frontend", "api", "0.2"})
		}
		return edgeVector(
			[3]string{"frontend", "api", "2"},
			[3]string{"api", "backend", "1"},
			[3]string{"other", "unrelated", "5"})
	})

	in, out, err := c.Dependencies(context.Background(), "api")
	if err != nil {
		t.Fatalf("Dependencies: %v", err)
	}
	if len(in) != 1 || in[0].Client != "frontend" {
		t.Fatalf("inbound: got %+v, want one edge from frontend", in)
	}
	if in[0].ErrorRatio != 0.1 {
		t.Errorf("inbound error ratio: got %v, want 0.1", in[0].ErrorRatio)
	}
	if len(out) != 1 || out[0].Server != "backend" {
		t.Fatalf("outbound: got %+v, want one edge to backend", out)
	}
	// An edge touching neither side of this service has no business appearing.
	for _, e := range append(in, out...) {
		if e.Client == "other" || e.Server == "unrelated" {
			t.Errorf("unrelated edge leaked in: %+v", e)
		}
	}
}

func TestDependenciesSurvivesMissingFailureSeries(t *testing.T) {
	// Failure counts are a refinement. A cluster that has never had a failing
	// span still has to render its topology.
	c, _ := newClient(t, func(q string) string {
		if strings.Contains(q, "request_failed_total") {
			return `{"status":"error","errorType":"bad_data","error":"no such metric"}`
		}
		return edgeVector([3]string{"frontend", "api", "2"})
	})

	in, _, err := c.Dependencies(context.Background(), "api")
	if err != nil {
		t.Fatalf("Dependencies should tolerate a missing failure series: %v", err)
	}
	if len(in) != 1 || in[0].ErrorRatio != 0 {
		t.Fatalf("got %+v, want one edge with a zero error ratio", in)
	}
}

func TestAllEdgesSortsByBusiest(t *testing.T) {
	c, _ := newClient(t, func(string) string {
		return edgeVector(
			[3]string{"a", "b", "1"},
			[3]string{"c", "d", "9"},
			[3]string{"e", "f", "5"})
	})

	edges, err := c.AllEdges(context.Background())
	if err != nil {
		t.Fatalf("AllEdges: %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("got %d edges, want 3", len(edges))
	}
	if edges[0].RequestRate != 9 || edges[2].RequestRate != 1 {
		t.Errorf("edges are not busiest first: %+v", edges)
	}
}

func TestPodResourcesJoinsCPUAndMemory(t *testing.T) {
	c, s := newClient(t, func(q string) string {
		if strings.Contains(q, "container_memory_working_set_bytes") {
			return podVector([2]string{"pod-a", "104857600"}, [2]string{"pod-c", "1024"})
		}
		return podVector([2]string{"pod-a", "0.05"}, [2]string{"pod-b", "0.1"})
	})

	usage, err := c.PodResources(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PodResources: %v", err)
	}

	if got := usage["pod-a"]; got == nil || got.CPUMillis != 50 || got.MemBytes != 104857600 {
		t.Errorf("pod-a: got %+v, want 50m and 100MiB", got)
	}
	// A pod with CPU and no memory sample, and one with memory and no CPU, both
	// have to survive the join.
	if got := usage["pod-b"]; got == nil || got.CPUMillis != 100 || got.MemBytes != 0 {
		t.Errorf("pod-b: got %+v, want 100m and no memory", got)
	}
	if got := usage["pod-c"]; got == nil || got.MemBytes != 1024 {
		t.Errorf("pod-c: got %+v, want memory only", got)
	}

	// cAdvisor labels differ from the span metrics, so the namespace has to be
	// applied with its own label name.
	all := strings.Join(s.queries, "\n")
	if !strings.Contains(all, `namespace="demo"`) {
		t.Errorf("namespace filter missing from cAdvisor queries:\n%s", all)
	}
	if strings.Contains(all, "k8s_namespace_name") {
		t.Errorf("cAdvisor queried with the span metrics label name:\n%s", all)
	}
}

func TestQueryErrorsAreReported(t *testing.T) {
	c, _ := newClient(t, func(string) string {
		return `{"status":"error","errorType":"bad_data","error":"boom"}`
	})
	if _, err := c.ServiceRED(context.Background()); err == nil {
		t.Fatal("a failing query has to surface as an error")
	}
}
