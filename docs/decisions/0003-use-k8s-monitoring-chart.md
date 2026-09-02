# 0003. Depend on grafana/k8s-monitoring for the ingest config

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc 4.2 listed an OTel Collector Deployment and an Alloy DaemonSet as
components whose configuration we would own, with a `/collector-config`
directory in the repo layout. Design doc 10 separately flagged the risk of
overlapping with Grafana's own k8s-monitoring chart.

## Decision

Depend on `k8s-monitoring` 4.5.0 and drop `/collector-config`.

Its `applicationObservability` feature is an Alloy collector running the otelcol
receivers, the `k8sattributes` processor, batch and memory-limiter, and OTLP
export. A second Collector for the ingest path adds nothing. The same chart also
covers pod logs, cAdvisor and kubelet metrics, kube-state-metrics, node-exporter
and cluster events.

The OpenTelemetry Operator stays as a separate dependency. k8s-monitoring's
`autoInstrumentation` means Grafana Beyla, whereas the Operator's
`Instrumentation` CR is what gives us zero-touch language agents.

## Consequences

- Two components and a directory leave the list of things we maintain.
- We inherit the v4 collector model, where features are assigned to named Alloy
  instances. We run three: `singleton` for cluster-wide scraping, `logs` as a
  DaemonSet, and `receiver` as the OTLP gateway.
- We follow k8s-monitoring's release cadence, and v3 to v4 was a full config
  rewrite. The pinned version plus kind e2e in CI is the guard.
- `selfReporting` has to stay off. It defaults on and reports to Grafana Cloud.
- kube-state-metrics and node-exporter are deployed here, so the Prometheus
  chart's copies are disabled, which avoids duplicate workloads and metrics.
