# 0001. BFF in Go

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §11.1 left the BFF language open: Go for the Kubernetes client and a
single static binary, TypeScript for shared types with the UI.

The services list is the hot path — every poll needs every service plus its
pods, deployment and rollout state.

## Decision

Go.

Two things decide it. `client-go` gives SharedInformers and listers, so the BFF
answers from a warm in-memory cache instead of making N API calls per request.
And `prometheus/client_golang/api/prometheus/v1` is the only official client for
the Prometheus HTTP API in either language — Loki and Tempo are plain HTTP
either way, with no official client for either language.

TypeScript's real advantage, shared types with the UI, is recovered by
generating the UI's types from the BFF's OpenAPI spec. That makes it a
non-reason to give up informers.

## Consequences

- The BFF ships as a single static binary; the chart stays simple.
- We maintain an OpenAPI spec as the contract, and the UI's types are generated
  from it rather than hand-written. If that generation is skipped, drift is
  silent — CI must fail when the generated types are stale.
- Go is added to the pinned toolchain in `scripts/bootstrap.sh`.
