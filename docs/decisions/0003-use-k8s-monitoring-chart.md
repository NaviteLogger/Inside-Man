# 0003. Depend on grafana/k8s-monitoring instead of writing our own Alloy and Collector config

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §4.2 listed an OTel Collector Deployment and an Alloy DaemonSet as
components whose configuration we would own, with a `/collector-config`
directory in the repo layout (§8). Doc §10 separately flagged the risk of
overlapping with Grafana's own k8s-monitoring chart.

## Decision

Depend on `k8s-monitoring` 4.5.0 and delete `/collector-config`.

Its `applicationObservability` feature *is* an Alloy collector running the otelcol
receivers, the `k8sattributes` processor, batch and memory-limiter, and OTLP
export. Running a second Collector for the ingest path buys nothing. The same
chart also covers pod logs, cAdvisor and kubelet metrics, kube-state-metrics,
node-exporter and cluster events.

We keep the OpenTelemetry Operator as a separate dependency: k8s-monitoring's
`autoInstrumentation` means Grafana Beyla (eBPF), not the Operator's
`Instrumentation` CR, which is what we want for zero-touch language agents.

## Consequences

- Two components and a whole directory disappear from what we maintain. This is
  the single largest scope reduction in the project.
- We inherit the chart's v4 collector model: features are assigned to named
  Alloy instances. We run three — `singleton` (cluster-wide scraping),
  `logs` (DaemonSet), `receiver` (OTLP gateway).
- We are exposed to k8s-monitoring's release cadence, and v3→v4 was a full
  config rewrite. Pinned version plus kind e2e in CI is the guard.
- `selfReporting` must stay off; it defaults on and reports toward Grafana Cloud.
- kube-state-metrics and node-exporter are deployed here, so the Prometheus
  chart's own copies are disabled to avoid duplicate workloads and metrics.
