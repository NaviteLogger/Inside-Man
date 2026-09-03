package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/NaviteLogger/Inside-Man/bff/internal/health"
	"github.com/NaviteLogger/Inside-Man/bff/internal/kube"
	"github.com/NaviteLogger/Inside-Man/bff/internal/logs"
	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
	"github.com/NaviteLogger/Inside-Man/bff/internal/traces"
)

// The UI's types are generated from openapi.yaml, so a Go struct that drifts
// from the spec silently breaks the UI's contract. This walks the actual JSON
// each response type emits and compares it against the spec's properties.
func TestResponseShapesMatchOpenAPISpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "openapi.yaml")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("reading %s: %v", specPath, err)
	}

	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing spec: %v", err)
	}

	// Fixtures carry a value for every optional field too. Go omits empty
	// fields, and a name can only be checked against the spec if it is
	// actually emitted.
	cases := []struct {
		schema string
		value  any
	}{
		{"ServicesResponse", servicesResponse{
			Services: []Service{{}},
			Window:   "5m",
		}},
		{"Service", Service{
			Health:    health.Result{Status: health.Healthy},
			Sparkline: []float64{1},
			Workload:  &kube.Workload{},
		}},
		{"Health", health.Result{Status: health.Healthy, Reasons: []string{"x"}}},
		{"Workload", kube.Workload{Pods: []kube.Pod{{}}}},
		{"Pod", kube.Pod{Node: "node-1"}},
		{"Edge", promql.Edge{}},
		{"PodUsage", promql.PodUsage{}},
		{"TraceSummary", traces.Summary{}},
		{"LogLine", logs.Line{Timestamp: time.Now(), Pod: "pod-1"}},
		{"Check", Check{Hint: "do the thing"}},
		{"DiagnosticsResponse", diagnosticsResponse{Checks: []Check{{}}, CheckedAt: "now"}},
		{"ServiceDetail", ServiceDetail{
			Health:      health.Result{Status: health.Healthy},
			Workload:    &kube.Workload{},
			Resources:   map[string]*promql.PodUsage{"p": {}},
			Inbound:     []promql.Edge{},
			Outbound:    []promql.Edge{},
			ErrorTraces: []traces.Summary{},
			Links:       map[string]string{"logs": "x"},
			Window:      "5m",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.schema, func(t *testing.T) {
			declared, ok := spec.Components.Schemas[tc.schema]
			if !ok {
				t.Fatalf("openapi.yaml has no schema named %s", tc.schema)
			}

			blob, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshalling: %v", err)
			}
			var emitted map[string]json.RawMessage
			if err := json.Unmarshal(blob, &emitted); err != nil {
				t.Fatalf("unmarshalling: %v", err)
			}

			for field := range emitted {
				if _, found := declared.Properties[field]; !found {
					t.Errorf("Go emits %q, which openapi.yaml does not declare on %s",
						field, tc.schema)
				}
			}

			// A property in the spec that Go never emits would leave the UI
			// with a type for a field that never arrives.
			var missing []string
			for field := range declared.Properties {
				if _, found := emitted[field]; !found {
					missing = append(missing, field)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("openapi.yaml declares %v on %s, which Go never emits",
					missing, tc.schema)
			}
		})
	}
}
