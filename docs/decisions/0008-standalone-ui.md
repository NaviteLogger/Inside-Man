# 0008. Standalone React UI rather than a Grafana app plugin

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §6 chose React + TypeScript on Vite with a separate BFF. The
alternative surfaced during research was shipping the Services list and Service
detail as a **Grafana app plugin** instead.

The plugin route would have inherited authentication, RBAC, theming, time-range
sync, datasource plumbing and Drilldown deep links for free, and would have
removed oauth2-proxy, the second login surface and our charting layer from the
project entirely. It was the smaller-scope option by a wide margin.

## Decision

Standalone React + Vite, per the design doc, with oauth2-proxy providing one
login across the UI and Grafana.

Chosen knowingly by the project owner. Inside Man's premise is that engineers
avoid half-finished Grafana; the product identity and the ability to shape the
service-centric UX without Grafana's shell are the point, not incidental.

## Consequences

- **This is the largest self-inflicted scope in the project.** It is contained
  deliberately: we build charts and the service map, and we deep-link to
  Drilldown for traces and logs rather than rebuilding them
  ([0005](0005-embed-grafana-drilldown-for-traces-and-logs.md)).
- We own authentication as a deployment concern. oauth2-proxy handles it, with
  a documented `auth.enabled=false` dev mode.
- We own theming, time-range synchronisation and URL state.
- Grafana's Kubernetes Monitoring app and Application Observability — the
  closest equivalents — are Cloud-only and closed, so there is no OSS component
  we are duplicating here. That gap is the project's reason to exist.
