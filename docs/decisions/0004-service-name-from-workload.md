# 0004. Derive service.name from the workload, not from Kubernetes labels

**Status:** Accepted
**Date:** 2026-09-02

## Context

The OpenTelemetry Operator can resolve `service.name` from Kubernetes labels
(`app.kubernetes.io/instance`, then `app.kubernetes.io/name`) when
`defaults.useLabelsForResourceAttributes` is true, or fall through to the
workload name — Deployment, then StatefulSet/DaemonSet/Job, then Pod, then
container.

The two produce different service identities for the same cluster.

## Decision

Leave `useLabelsForResourceAttributes` off. Identity is the workload name unless
an application sets `OTEL_SERVICE_NAME` or the
`resource.opentelemetry.io/service.name` annotation explicitly.

This matches design doc §4.3 rule 1 and needs no convention from application
teams, which is the point of "zero-touch".

## Consequences

- Services are named after their Deployment, which is what a platform engineer
  expects when they have not configured anything.
- **This is a one-way door.** Turning the flag on later renames every service
  that has an `app.kubernetes.io/name` label differing from its Deployment name,
  breaking dashboards, alert rules and saved links. Changing it is a migration,
  not a config tweak.
- Teams wanting a different name have two documented opt-outs that both beat the
  default in the precedence order.
