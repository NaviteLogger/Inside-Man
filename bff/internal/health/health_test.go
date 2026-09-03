package health

import (
	"testing"
	"time"
)

func defaults() Thresholds {
	return Thresholds{ErrorRateWarn: 0.01, ErrorRateCrit: 0.05, P95Warn: 500 * time.Millisecond}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Status
	}{
		{"no spans reports unknown",
			Input{HasSpans: false}, Unknown},
		{"spans with no errors is healthy",
			Input{HasSpans: true, ErrorRate: 0, P95: 10 * time.Millisecond}, Healthy},
		{"error rate above crit is critical",
			Input{HasSpans: true, ErrorRate: 0.2}, Critical},
		{"error rate above warn is warning",
			Input{HasSpans: true, ErrorRate: 0.02}, Warning},
		{"error rate exactly at warn stays healthy",
			Input{HasSpans: true, ErrorRate: 0.01}, Healthy},
		{"error rate exactly at crit is warning, since it passed warn",
			Input{HasSpans: true, ErrorRate: 0.05}, Warning},
		{"p95 beyond the default slo is warning",
			Input{HasSpans: true, P95: time.Second}, Warning},
		{"per-service slo overrides the default",
			Input{HasSpans: true, P95: time.Second, SLOP95: 2 * time.Second}, Healthy},
		{"a tighter per-service slo can trip on a fast service",
			Input{HasSpans: true, P95: 100 * time.Millisecond, SLOP95: 50 * time.Millisecond}, Warning},
		{"a critical alert wins over healthy metrics",
			Input{HasSpans: true, ErrorRate: 0, FiringCritical: true}, Critical},
		{"a critical alert wins even with no spans",
			Input{HasSpans: false, FiringCritical: true}, Critical},
		{"a warning alert alone is warning",
			Input{HasSpans: true, FiringWarning: true}, Warning},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in, defaults())
			if got.Status != tc.want {
				t.Fatalf("got %s, want %s (reasons: %v)", got.Status, tc.want, got.Reasons)
			}
			if tc.want != Healthy && len(got.Reasons) == 0 {
				t.Fatalf("status %s came with no reason to show the user", got.Status)
			}
		})
	}
}

func TestEvaluateCollectsEveryWarningReason(t *testing.T) {
	got := Evaluate(Input{
		HasSpans:      true,
		ErrorRate:     0.02,
		P95:           time.Second,
		FiringWarning: true,
	}, defaults())

	if got.Status != Warning {
		t.Fatalf("got %s, want warning", got.Status)
	}
	if len(got.Reasons) != 3 {
		t.Fatalf("want all 3 reasons surfaced, got %d: %v", len(got.Reasons), got.Reasons)
	}
}

func TestRankSortsWorstFirst(t *testing.T) {
	if !(Critical.Rank() < Warning.Rank() &&
		Warning.Rank() < Healthy.Rank() &&
		Healthy.Rank() < Unknown.Rank()) {
		t.Fatal("ranking must order critical, warning, healthy, unknown")
	}
}
