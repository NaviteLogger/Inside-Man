# v2 backlog

Out of scope for v1. This file exists so "could we also..." has an answer other
than "yes, let's start now". Design doc 10 names scope creep as the risk most
likely to stop the project shipping.

## Parked

- Own trace waterfall. We deep-link Traces Drilldown instead, see
  [ADR 0005](decisions/0005-embed-grafana-drilldown-for-traces-and-logs.md)
- Own log explorer, same reasoning
- Anomaly and baseline detection. v1 health is plain thresholds
- Multi-cluster and multi-tenant SaaS
- SLO objects and error budgets
- Profiling (Pyroscope)
- Docker Compose dev profile
- Notification routing UI
- Per-namespace RBAC
- Synthetic monitoring, RUM, browser monitoring
- Go auto-instrumentation, revisit when OBI's Kubernetes story matures, see
  [ADR 0007](decisions/0007-demo-app-languages.md)

## v1 non-goals

- Building our own storage or ingest pipeline. We query Loki, Tempo and
  Prometheus and never store telemetry ourselves
- Replacing Grafana dashboards for infrastructure deep-dives
- Running outside Kubernetes
