# 0007. Demo app in Java, Node and Python — deliberately not Go

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc §11.7 left the demo stacks open, and §7's `instrumentation.languages`
default listed `[java, nodejs, python, dotnet, go]`. The demo app exists to
prove auto-instrumentation actually works with nothing but an annotation.

## Decision

Java, Node and Python, in a Node → Python → Java call chain. Go is dropped from
both the demo app and the default `languages` list.

Go auto-instrumentation via the Operator is not comparable to the others:

- it injects a **privileged eBPF sidecar**, not an init container
- it needs the `enable-go-instrumentation` feature gate, which is off by default
- it needs `shareProcessNamespace: true` and does not support multi-container pods
- `opentelemetry-go-instrumentation` is effectively in maintenance mode, with
  contributors moved to OBI (the donated Beyla)

Requiring a privileged sidecar in every application pod is not something a
platform engineer will accept, and the upstream project is not the strategic
path.

## Consequences

- The three shipped languages all use ordinary init-container injection, so the
  demo genuinely demonstrates "annotation is enough".
- Go users are pointed at OBI or the Go SDK, documented rather than automated.
  Revisit when OBI's Kubernetes story matures.
- .NET stays in the default `languages` list — it works via init container —
  but is not in the demo app. Note it defaults to glibc images; musl needs the
  `instrumentation.opentelemetry.io/dotnet-runtime: linux-musl-x64` annotation.
