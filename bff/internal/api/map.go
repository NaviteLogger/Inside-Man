package api

import (
	"context"
	"net/http"
	"sort"

	"github.com/NaviteLogger/Inside-Man/bff/internal/alerts"
	"github.com/NaviteLogger/Inside-Man/bff/internal/health"
	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
)

// MapNode is one service on the map, coloured by the same health the services
// list uses so the two screens never disagree.
type MapNode struct {
	Name        string        `json:"name"`
	Namespace   string        `json:"namespace,omitempty"`
	Health      health.Result `json:"health"`
	RequestRate float64       `json:"requestRate"`
	ErrorRatio  float64       `json:"errorRatio"`
}

type mapResponse struct {
	Nodes  []MapNode     `json:"nodes"`
	Edges  []promql.Edge `json:"edges"`
	Window string        `json:"window"`
}

func (s *Server) handleMap(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	edges, err := s.prom.AllEdges(ctx)
	if err != nil {
		s.fail(w, "querying the service graph", err)
		return
	}

	red, err := s.prom.ServiceRED(ctx)
	if err != nil {
		s.fail(w, "querying service metrics", err)
		return
	}

	firing := s.firingByService(ctx)
	thresholds := s.thresholds()

	nodes := make([]MapNode, 0, len(red))
	seen := map[string]bool{}
	for key, m := range red {
		if key.Service == "" {
			continue
		}
		seen[key.Service] = true
		in := health.Input{HasSpans: m.RequestRate > 0, ErrorRate: m.ErrorRatio, P95: m.P95}
		if key.Namespace != "" {
			if wl, err := s.cache.Lookup(key.Namespace, key.Service); err == nil {
				in.SLOP95 = wl.SLOP95
			}
		}
		applyAlerts(&in, firing[key.Service])

		nodes = append(nodes, MapNode{
			Name:        key.Service,
			Namespace:   key.Namespace,
			Health:      health.Evaluate(in, thresholds),
			RequestRate: m.RequestRate,
			ErrorRatio:  m.ErrorRatio,
		})
	}

	// The graph names services that never reported span metrics of their own:
	// a database, a third party, or Tempo's virtual "user" node for a request
	// nobody traced. They belong on the map, with no health to show.
	for _, e := range edges {
		for _, name := range []string{e.Client, e.Server} {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			nodes = append(nodes, MapNode{
				Name:   name,
				Health: health.Result{Status: health.Unknown, Reasons: []string{"no spans of its own in the window"}},
			})
		}
	}

	sort.Slice(nodes, func(i, j int) bool {
		if a, b := nodes[i].Health.Status.Rank(), nodes[j].Health.Status.Rank(); a != b {
			return a < b
		}
		return nodes[i].Name < nodes[j].Name
	})

	writeJSON(w, http.StatusOK, mapResponse{Nodes: nodes, Edges: edges, Window: s.cfg.Window.String()})
}

type alertsResponse struct {
	Alerts    []alerts.Alert            `json:"alerts"`
	ByService map[string][]alerts.Alert `json:"byService"`
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.QueryTimeout)
	defer cancel()

	firing, err := s.alerts.Firing(ctx)
	if err != nil {
		s.fail(w, "querying alerts", err)
		return
	}
	if firing == nil {
		firing = []alerts.Alert{}
	}
	writeJSON(w, http.StatusOK, alertsResponse{Alerts: firing, ByService: alerts.ByService(firing)})
}

// firingByService fetches alerts for folding into health. Alertmanager being
// down should cost the caller its alerts and nothing else, so the error is
// logged and the result is empty.
func (s *Server) firingByService(ctx context.Context) map[string][]alerts.Alert {
	firing, err := s.alerts.Firing(ctx)
	if err != nil {
		s.log.Warn("alerts unavailable", "err", err)
		return map[string][]alerts.Alert{}
	}
	return alerts.ByService(firing)
}

// applyAlerts folds firing alerts into a health input. Design doc 5.2 makes an
// alert decisive: someone declared this condition worth paging on, so it beats
// whatever the metrics say.
func applyAlerts(in *health.Input, firing []alerts.Alert) {
	switch alerts.Worst(firing) {
	case "critical":
		in.FiringCritical = true
	case "warning":
		in.FiringWarning = true
	}
}
