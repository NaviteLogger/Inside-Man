package api

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
)

// Check is one line on the diagnostics page. When a screen is unexpectedly
// empty this is the first place to look, which is why it ships in M2 rather
// than waiting for the hardening milestone.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pass, warn or fail
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

type diagnosticsResponse struct {
	Checks    []Check `json:"checks"`
	CheckedAt string  `json:"checkedAt"`
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	checks := []Check{s.checkPrometheus(ctx)}

	red, err := s.prom.ServiceRED(ctx)
	if err != nil {
		checks = append(checks, Check{
			Name:   "span metrics",
			Status: "fail",
			Detail: "querying span metrics failed: " + err.Error(),
			Hint:   "Check that Alloy's spanMetrics connector is enabled and writing to Prometheus.",
		})
	} else {
		checks = append(checks, spanMetricChecks(red)...)
		checks = append(checks, s.checkWorkloadJoin(red))
	}

	writeJSON(w, http.StatusOK, diagnosticsResponse{
		Checks:    checks,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) checkPrometheus(ctx context.Context) Check {
	if err := s.prom.Up(ctx); err != nil {
		return Check{
			Name:   "metrics store reachable",
			Status: "fail",
			Detail: err.Error(),
			Hint:   "Check PROMETHEUS_URL and that the metrics store is running.",
		}
	}
	return Check{Name: "metrics store reachable", Status: "pass", Detail: s.cfg.PrometheusURL}
}

func spanMetricChecks(red map[promql.Key]*promql.RED) []Check {
	if len(red) == 0 {
		return []Check{{
			Name:   "span metrics present",
			Status: "fail",
			Detail: "no service is producing span metrics",
			Hint:   "Annotate a pod with instrumentation.opentelemetry.io/inject-<language> and send it traffic.",
		}}
	}

	var unnamed, noNamespace []string
	for k := range red {
		switch {
		case k.Service == "":
			unnamed = append(unnamed, "(empty)")
		case k.Namespace == "":
			noNamespace = append(noNamespace, k.Service)
		}
	}
	sort.Strings(noNamespace)

	checks := []Check{{
		Name:   "span metrics present",
		Status: "pass",
		Detail: plural(len(red), "service") + " reporting span metrics",
	}}

	// A service with no k8s.namespace.name never reached the k8sattributes
	// processor, so its pods, logs and alerts cannot be joined to it.
	if len(noNamespace) > 0 {
		checks = append(checks, Check{
			Name:   "kubernetes attributes attached",
			Status: "warn",
			Detail: plural(len(noNamespace), "service") + " have no k8s_namespace_name label: " + join(noNamespace),
			Hint:   "These services cannot be joined to pods or logs. Check the k8sattributes processor in the collector.",
		})
	} else {
		checks = append(checks, Check{
			Name:   "kubernetes attributes attached",
			Status: "pass",
			Detail: "every reporting service carries k8s_namespace_name",
		})
	}

	if len(unnamed) > 0 {
		checks = append(checks, Check{
			Name:   "service identity set",
			Status: "fail",
			Detail: "span metrics exist with an empty service_name",
			Hint:   "Something is exporting spans without service.name. See docs/join-key.md.",
		})
	}
	return checks
}

// checkWorkloadJoin reports how many reporting services resolve to a workload,
// which is the join the service detail screen depends on.
func (s *Server) checkWorkloadJoin(red map[promql.Key]*promql.RED) Check {
	var matched int
	var missing []string
	for k := range red {
		if k.Service == "" || k.Namespace == "" {
			continue
		}
		if _, err := s.cache.Lookup(k.Namespace, k.Service); err == nil {
			matched++
		} else {
			missing = append(missing, k.Namespace+"/"+k.Service)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		return Check{
			Name:   "services resolve to workloads",
			Status: "warn",
			Detail: plural(matched, "service") + " matched, no Deployment found for " + join(missing),
			Hint:   "service.name should equal the Deployment name. See docs/decisions/0004-service-name-from-workload.md.",
		}
	}
	return Check{
		Name:   "services resolve to workloads",
		Status: "pass",
		Detail: plural(matched, "service") + " matched a Deployment",
	}
}
