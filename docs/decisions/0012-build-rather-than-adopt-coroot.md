# 0012. Build Inside Man rather than adopt Coroot

**Status:** Accepted
**Date:** 2026-09-02

## Context

The project's constraint is to avoid reinventing the wheel, so the closest OSS
prior art had to be examined before writing a UI.

- **Grafana Kubernetes Monitoring app** and **Application Observability** are
  the closest match to the product vision. Both are **Cloud-only and closed** —
  Grafana's docs state Kubernetes Monitoring "is available only in Grafana
  Cloud, and only supports data sources hosted in Grafana Cloud". Grafana has
  deliberately kept this layer proprietary.
- **Coroot** (Apache-2.0, v1.25.0) genuinely ships a service-centric Kubernetes
  UI with an eBPF service map, SLOs and a Helm install.
- **SigNoz**, **Uptrace**, **OpenObserve** each replace the LGTM stack with
  their own store.
- **Odigos** and **Pixie** are instrumentation layers, complementary rather than
  competing.

## Decision

Build, and record the comparison rather than run a spike.

Coroot's architecture is "bring our storage engine and our eBPF agents". Inside
Man's premise is the opposite: query the LGTM stack a team already runs, and add
the service-centric layer Grafana keeps closed. A team that has already invested
in Loki, Tempo and Prometheus is the target user, and for them Coroot is a
parallel stack rather than a layer.

## Consequences

- We accept building and maintaining a BFF and UI that partially overlap
  Coroot's feature set.
- The differentiator must stay real. If Inside Man ever grows its own storage or
  its own agents, this decision is void and Coroot should be reconsidered.
- Grafana could open-source Application Observability at any point, which would
  substantially undercut the project. Worth watching; it would be a good problem
  to have.
- Research deliberately not converted into a running spike. If adoption stalls,
  installing Coroot side by side on the kind cluster is a cheap next step.
