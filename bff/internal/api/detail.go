package api

import (
	"context"
	"net/http"
	"time"

	"github.com/NaviteLogger/Inside-Man/bff/internal/health"
	"github.com/NaviteLogger/Inside-Man/bff/internal/kube"
	"github.com/NaviteLogger/Inside-Man/bff/internal/logs"
	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
	"github.com/NaviteLogger/Inside-Man/bff/internal/traces"
)

// ServiceDetail is everything the detail screen shows for one service.
type ServiceDetail struct {
	Name        string                      `json:"name"`
	Namespace   string                      `json:"namespace"`
	Health      health.Result               `json:"health"`
	RequestRate float64                     `json:"requestRate"`
	ErrorRatio  float64                     `json:"errorRatio"`
	P95Millis   float64                     `json:"p95Millis"`
	Workload    *kube.Workload              `json:"workload,omitempty"`
	Resources   map[string]*promql.PodUsage `json:"resources,omitempty"`
	Inbound     []promql.Edge               `json:"inbound"`
	Outbound    []promql.Edge               `json:"outbound"`
	ErrorTraces []traces.Summary            `json:"errorTraces"`
	Links       map[string]string           `json:"links"`
	Window      string                      `json:"window"`
}

func (s *Server) handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	namespace := r.URL.Query().Get("namespace")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a service name is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	red, err := s.prom.ServiceRED(ctx)
	if err != nil {
		s.fail(w, "querying service metrics", err)
		return
	}

	key := promql.Key{Service: name, Namespace: namespace}
	m, ok := red[key]
	if !ok {
		// The caller may not know the namespace, so fall back to the first
		// service with a matching name.
		for k, v := range red {
			if k.Service == name {
				key, m, ok = k, v, true
				break
			}
		}
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "no span metrics for service " + name,
			"hint":  "The service may not be instrumented, or may have had no traffic in the window.",
		})
		return
	}

	detail := ServiceDetail{
		Name:        key.Service,
		Namespace:   key.Namespace,
		RequestRate: m.RequestRate,
		ErrorRatio:  m.ErrorRatio,
		P95Millis:   float64(m.P95) / float64(time.Millisecond),
		Inbound:     []promql.Edge{},
		Outbound:    []promql.Edge{},
		ErrorTraces: []traces.Summary{},
		Window:      s.cfg.Window.String(),
	}

	in := health.Input{HasSpans: m.RequestRate > 0, ErrorRate: m.ErrorRatio, P95: m.P95}

	if key.Namespace != "" {
		if wl, err := s.cache.Lookup(key.Namespace, key.Service); err == nil {
			detail.Workload = wl
			in.SLOP95 = wl.SLOP95
		}
		// Resource usage is a nice-to-have. A missing cAdvisor should not cost
		// the screen its topology or its traces.
		if usage, err := s.prom.PodResources(ctx, key.Namespace); err == nil {
			// The query is per namespace, since cAdvisor has no notion of a
			// service. Narrow it to this workload's pods so the screen does not
			// show a neighbour's memory as its own.
			detail.Resources = forPods(usage, detail.Workload)
		} else {
			s.log.Warn("pod resources unavailable", "namespace", key.Namespace, "err", err)
		}
	}

	detail.Health = health.Evaluate(in, s.thresholds())

	if inbound, outbound, err := s.prom.Dependencies(ctx, key.Service); err == nil {
		detail.Inbound, detail.Outbound = orEmpty(inbound), orEmpty(outbound)
	} else {
		s.log.Warn("dependencies unavailable", "service", key.Service, "err", err)
	}

	if t, err := s.tempo.ErrorTraces(ctx, key.Service, time.Hour, 10); err == nil {
		detail.ErrorTraces = t
	} else {
		s.log.Warn("error traces unavailable", "service", key.Service, "err", err)
	}

	detail.Links = s.grafanaLinks(key.Service)
	writeJSON(w, http.StatusOK, detail)
}

// handleTraceLogs answers "logs for this trace", the pivot the product rests on.
func (s *Server) handleTraceLogs(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceID")
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	lines, err := s.loki.ForTrace(ctx, traceID, 6*time.Hour, 200)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"traceId": traceID,
		"lines":   orEmptyLines(lines),
	})
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	span := 15 * time.Minute
	var (
		lines []logs.Line
		err   error
	)
	if traceID := r.URL.Query().Get("traceId"); traceID != "" {
		lines, err = s.loki.ForServiceAndTrace(ctx, name, traceID, 6*time.Hour, 200)
	} else {
		lines, err = s.loki.ForService(ctx, name, span, 200)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": name,
		"lines":   orEmptyLines(lines),
	})
}

func (s *Server) handleServiceTraces(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	var (
		found []traces.Summary
		err   error
	)
	if r.URL.Query().Get("status") == "error" {
		found, err = s.tempo.ErrorTraces(ctx, name, time.Hour, 20)
	} else {
		found, err = s.tempo.Search(ctx, `{resource.service.name="`+name+`"}`, time.Hour, 20)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if found == nil {
		found = []traces.Summary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": name, "traces": found})
}

func orEmpty(e []promql.Edge) []promql.Edge {
	if e == nil {
		return []promql.Edge{}
	}
	return e
}

func orEmptyLines(l []logs.Line) []logs.Line {
	if l == nil {
		return []logs.Line{}
	}
	return l
}

// forPods keeps only the usage belonging to the given workload's pods.
func forPods(usage map[string]*promql.PodUsage, w *kube.Workload) map[string]*promql.PodUsage {
	if w == nil {
		return nil
	}
	out := make(map[string]*promql.PodUsage, len(w.Pods))
	for _, p := range w.Pods {
		if u, ok := usage[p.Name]; ok {
			out[p.Name] = u
		}
	}
	return out
}
