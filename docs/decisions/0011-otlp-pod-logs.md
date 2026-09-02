# 0011. Ship pod logs over OTLP, accepting a public-preview Alloy component

**Status:** Accepted
**Date:** 2026-09-02

## Context

k8s-monitoring offers several ways to collect pod logs. Two matter here:

- `podLogsViaLoki` — Loki-native push. All components generally available.
- `podLogsViaOpenTelemetry` — OTLP. Uses `otelcol.receiver.filelog`, which Alloy
  classifies as **public-preview**, so the collector must declare
  `stabilityLevel: public-preview` or the chart refuses to render.

Loki 3.x automatically promotes OTLP resource attributes to index labels,
turning `service.name` into the `service_name` label and putting `trace_id` into
structured metadata.

## Decision

Use `podLogsViaOpenTelemetry` and set `stabilityLevel: public-preview` on the
`logs` collector.

The OTLP path is what makes design doc §4.3 rules 2 and 3 hold with no work:
logs carry the same identity traces do, with no mapping table, and "logs for
this trace" is a structured-metadata filter rather than a regex over the log
line. Taking the GA path would mean hand-maintaining a label mapping — exactly
the fragile glue the join key is meant to avoid.

## Consequences

- We depend on a public-preview Alloy component. The underlying OpenTelemetry
  filelog receiver is long-standing and widely deployed; it is Alloy's wrapper
  that carries the preview label. Judged an acceptable risk for the payoff.
- Loki promotes roughly 17 attributes by default and caps index labels at 15.
  Trim the list toward the ~6 we actually query. Grafana already advises against
  `k8s.pod.name` and `service.instance.id` as default labels on cardinality
  grounds.
- If this component regresses, the fallback is `podLogsViaLoki` plus an explicit
  label mapping — more configuration, same join key.
