# 0011. Ship pod logs over OTLP, accepting a public-preview Alloy component

**Status:** Accepted
**Date:** 2026-09-02

## Context

k8s-monitoring offers several ways to collect pod logs. Two matter here:

- `podLogsViaLoki`, Loki's native push, with all components generally available
- `podLogsViaOpenTelemetry`, OTLP, which uses `otelcol.receiver.filelog`. Alloy
  classes that as public-preview, so the collector has to declare
  `stabilityLevel: public-preview` or the chart refuses to render

Loki 3.x automatically promotes OTLP resource attributes to index labels,
turning `service.name` into the `service_name` label and putting `trace_id` into
structured metadata.

## Decision

Use `podLogsViaOpenTelemetry` and set `stabilityLevel: public-preview` on the
`logs` collector.

The OTLP path is what makes design doc 4.3 rules 2 and 3 hold with no work. Logs
carry the same identity traces do without a mapping table, and "logs for this
trace" becomes a structured-metadata filter over a regex on the log line.
The GA path would mean hand-maintaining a label mapping, which is the fragile
glue the join key exists to avoid.

## Consequences

- We depend on a public-preview Alloy component. The underlying OpenTelemetry
  filelog receiver is long-standing and widely deployed, and it is Alloy's
  wrapper that carries the preview label.
- Loki promotes around 17 attributes by default and caps index labels at 15, so
  the list wants trimming toward the six we query. Grafana advises against
  `k8s.pod.name` and `service.instance.id` as default labels on cardinality
  grounds.
- If the component regresses, the fallback is `podLogsViaLoki` plus an explicit
  label mapping: more configuration, same join key.
