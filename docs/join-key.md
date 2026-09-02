# The join key

This is the convention every screen depends on: service to pods to traces to
logs to alerts. If it breaks, the UI shows nothing, or worse, shows mismatched
data. Treat a violation as a bug.

## The convention

`service.name` is the primary key.

| Signal | Where identity lives | Set by |
|---|---|---|
| Traces | `service.name` resource attribute | OTel SDK or auto-injected agent |
| Metrics | `service_name` label on `traces_spanmetrics_*` | Alloy `spanMetrics` connector |
| Logs | `service_name` index label | Loki's OTLP attribute promotion |

Metrics and logs use the identical label name, `service_name`, so a pivot is a
straight substitution with nothing to translate.

Design doc section 4.4 wrote these queries with a `service` label, which is
Tempo's metrics-generator convention. Generating span metrics in Alloy instead
([ADR 0011](decisions/0011-otlp-pod-logs.md) covers the related log path) gives
`service_name`, because the connector derives labels from OTLP resource
attributes with dots turned into underscores.

Grouping and topology come from `service.namespace` plus `k8s.namespace.name`,
`k8s.deployment.name` and `k8s.pod.name`, which Alloy's `k8sattributes`
processor attaches.

## Why it holds without a mapping table

Two upstream behaviours do the work, which is why we configure it this way and
leave relabelling alone.

**The OpenTelemetry Operator always produces a name.** Its resolution order, as
of operator appVersion 0.158.0:

1. `OTEL_SERVICE_NAME` already set on the container, which beats `service.name`
   inside `OTEL_RESOURCE_ATTRIBUTES`
2. the `resource.opentelemetry.io/service.name` pod annotation
3. Kubernetes labels, but only if `defaults.useLabelsForResourceAttributes` is
   true, which we leave off
4. the workload name: Deployment, then ReplicaSet, StatefulSet, DaemonSet,
   CronJob or Job, then Pod, then container
5. the `Instrumentation` CR's `resource.resourceAttributes`

With step 3 off, identity is the workload name unless an app opts out
explicitly. Turning that flag on later renames every service that has an
`app.kubernetes.io/name` differing from its Deployment name, which breaks
dashboards, alert rules and saved links. See
[ADR 0004](decisions/0004-service-name-from-workload.md).

**Loki promotes OTLP resource attributes to index labels.** On OTLP ingest, Loki
3.x maps around 17 resource attributes to labels, converting dots to
underscores: `service.name` becomes `service_name`, `k8s.namespace.name` becomes
`k8s_namespace_name`. Everything else, including `trace_id`, becomes structured
metadata.

That is why pod logs ship over OTLP (`podLogsViaOpenTelemetry`) in place of
Loki's native push API. Logs carry the same identity traces do, with nothing to
maintain.

One caveat, verified on a live cluster. Loki puts an OTLP log record's TraceId
field into structured metadata, but Alloy collects container stdout, so the
record's body is the raw log line and that field is never populated. A
`trace_id` printed by an application therefore lives in the body text, and
"logs for this trace" is a line filter or a query-time JSON parse. Both work and
both are shown below.

## Querying across the join

All of these are run against the demo app in `make e2e`.

```promql
# Request rate
sum by (service_name) (rate(traces_spanmetrics_calls_total[5m]))

# Error ratio
  sum by (service_name) (rate(traces_spanmetrics_calls_total{status_code="STATUS_CODE_ERROR"}[5m]))
/ sum by (service_name) (rate(traces_spanmetrics_calls_total[5m]))

# p95 latency. Note duration_seconds, where Tempo's generator emits latency.
histogram_quantile(0.95, sum by (service_name, le) (rate(traces_spanmetrics_duration_seconds_bucket[5m])))

# Dependencies
sum by (client, server) (rate(traces_service_graph_request_total[5m]))
```
```logql
# That service's logs
{service_name="checkout"}

# That request's logs, everywhere it went
{k8s_namespace_name="demo"} |= "4bf92f3577b34da6a3ce929d0e0e4736"

# Same thing, parsed, when the log line is JSON
{service_name="checkout"} | json | trace_id="4bf92f3577b34da6a3ce929d0e0e4736"
```
```traceql
# That service's failing traces
{resource.service.name="checkout" && status=error}
```

## Cardinality budget

Span metrics may carry the four intrinsic dimensions (`service`, `span_name`,
`span_kind`, `status_code`) plus at most `k8s_namespace_name`,
`k8s_cluster_name`, `service_namespace` and `deployment_environment_name`.

Never add `k8s.pod.name`, `http.route` containing IDs, `http.target`, `user.id`
or `db.statement`. Each is unbounded and will melt the metrics store. The e2e
suite asserts the active series count stays under budget, so a regression fails
CI before it reaches someone's cluster.

## Verifying it

`make e2e` asserts the round trip. In a running cluster, `/api/diagnostics` and
the UI page that renders it check, for every service seen in span metrics,
whether logs exist under the matching label and whether `k8s.*` attributes are
present. Start there when a screen is unexpectedly empty.

## Upstream watch item

A regression in the Collector's `k8sattributes` processor
([contrib #47534](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/47534))
stopped `resource.opentelemetry.io/service.name` taking precedence over
`app.kubernetes.io/name` when `otel_annotations` is enabled. It was broken in
v0.146.0 and working in v0.145.0. We ship
`applicationObservability.processors.k8sattributes.otelAnnotations: false` until
that is confirmed fixed.
