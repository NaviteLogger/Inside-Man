# 0002. Prometheus by default, Mimir for HA

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc 6 chose Mimir as the default metrics store with single-node
Prometheus as a small profile. Design doc 12 also requires the small profile to
fit in 4 CPU and 8 GiB and to reach a live services screen within 15 minutes.

Mimir-distributed is many components and expects object storage, so those two
goals pull against each other.

## Decision

Prometheus is the default and the small profile. Mimir is the HA profile.

Prometheus started with `--web.enable-remote-write-receiver` accepts Alloy's
`prometheus` destination type unchanged, so switching profiles is one values key
with no code change.

## Consequences

- The default install measures 1050m CPU and 2.37 GiB of requests.
- Single-node Prometheus is a single point of failure with no long-term
  retention. That suits one project on one cluster, and `values-ha.yaml` exists
  for anyone who outgrows it.
- CI renders both profiles, otherwise the HA path rots.
