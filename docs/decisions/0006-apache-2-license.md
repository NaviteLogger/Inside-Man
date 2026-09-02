# 0006. Apache-2.0

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §11.6 left the licence open, noting that the Grafana components we
build on — Grafana, Loki, Mimir, Tempo — are AGPL-3.0.

## Decision

Apache-2.0, and the project is named Inside Man.

Our Helm chart installs those components and our BFF queries them over HTTP. We
neither link against nor derive from their source, so their AGPL does not reach
our code. Apache-2.0 is also the norm for cloud-native tooling and the least
friction for adoption inside companies that bar AGPL dependencies.

## Consequences

- Anyone may run a hosted Inside Man without contributing back. Accepted: the
  goal is adoption.
- Operators still receive AGPL software in the bundle and carry those
  obligations for the components themselves. Worth a line in the README.
- *This is engineering judgement, not legal advice.* Anyone shipping this
  commercially should have counsel confirm it.
