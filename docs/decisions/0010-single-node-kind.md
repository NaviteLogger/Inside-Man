# 0010. Single-node kind cluster for local development and e2e

**Status:** Accepted
**Date:** 2026-09-02

## Context

The devcontainer runs Docker-in-Docker with `fs.inotify.max_user_instances` set
to 128 and `/proc/sys` mounted read-only. kind recommends far higher limits for
multi-node clusters, and exhaustion shows up as pods stuck in
`ContainerCreating` with no obvious cause. We cannot raise the limit from inside
the container.

## Decision

`scripts/kind-cluster.yaml` defines one control-plane node and nothing else.

## Consequences

- Cannot exercise anything genuinely multi-node: scheduling spread, DaemonSet
  behaviour across nodes, node-failure scenarios. Acceptable — the product
  targets one install per project cluster, and the DaemonSet path is still
  exercised with one instance.
- Resource requests must fit one node. The measured 1050m CPU / 2.37 GiB does,
  with room to spare.
- If inotify exhaustion appears anyway, the fix is raising the sysctl on the
  Docker-in-Docker host via the devcontainer feature, which needs a container
  rebuild.
