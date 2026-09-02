# 0006. Apache-2.0

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc 11.6 left the licence open, noting that the Grafana components we
build on (Grafana, Loki, Mimir, Tempo) are AGPL-3.0.

## Decision

Apache-2.0. The project is named Inside Man.

Our Helm chart installs those components and our BFF queries them over HTTP. We
neither link against nor derive from their source, so their AGPL does not reach
our code. Apache-2.0 is also the norm for cloud-native tooling and the least
friction for companies that bar AGPL dependencies.

## Consequences

- Anyone may run a hosted Inside Man without contributing back, which is the
  trade we want for adoption.
- Operators still receive AGPL software in the bundle and carry its obligations
  for those components. The README says so.
- This is engineering judgement, and no substitute for legal advice. Anyone
  shipping this commercially should have counsel confirm it.
