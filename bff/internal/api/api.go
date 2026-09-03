// Package api serves the JSON the UI consumes.
//
// Every response is assembled around service identity, which is what the whole
// product rests on. See docs/join-key.md.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/NaviteLogger/Inside-Man/bff/internal/config"
	"github.com/NaviteLogger/Inside-Man/bff/internal/health"
	"github.com/NaviteLogger/Inside-Man/bff/internal/kube"
	"github.com/NaviteLogger/Inside-Man/bff/internal/logs"
	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
	"github.com/NaviteLogger/Inside-Man/bff/internal/traces"
)

type Server struct {
	cfg   config.Config
	prom  *promql.Client
	tempo *traces.Client
	loki  *logs.Client
	cache *kube.Cache
	log   *slog.Logger
}

func NewServer(cfg config.Config, prom *promql.Client, tempo *traces.Client, loki *logs.Client,
	cache *kube.Cache, log *slog.Logger) *Server {
	return &Server{cfg: cfg, prom: prom, tempo: tempo, loki: loki, cache: cache, log: log}
}

func (s *Server) thresholds() health.Thresholds {
	return health.Thresholds{
		ErrorRateWarn: s.cfg.ErrorRateWarn,
		ErrorRateCrit: s.cfg.ErrorRateCrit,
		P95Warn:       s.cfg.P95Warn,
	}
}

// grafanaLinks builds deep links into Grafana's bundled Drilldown apps. We do
// not rebuild a trace waterfall or a log explorer, see
// docs/decisions/0005-embed-grafana-drilldown-for-traces-and-logs.md.
func (s *Server) grafanaLinks(service string) map[string]string {
	base := strings.TrimSuffix(s.cfg.GrafanaURL, "/")
	logQL := url.QueryEscape(fmt.Sprintf("{service_name=%q}", service))
	traceQL := url.QueryEscape(fmt.Sprintf("{resource.service.name=%q}", service))

	return map[string]string{
		"logs":   fmt.Sprintf("%s/a/grafana-lokiexplore-app/explore?var-ds=loki&var-filters=service_name%%7C%%3D%%7C%s", base, url.QueryEscape(service)),
		"traces": fmt.Sprintf("%s/a/grafana-exploretraces-app/explore?var-ds=tempo&var-filters=resource.service.name%%7C%%3D%%7C%s", base, url.QueryEscape(service)),
		// Explore keeps working whatever happens to the Drilldown URL formats,
		// so it is the stable fallback.
		"exploreLogs":   fmt.Sprintf(`%s/explore?left={"datasource":"loki","queries":[{"expr":"%s"}]}`, base, logQL),
		"exploreTraces": fmt.Sprintf(`%s/explore?left={"datasource":"tempo","queries":[{"query":"%s","queryType":"traceql"}]}`, base, traceQL),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /api/services", s.handleServices)
	mux.HandleFunc("GET /api/services/{name}", s.handleServiceDetail)
	mux.HandleFunc("GET /api/services/{name}/logs", s.handleServiceLogs)
	mux.HandleFunc("GET /api/services/{name}/traces", s.handleServiceTraces)
	mux.HandleFunc("GET /api/traces/{traceID}/logs", s.handleTraceLogs)
	mux.HandleFunc("GET /api/diagnostics", s.handleDiagnostics)
	return mux
}

// Service is one row of the home screen.
type Service struct {
	Name        string         `json:"name"`
	Namespace   string         `json:"namespace"`
	Health      health.Result  `json:"health"`
	RequestRate float64        `json:"requestRate"`
	ErrorRatio  float64        `json:"errorRatio"`
	P95Millis   float64        `json:"p95Millis"`
	Sparkline   []float64      `json:"sparkline,omitempty"`
	Workload    *kube.Workload `json:"workload,omitempty"`
}

type servicesResponse struct {
	Services []Service `json:"services"`
	Window   string    `json:"window"`
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	red, err := s.prom.ServiceRED(ctx)
	if err != nil {
		s.fail(w, "querying service metrics", err)
		return
	}

	// A sparkline failure should not take down the screen, so it degrades to
	// rows without one.
	spark, err := s.prom.Sparkline(ctx, time.Hour, 30)
	if err != nil {
		s.log.Warn("sparkline unavailable", "err", err)
		spark = map[promql.Key][]float64{}
	}

	thresholds := s.thresholds()

	out := make([]Service, 0, len(red))
	for key, m := range red {
		if key.Service == "" {
			continue
		}
		svc := Service{
			Name:        key.Service,
			Namespace:   key.Namespace,
			RequestRate: m.RequestRate,
			ErrorRatio:  m.ErrorRatio,
			P95Millis:   float64(m.P95) / float64(time.Millisecond),
			Sparkline:   spark[key],
		}

		in := health.Input{
			HasSpans:  m.RequestRate > 0,
			ErrorRate: m.ErrorRatio,
			P95:       m.P95,
		}

		// The Kubernetes half is best-effort. A service can report spans from
		// outside the cluster, and the screen still has something to show.
		if key.Namespace != "" {
			if wl, err := s.cache.Lookup(key.Namespace, key.Service); err == nil {
				svc.Workload = wl
				in.SLOP95 = wl.SLOP95
			}
		}

		svc.Health = health.Evaluate(in, thresholds)
		out = append(out, svc)
	}

	// Worst first, which is the point of the screen. Ties break by name so the
	// order is stable between polls.
	sort.Slice(out, func(i, j int) bool {
		if a, b := out[i].Health.Status.Rank(), out[j].Health.Status.Rank(); a != b {
			return a < b
		}
		if out[i].ErrorRatio != out[j].ErrorRatio {
			return out[i].ErrorRatio > out[j].ErrorRatio
		}
		return out[i].Name < out[j].Name
	})

	writeJSON(w, http.StatusOK, servicesResponse{Services: out, Window: s.cfg.Window.String()})
}

func (s *Server) fail(w http.ResponseWriter, what string, err error) {
	s.log.Error(what, "err", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": what})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status line is already sent, so logging is all that is left.
		slog.Error("writing response", "err", err)
	}
}
