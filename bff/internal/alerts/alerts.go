// Package alerts reads firing alerts from Alertmanager's v2 API.
//
// Design doc 4.4 puts active alerts here, filtered by a service label. In the
// small profile Prometheus evaluates the rules and routes them to the bundled
// Alertmanager; in the ha profile Mimir's ruler does the evaluating. Both end
// up at the same v2 API, so this one client covers both.
package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SeverityLabel is the conventional label alert rules carry, and the one our
// baseline rules set.
const SeverityLabel = "severity"

// ServiceLabels are the labels an alert might use to name a service, in the
// order they are trusted. service_name matches the join key, so it wins.
var ServiceLabels = []string{"service_name", "service", "job"}

type Client struct {
	base string
	http *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: timeout}}
}

// Alert is one firing alert, reduced to what the Issues screen shows.
type Alert struct {
	Name        string            `json:"name"`
	Service     string            `json:"service"`
	Namespace   string            `json:"namespace,omitempty"`
	Severity    string            `json:"severity"`
	Summary     string            `json:"summary,omitempty"`
	Description string            `json:"description,omitempty"`
	StartsAt    time.Time         `json:"startsAt"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type apiAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	Status      struct {
		State string `json:"state"`
	} `json:"status"`
}

// Firing returns active alerts, newest first, with silenced and inhibited ones
// left out, since someone suppressed those knowingly.
func (c *Client) Firing(ctx context.Context) ([]Alert, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/api/v2/alerts?active=true&silenced=false&inhibited=false", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alertmanager: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alertmanager returned %s", res.Status)
	}

	var body []apiAlert
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("alertmanager response: %w", err)
	}

	out := make([]Alert, 0, len(body))
	for _, a := range body {
		if a.Status.State == "suppressed" {
			continue
		}
		out = append(out, Alert{
			Name:        a.Labels["alertname"],
			Service:     serviceOf(a.Labels),
			Namespace:   a.Labels["k8s_namespace_name"],
			Severity:    strings.ToLower(a.Labels[SeverityLabel]),
			Summary:     a.Annotations["summary"],
			Description: a.Annotations["description"],
			StartsAt:    a.StartsAt,
			Labels:      a.Labels,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].Name < out[j].Name
		}
		return out[i].StartsAt.After(out[j].StartsAt)
	})
	return out, nil
}

// Up reports whether Alertmanager answers, for the diagnostics page.
func (c *Client) Up(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v2/status", nil)
	if err != nil {
		return err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("alertmanager: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("alertmanager returned %s", res.Status)
	}
	return nil
}

// serviceOf picks the label naming the service. An alert with none is still
// worth showing, so it lands under an empty service and the UI groups it as
// cluster-wide.
func serviceOf(labels map[string]string) string {
	for _, key := range ServiceLabels {
		if v := labels[key]; v != "" {
			return v
		}
	}
	return ""
}

// ByService groups alerts for the Issues screen, which lists them per service.
func ByService(all []Alert) map[string][]Alert {
	out := map[string][]Alert{}
	for _, a := range all {
		out[a.Service] = append(out[a.Service], a)
	}
	return out
}

// Worst reports the highest severity among the given alerts, so the services
// list can fold a firing alert into a service's health.
func Worst(all []Alert) string {
	worst := ""
	for _, a := range all {
		switch a.Severity {
		case "critical":
			return "critical"
		case "warning":
			worst = "warning"
		}
	}
	return worst
}
