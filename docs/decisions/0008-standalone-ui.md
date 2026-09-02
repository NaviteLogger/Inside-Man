# 0008. Standalone React UI over a Grafana app plugin

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc 6 chose React and TypeScript on Vite with a separate BFF. The
alternative that came up during research was shipping the Services list and
Service detail as a Grafana app plugin.

The plugin route would have inherited authentication, RBAC, theming, time-range
sync, datasource plumbing and Drilldown deep links, and would have removed
oauth2-proxy, the second login surface and our charting layer from the project.
It was the smaller-scope option by a wide margin.

## Decision

Standalone React and Vite, per the design doc, with oauth2-proxy providing one
login across the UI and Grafana.

Chosen knowingly by the project owner. Inside Man exists because engineers avoid
half-finished Grafana, so the product identity and control over the
service-centric UX are the point.

## Consequences

- This is the largest self-inflicted scope in the project. It stays contained:
  we build charts and the service map, and deep-link to Drilldown for traces and
  logs, see [0005](0005-embed-grafana-drilldown-for-traces-and-logs.md).
- We own authentication as a deployment concern. oauth2-proxy handles it, with a
  documented `auth.enabled=false` dev mode.
- We own theming, time-range sync and URL state.
- Grafana's Kubernetes Monitoring app and Application Observability are the
  closest equivalents and both are Cloud-only, so no OSS component covers this
  ground. That gap is why the project exists.
