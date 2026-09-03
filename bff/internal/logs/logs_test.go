package logs

import (
	"strings"
	"testing"
	"time"
)

func TestIsHexIDRejectsInjection(t *testing.T) {
	// The trace id goes into a LogQL line filter, so anything that is not a
	// plain id has to be refused before it gets there.
	bad := []string{
		"", "not-a-trace", "../etc/passwd",
		// The W3C spec's example trace id, used here as injection bait. It is
		// 32 hex characters, which secret scanners read as an api key.
		`4bf92f3577b34da6a3ce929d0e0e4736" or ""=="`, //gitleaks:allow
		"4BF92F3577B34DA6A3CE929D0E0E4736",           // upper case is not what Loki holds
		"4bf92f3577b34da6a3ce929d0e0e47",             // 30 chars
	}
	for _, s := range bad {
		if isHexID(s) {
			t.Errorf("accepted %q", s)
		}
	}

	good := []string{
		"4bf92f3577b34da6a3ce929d0e0e4736",
		"00f067aa0ba902b7",
	}
	for _, s := range good {
		if !isHexID(s) {
			t.Errorf("rejected valid id %q", s)
		}
	}
}

func TestForTraceExcludesOurOwnNamespace(t *testing.T) {
	// nginx logs the trace id in the request URL and Loki logs it in the query
	// it just ran, so without this the answer includes logs about the question.
	c := New("http://loki", time.Second, "inside-man")
	sel := c.selector()
	if !strings.Contains(sel, `k8s_namespace_name!="inside-man"`) {
		t.Fatalf("selector does not exclude our namespace: %s", sel)
	}

	// With nothing to exclude the selector stays simple.
	if got := New("http://loki", time.Second, "").selector(); got != `{service_name=~".+"}` {
		t.Fatalf("unexpected selector with no exclusion: %s", got)
	}
}
