package alerts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newClient(t *testing.T, body string, status int) (*Client, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.String())
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("content-type", "application/json")
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, 5*time.Second), &paths
}

func TestFiringMapsLabelsAndAnnotations(t *testing.T) {
	c, _ := newClient(t, `[
	  {"labels":{"alertname":"HighErrorRate","service_name":"checkout",
	             "k8s_namespace_name":"shop","severity":"critical"},
	   "annotations":{"summary":"checkout is failing","description":"error rate above 5%"},
	   "startsAt":"2026-09-03T10:00:00Z","status":{"state":"active"}}
	]`, http.StatusOK)

	got, err := c.Firing(context.Background())
	if err != nil {
		t.Fatalf("Firing: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d alerts, want 1", len(got))
	}
	a := got[0]
	if a.Name != "HighErrorRate" || a.Service != "checkout" ||
		a.Namespace != "shop" || a.Severity != "critical" {
		t.Errorf("mapped wrong: %+v", a)
	}
	if a.Summary == "" || a.Description == "" {
		t.Error("annotations dropped, so the screen would show an alert with no explanation")
	}
}

func TestFiringPrefersTheJoinKeyLabel(t *testing.T) {
	// service_name is the join key, so it wins over the looser labels an
	// off-the-shelf rule might carry.
	c, _ := newClient(t, `[
	  {"labels":{"alertname":"A","service_name":"checkout","service":"other","job":"third"},
	   "annotations":{},"startsAt":"2026-09-03T10:00:00Z","status":{"state":"active"}},
	  {"labels":{"alertname":"B","service":"catalog","job":"third"},
	   "annotations":{},"startsAt":"2026-09-03T10:00:00Z","status":{"state":"active"}},
	  {"labels":{"alertname":"C","job":"node-exporter"},
	   "annotations":{},"startsAt":"2026-09-03T10:00:00Z","status":{"state":"active"}}
	]`, http.StatusOK)

	got, err := c.Firing(context.Background())
	if err != nil {
		t.Fatalf("Firing: %v", err)
	}
	byName := map[string]string{}
	for _, a := range got {
		byName[a.Name] = a.Service
	}
	if byName["A"] != "checkout" || byName["B"] != "catalog" || byName["C"] != "node-exporter" {
		t.Errorf("service resolution wrong: %v", byName)
	}
}

func TestFiringDropsSuppressedAlerts(t *testing.T) {
	// Someone suppressed these knowingly, so the Issues screen leaves them out.
	c, _ := newClient(t, `[
	  {"labels":{"alertname":"Loud"},"annotations":{},
	   "startsAt":"2026-09-03T10:00:00Z","status":{"state":"active"}},
	  {"labels":{"alertname":"Silenced"},"annotations":{},
	   "startsAt":"2026-09-03T10:00:00Z","status":{"state":"suppressed"}}
	]`, http.StatusOK)

	got, err := c.Firing(context.Background())
	if err != nil {
		t.Fatalf("Firing: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Loud" {
		t.Errorf("suppressed alert leaked through: %+v", got)
	}
}

func TestFiringSortsNewestFirst(t *testing.T) {
	c, _ := newClient(t, `[
	  {"labels":{"alertname":"Older"},"annotations":{},
	   "startsAt":"2026-09-03T09:00:00Z","status":{"state":"active"}},
	  {"labels":{"alertname":"Newer"},"annotations":{},
	   "startsAt":"2026-09-03T11:00:00Z","status":{"state":"active"}}
	]`, http.StatusOK)

	got, _ := c.Firing(context.Background())
	if len(got) != 2 || got[0].Name != "Newer" {
		t.Errorf("not newest first: %+v", got)
	}
}

func TestFiringAsksOnlyForActiveAlerts(t *testing.T) {
	c, paths := newClient(t, `[]`, http.StatusOK)
	if _, err := c.Firing(context.Background()); err != nil {
		t.Fatalf("Firing: %v", err)
	}
	p := (*paths)[0]
	for _, want := range []string{"active=true", "silenced=false", "inhibited=false"} {
		if !strings.Contains(p, want) {
			t.Errorf("request %q missing %q", p, want)
		}
	}
}

func TestFiringReportsHTTPFailure(t *testing.T) {
	c, _ := newClient(t, "", http.StatusBadGateway)
	if _, err := c.Firing(context.Background()); err == nil {
		t.Fatal("a bad status has to surface as an error")
	}
}

func TestWorstPicksTheHighestSeverity(t *testing.T) {
	cases := []struct {
		name string
		in   []Alert
		want string
	}{
		{"nothing firing", nil, ""},
		{"only warnings", []Alert{{Severity: "warning"}}, "warning"},
		{"critical beats warning whatever the order",
			[]Alert{{Severity: "warning"}, {Severity: "critical"}}, "critical"},
		{"an unknown severity counts for nothing",
			[]Alert{{Severity: "page"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Worst(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestByServiceGroups(t *testing.T) {
	got := ByService([]Alert{
		{Name: "A", Service: "checkout"},
		{Name: "B", Service: "checkout"},
		{Name: "C", Service: ""},
	})
	if len(got["checkout"]) != 2 {
		t.Errorf("checkout should have 2 alerts, got %d", len(got["checkout"]))
	}
	// An alert naming no service still has to reach the screen.
	if len(got[""]) != 1 {
		t.Errorf("cluster-wide alert lost: %v", got)
	}
}
