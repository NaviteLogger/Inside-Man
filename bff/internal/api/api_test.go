package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/NaviteLogger/Inside-Man/bff/internal/config"
	"github.com/NaviteLogger/Inside-Man/bff/internal/kube"
	"github.com/NaviteLogger/Inside-Man/bff/internal/logs"
	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
	"github.com/NaviteLogger/Inside-Man/bff/internal/traces"
)

// vector builds a Prometheus instant-query response body.
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

// stubProm records the queries it is asked and answers by matching on them, so
// a wrong metric or label name shows up as a test failure.
type stubProm struct {
	queries []string
}

func (s *stubProm) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		q := r.FormValue("query")
		s.queries = append(s.queries, q)
		w.Header().Set("content-type", "application/json")

		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/query_range"):
			io.WriteString(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
		case strings.Contains(q, "STATUS_CODE_ERROR"):
			io.WriteString(w, vector([3]string{"checkout", "shop", "0.5"}))
		case strings.Contains(q, "histogram_quantile"):
			io.WriteString(w, vector(
				[3]string{"checkout", "shop", "0.9"},
				[3]string{"catalog", "shop", "0.01"}))
		default:
			io.WriteString(w, vector(
				[3]string{"checkout", "shop", "2"},
				[3]string{"catalog", "shop", "10"}))
		}
	})
}

func newTestServer(t *testing.T) (*Server, *stubProm) {
	t.Helper()

	stub := &stubProm{}
	prom := httptest.NewServer(stub.handler())
	t.Cleanup(prom.Close)

	client, err := promql.New(prom.URL, 5*time.Minute)
	if err != nil {
		t.Fatalf("promql client: %v", err)
	}

	replicas := int32(2)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "shop"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}},
			Template: corev1.PodTemplateSpec{},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cache, err := kube.NewCacheFromClient(ctx, fake.NewSimpleClientset(dep), 0)
	if err != nil {
		t.Fatalf("kube cache: %v", err)
	}

	cfg := config.Config{
		Window: 5 * time.Minute, QueryTimeout: 5 * time.Second,
		ErrorRateWarn: 0.01, ErrorRateCrit: 0.05, P95Warn: 500 * time.Millisecond,
		PrometheusURL: prom.URL,
	}
	return NewServer(cfg, client,
		traces.New(prom.URL, 5*time.Second),
		logs.New(prom.URL, 5*time.Second, "inside-man"),
		cache, slog.New(slog.DiscardHandler)), stub
}

func TestServicesRanksWorstFirst(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var got servicesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Services) != 2 {
		t.Fatalf("want 2 services, got %d", len(got.Services))
	}

	// checkout runs at 2 req/s with 0.5 of them failing, so 25% errors, past
	// the 5% critical threshold. It has to sort above the healthy service.
	first := got.Services[0]
	if first.Name != "checkout" {
		t.Fatalf("want the broken service first, got %q", first.Name)
	}
	if first.Health.Status != "critical" {
		t.Fatalf("want checkout critical, got %s", first.Health.Status)
	}
	if len(first.Health.Reasons) == 0 {
		t.Fatal("a critical service must explain itself")
	}
	if first.Workload == nil || first.Workload.Ready != 2 {
		t.Fatalf("want the Deployment joined onto the row, got %+v", first.Workload)
	}
	if got.Services[1].Health.Status != "healthy" {
		t.Fatalf("want catalog healthy, got %s", got.Services[1].Health.Status)
	}
}

// The metric and label names were verified against a live cluster and differ
// from the design doc. Pinning them here turns a regression into a failing unit
// test, well before anyone sees an empty screen.
func TestServicesUsesVerifiedMetricNames(t *testing.T) {
	srv, stub := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services", nil))

	all := strings.Join(stub.queries, "\n")
	for _, want := range []string{
		"traces_spanmetrics_calls_total",
		"traces_spanmetrics_duration_seconds_bucket",
		"service_name",
		"k8s_namespace_name",
		`status_code="STATUS_CODE_ERROR"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no query mentioned %q\nqueries:\n%s", want, all)
		}
	}
	for _, unwanted := range []string{"traces_spanmetrics_latency", "by (service,"} {
		if strings.Contains(all, unwanted) {
			t.Errorf("query used %q, which is Tempo's convention and not Alloy's", unwanted)
		}
	}
}

func TestDiagnosticsReportsJoinHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got diagnosticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	byName := map[string]Check{}
	for _, c := range got.Checks {
		byName[c.Name] = c
	}

	if c := byName["span metrics present"]; c.Status != "pass" {
		t.Errorf("want span metrics pass, got %+v", c)
	}
	// catalog reports metrics but has no Deployment, so the join is incomplete
	// and diagnostics has to say so.
	c := byName["services resolve to workloads"]
	if c.Status != "warn" || !strings.Contains(c.Detail, "shop/catalog") {
		t.Errorf("want a warning naming shop/catalog, got %+v", c)
	}
}
