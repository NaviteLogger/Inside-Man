# 0001. BFF in Go

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc 11.1 left the BFF language open: Go for the Kubernetes client and a
single static binary, TypeScript for shared types with the UI.

The services list is the hot path. Every poll needs every service plus its pods,
deployment and rollout state.

## Decision

Go.

`client-go` gives SharedInformers and listers, so the BFF answers from a warm
in-memory cache and avoids N API calls per request. And
`prometheus/client_golang/api/prometheus/v1` is the only official client for the
Prometheus HTTP API in either language. Loki and Tempo are plain HTTP either
way, with no official client for either language.

TypeScript's real advantage, shared types with the UI, is recovered by
generating the UI's types from the BFF's OpenAPI spec.

## Consequences

- The BFF ships as a single static binary, so the chart stays simple.
- We maintain an OpenAPI spec as the contract, and the UI's types are generated
  from it. If that generation is skipped the drift is silent, so CI has to fail
  when the generated types are stale.
- Go joins the pinned toolchain in `scripts/bootstrap.sh`.
