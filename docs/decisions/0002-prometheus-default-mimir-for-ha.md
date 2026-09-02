# 0002. Prometheus by default, Mimir for HA

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §6 chose Mimir as the default metrics store with single-node
Prometheus as a "small" profile. Doc §12 also requires the small profile to fit
in under 4 CPU / 8 GiB and to reach a live services screen in under 15 minutes.

Mimir-distributed is many components and expects object storage. Those two
goals are in tension.

## Decision

Prometheus is the default and the `small` profile. Mimir is the `ha` profile.

Prometheus started with `--web.enable-remote-write-receiver` accepts Alloy's
`prometheus` destination type unchanged, so switching profiles is one values
key with no code change anywhere.

## Consequences

- Measured footprint of the default install is 1050m CPU / 2.37 GiB of requests,
  comfortably inside the budget.
- Single-node Prometheus is a single point of failure and has no long-term
  retention story. That is the correct trade for the target audience — one
  project, one cluster — and `values-ha.yaml` exists for anyone who outgrows it.
- Both profiles must stay covered by CI rendering, or the HA path will rot.
