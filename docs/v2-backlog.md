# v2 backlog (parked)

Deliberately out of scope. This file exists so that "could we also…" has an
answer that is not "yes, let's start now" — design doc §10 names scope creep as
the risk most likely to stop the project shipping.

## Parked

- Own trace waterfall — we deep-link Traces Drilldown instead
  ([ADR 0005](decisions/0005-embed-grafana-drilldown-for-traces-and-logs.md))
- Own log explorer — same, Logs Drilldown
- Anomaly and baseline detection (v1 health is deliberately dumb thresholds)
- Multi-cluster and multi-tenant SaaS
- SLO objects and error budgets
- Profiling (Pyroscope)
- Docker Compose dev profile
- Notification routing UI
- Per-namespace RBAC
- Synthetic monitoring, RUM, browser monitoring
- Go auto-instrumentation — revisit when OBI's Kubernetes story matures
  ([ADR 0007](decisions/0007-demo-app-languages.md))

## Explicit v1 non-goals

- Building our own storage or ingest pipeline. We query Loki, Tempo and
  Prometheus; we never store telemetry ourselves.
- Replacing Grafana dashboards for infrastructure deep-dives.
- Running outside Kubernetes.
