# 0005. Deep-link to Grafana Drilldown for traces and logs; never build a waterfall

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §11.4 asked whether to embed Grafana's trace view in v1 or build a
minimal waterfall immediately. §10 separately named "scope creep into own trace
waterfall" as a risk that would stop the project shipping.

Grafana's Drilldown apps — Metrics, Logs, Traces, Profiles — are Apache-2.0 and
bundled by default in Grafana 12 and later, so they are already present in the
13.2.0 we ship. Nothing needs installing.

## Decision

The trace view deep-links to Traces Drilldown; log exploration deep-links to
Logs Drilldown. We build neither a waterfall nor a log explorer, in v1 or later.

## Consequences

- Two of the largest UI surfaces cost us nothing.
- Users leave our UI for those workflows. That is consistent with design doc
  §2's goal 5 — our UI covers the 80% workflow and Grafana is one click away.
- We depend on Drilldown's URL parameter format, which is not a stability
  contract. The e2e suite should assert the generated links resolve, so a
  Grafana bump that changes them fails CI instead of dead-ending a user.
- Requires Grafana ≥ 12, which is a hard floor on the chart version and part of
  why [0009](0009-grafana-helm-chart-repo-split.md) matters.
