# The join key

This is the convention every screen depends on: service to pods to traces to
logs to alerts. If it breaks, the UI shows nothing, or worse, shows mismatched
data. Treat a violation as a bug.

## The convention

`service.name` is the primary key.

| Signal | Where identity lives | Set by |
|---|---|---|
| Traces | `service.name` resource attribute | OTel SDK or auto-injected agent |
| Metrics | `service` label on `traces_spanmetrics_*` | Alloy `spanMetrics` connector |
| Logs | `service_name` index label | Loki's OTLP attribute promotion |

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
Loki's native push API. Logs carry the same identity traces do, and "logs for
this trace" becomes a structured-metadata filter over a regex on the log line.

## Querying across the join

```promql
# RED metrics for a service
sum by (service) (rate(traces_spanmetrics_calls_total{service="checkout"}[5m]))
```
```logql
# That service's logs
{service_name="checkout"}

# That request's logs, via structured metadata
{service_name="checkout"} | trace_id="4bf92f3577b34da6a3ce929d0e0e4736"
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
