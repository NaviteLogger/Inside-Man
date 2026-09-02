# 0007. Demo app in Java, Node and Python, without Go

**Status:** Accepted
**Date:** 2026-09-02

## Context

Design doc 11.7 left the demo stacks open, and design doc 7's
`instrumentation.languages` default listed `[java, nodejs, python, dotnet, go]`.
The demo app exists to prove auto-instrumentation works with nothing but an
annotation.

## Decision

Java, Node and Python, in a Node to Python to Java call chain. Go is dropped
from the demo app and from the default `languages` list.

Go auto-instrumentation via the Operator is not comparable to the others:

- it injects a privileged eBPF sidecar, where the others use an init container
- it needs the `enable-go-instrumentation` feature gate, off by default
- it needs `shareProcessNamespace: true` and cannot handle multi-container pods
- `opentelemetry-go-instrumentation` is in maintenance mode, with contributors
  moved to OBI

A privileged sidecar in every application pod is not something a platform
engineer will accept.

## Consequences

- The three shipped languages all use init-container injection, so the demo
  actually demonstrates that an annotation is enough.
- Go users are pointed at OBI or the Go SDK. Revisit when OBI's Kubernetes story
  matures.
- .NET stays in the default `languages` list because it works via init
  container, though it is not in the demo app. It defaults to glibc images, and
  musl needs the `instrumentation.opentelemetry.io/dotnet-runtime:
  linux-musl-x64` annotation.
