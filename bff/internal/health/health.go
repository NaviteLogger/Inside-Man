// Package health implements the v1 health model from design doc 5.2.
//
// It is intentionally plain thresholds. Anomaly detection is a v2 item, and a
// model nobody can explain is worse than one that is occasionally blunt.
package health

import (
	"fmt"
	"time"
)

type Status string

const (
	Healthy  Status = "healthy"
	Warning  Status = "warning"
	Critical Status = "critical"
	Unknown  Status = "unknown"
)

// Rank orders statuses worst first, which is how the services list sorts.
func (s Status) Rank() int {
	switch s {
	case Critical:
		return 0
	case Warning:
		return 1
	case Healthy:
		return 2
	default:
		return 3
	}
}

type Thresholds struct {
	ErrorRateWarn float64
	ErrorRateCrit float64
	P95Warn       time.Duration
}

// Input is what the evaluator needs to know about one service.
type Input struct {
	// HasSpans is false when the service produced no spans in the window, which
	// is different from producing spans that all succeeded.
	HasSpans  bool
	ErrorRate float64
	P95       time.Duration

	// SLOP95 overrides Thresholds.P95Warn for this service, via the
	// insideman.io/slo-p95 annotation. Zero means unset.
	SLOP95 time.Duration

	FiringCritical bool
	FiringWarning  bool
}

// Result carries the status plus the reasons behind it, so the UI can explain
// itself without recomputing anything.
type Result struct {
	Status  Status   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
}

func Evaluate(in Input, t Thresholds) Result {
	// An alert beats metrics. Someone declared this condition worth paging on.
	if in.FiringCritical {
		return Result{Critical, []string{"a critical alert is firing"}}
	}

	if !in.HasSpans {
		return Result{Unknown, []string{"no spans in the selected window"}}
	}

	if in.ErrorRate > t.ErrorRateCrit {
		return Result{Critical, []string{formatRate("error rate", in.ErrorRate, t.ErrorRateCrit)}}
	}

	var reasons []string
	if in.FiringWarning {
		reasons = append(reasons, "a warning alert is firing")
	}
	if in.ErrorRate > t.ErrorRateWarn {
		reasons = append(reasons, formatRate("error rate", in.ErrorRate, t.ErrorRateWarn))
	}
	if slo := in.slo(t); slo > 0 && in.P95 > slo {
		reasons = append(reasons, "p95 "+in.P95.String()+" exceeds "+slo.String())
	}
	if len(reasons) > 0 {
		return Result{Warning, reasons}
	}
	return Result{Healthy, nil}
}

func (in Input) slo(t Thresholds) time.Duration {
	if in.SLOP95 > 0 {
		return in.SLOP95
	}
	return t.P95Warn
}

func formatRate(label string, actual, limit float64) string {
	return fmt.Sprintf("%s %.2f%% exceeds %.2f%%", label, actual*100, limit*100)
}
