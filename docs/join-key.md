# The join key

**This is the most important convention in Inside Man.** Every screen pivots on it:
service → pods → traces → logs → alerts. If it breaks, the UI shows nothing or,
worse, mismatched data. It is normative — treat a violation as a bug.

## The convention

`service.name` is the primary key for everything.

| Signal | Where identity lives | Set by |
|---|---|---|
| Traces | `service.name` resource attribute | OTel SDK / auto-injected agent |
| Metrics | `service` label on `traces_spanmetrics_*` | Alloy `spanMetrics` connector |
| Logs | `service_name` index label | Loki's OTLP attribute promotion |

Grouping and topology come from `service.namespace` plus the Kubernetes
attributes `k8s.namespace.name`, `k8s.deployment.name` and `k8s.pod.name`,
attached by Alloy's `k8sattributes` processor.

## Why it holds without a mapping table

Two upstream behaviours do the work for us, which is why we configure it this
way rather than writing our own relabelling:

1. **The OpenTelemetry Operator always produces a name.** Its resolution order
   (verified against operator appVersion 0.158.0) is:

   1. `OTEL_SERVICE_NAME` already set on the container
      (beats `service.name` inside `OTEL_RESOURCE_ATTRIBUTES`)
   2. the `resource.opentelemetry.io/service.name` pod annotation
   3. Kubernetes labels — **only if** `defaults.useLabelsForResourceAttributes`
      is true, which **we deliberately leave off**
   4. the workload name: Deployment → ReplicaSet / StatefulSet / DaemonSet /
      CronJob / Job → Pod → container
   5. the `Instrumentation` CR's `resource.resourceAttributes`

   Because step 3 is off, identity is always the workload name unless an app
   opts out explicitly. Turning that flag on later would silently rename every
   service in the cluster, so it is a one-way door — see
   `decisions/0004-service-name-from-workload.md`.

2. **Loki promotes OTLP resource attributes to index labels.** On OTLP ingest,
   Loki 3.x maps roughly 17 resource attributes to labels, converting dots to
   underscores: `service.name` → `service_name`, `k8s.namespace.name` →
   `k8s_namespace_name`, and so on. Everything else — including `trace_id` —
   becomes structured metadata.

   This is why pod logs are shipped over OTLP (`podLogsViaOpenTelemetry`) rather
   than the Loki-native push API. It means logs carry the same identity traces
   do, for free, and "logs for this trace" is a structured-metadata filter
   instead of a regex over the log line.

## Querying across the join

```promql
# RED metrics for a service
sum by (service) (rate(traces_spanmetrics_calls_total{service="checkout"}[5m]))
```
```logql
# That service's logs
{service_name="checkout"}

# That request's logs — structured metadata, not a regex
{service_name="checkout"} | trace_id="4bf92f3577b34da6a3ce929d0e0e4736"
```
```traceql
# That service's failing traces
{resource.service.name="checkout" && status=error}
```

## Cardinality budget

Span metrics may carry the four intrinsic dimensions — `service`, `span_name`,
`span_kind`, `status_code` — plus at most `k8s_namespace_name`,
`k8s_cluster_name`, `service_namespace` and `deployment_environment_name`.

Never add `k8s.pod.name`, `http.route` containing IDs, `http.target`, `user.id`
or `db.statement`. Each is unbounded and will melt the metrics store. The e2e
suite asserts the active series count stays under budget so a regression fails
CI rather than someone's cluster.

## Verifying it

`make e2e` asserts the round trip end to end. In a running cluster, the
diagnostics endpoint (`/api/diagnostics`, and the UI page that renders it)
cross-checks, for every service seen in span metrics, whether logs exist under
the matching label and whether `k8s.*` attributes are present. **When a screen
is unexpectedly empty, look there first.**

## Known upstream watch item

A regression in the Collector's `k8sattributes` processor
([contrib #47534](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/47534))
stopped `resource.opentelemetry.io/service.name` taking precedence over
`app.kubernetes.io/name` when `otel_annotations` is enabled — broken in
v0.146.0, working in v0.145.0. We therefore ship
`applicationObservability.processors.k8sattributes.otelAnnotations: false`.
Re-check the issue before enabling it.
