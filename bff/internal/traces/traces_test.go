package traces

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newClient(t *testing.T, handler http.HandlerFunc) (*Client, *[]url.Values) {
	t.Helper()
	var seen []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query())
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, 5*time.Second), &seen
}

func TestSearchParsesSummaries(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"traces":[
			{"traceID":"4bf92f3577b34da6a3ce929d0e0e4736","rootServiceName":"frontend",
			 "rootTraceName":"GET /checkout","durationMillis":42,"startTimeUnixNano":"1700000000000000000"}
		]}`)
	})

	got, err := c.Search(context.Background(), `{}`, time.Hour, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d summaries, want 1", len(got))
	}
	if got[0].TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" ||
		got[0].RootServiceName != "frontend" || got[0].DurationMillis != 42 {
		t.Errorf("summary parsed wrong: %+v", got[0])
	}
}

func TestErrorTracesBuildsTraceQL(t *testing.T) {
	c, seen := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"traces":[]}`)
	})

	if _, err := c.ErrorTraces(context.Background(), "demo-backend", time.Hour, 10); err != nil {
		t.Fatalf("ErrorTraces: %v", err)
	}
	q := (*seen)[0].Get("q")
	// resource.service.name is Tempo's attribute path, and status=error is what
	// makes this the failing-traces list the detail screen leads with.
	if !strings.Contains(q, `resource.service.name="demo-backend"`) {
		t.Errorf("query does not select the service: %s", q)
	}
	if !strings.Contains(q, "status=error") {
		t.Errorf("query does not filter to errors: %s", q)
	}
}

func TestSlowTracesFiltersByDuration(t *testing.T) {
	c, seen := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"traces":[]}`)
	})
	if _, err := c.SlowTraces(context.Background(), "api", 250*time.Millisecond, time.Hour, 5); err != nil {
		t.Fatalf("SlowTraces: %v", err)
	}
	if q := (*seen)[0].Get("q"); !strings.Contains(q, "duration > 250ms") {
		t.Errorf("query does not filter by duration: %s", q)
	}
}

func TestSearchSendsAWindowAndLimit(t *testing.T) {
	c, seen := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"traces":[]}`)
	})
	if _, err := c.Search(context.Background(), `{}`, time.Hour, 0); err != nil {
		t.Fatalf("Search: %v", err)
	}
	q := (*seen)[0]
	if q.Get("limit") != "20" {
		t.Errorf("a zero limit should fall back to a default, got %q", q.Get("limit"))
	}
	if q.Get("start") == "" || q.Get("end") == "" {
		t.Error("search has to be bounded, or Tempo scans everything")
	}
}

func TestSearchReportsHTTPFailure(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.Search(context.Background(), `{}`, time.Hour, 5); err == nil {
		t.Fatal("a 500 from Tempo has to surface as an error")
	}
}

func TestUpTreatsPlainTextEchoAsHealthy(t *testing.T) {
	// /api/echo answers "echo" in plain text, so a JSON decode failure there is
	// expected and the status code is the real signal.
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "echo")
	})
	if err := c.Up(context.Background()); err != nil {
		t.Fatalf("Up should accept a plain-text echo: %v", err)
	}
}

func TestUpFailsOnBadStatus(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if err := c.Up(context.Background()); err == nil {
		t.Fatal("Up has to report an unhealthy Tempo")
	}
}
