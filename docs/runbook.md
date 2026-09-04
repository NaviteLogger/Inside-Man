# Runbook

What to do when Inside Man itself misbehaves. For "my service is broken", the
product is the answer; this is for when the product is the problem.

**Start at the diagnostics page** (`/diagnostics`, or `GET /api/diagnostics`).
It checks the things that make every other screen work, and most of what
follows is a longer explanation of one of its lines.

## The services list is empty

The diagnostics page distinguishes the three causes.

**"no service is producing span metrics"** means nothing is instrumented, or
nothing has had traffic in the query window. Check that an application pod
carries the annotation and that the agent was injected:

```bash
kubectl get pod -n <ns> <pod> -o jsonpath='{.spec.initContainers[*].name}'
```

An `opentelemetry-auto-instrumentation-<language>` init container means the
operator saw the annotation. Nothing there means it did not. Two usual causes: the annotation sits on the
Deployment where it belongs on the pod template, or the `Instrumentation` CR
lives in another namespace and the annotation value has to name it as
`<namespace>/<name>`.

**Services listed with no namespace** means spans arrived without the
Kubernetes attributes, so pods, logs and alerts cannot be joined to them. The
`k8sattributes` processor in the Alloy receiver is what attaches those. Check
the receiver is running and has RBAC to list pods.

**Services that resolve to no Deployment** means `service.name` and the
Deployment name disagree. Identity comes from the workload name unless an app
overrides it, see [decisions/0004](decisions/0004-service-name-from-workload.md).
An app setting `OTEL_SERVICE_NAME` to something else is the usual cause, and it
is legitimate; the join simply cannot find its pods.

## A service shows Unknown health

Unknown means no spans in the window, which is different from spans that all
succeeded. Either the service is idle, or it stopped reporting. Compare against
its pods: a Running pod with no spans points at the agent, an absent pod points
at the workload.

## Logs are missing for a service that has traces

The join key is `service_name`, and logs get it from Loki promoting the OTLP
`service.name` resource attribute. Check what Loki actually indexed:

```bash
curl -s "$LOKI/loki/api/v1/label/service_name/values"
```

If the service is absent there but present in metrics, the log pipeline is not
receiving the resource attribute. That usually means pod logs are being
collected through a path other than `podLogsViaOpenTelemetry`, which is the one
that carries OTLP attributes. See [join-key.md](join-key.md).

## "Logs for this trace" returns nothing

The trace id lives in the log body, since Alloy collects container stdout and
Loki never sees an OTLP TraceId field to promote. So the application has to
print it. Services that log without trace context will show traces and logs
separately but never together, which is expected.

The demo app shows three ways to emit it: the Node service reads the active
span from the agent's API, the Python service lets the agent inject it into
stdlib logging records, and the Java service parses the incoming `traceparent`
header.

## An upgrade fails on the webhook

```
failed calling webhook "minstrumentation.kb.io": tls: failed to verify
certificate: x509: certificate signed by unknown authority
```

The OpenTelemetry Operator mints a new webhook certificate on upgrade while the
running pod keeps serving the old one. The chart sets
`autoGenerateCert.recreate: false` to prevent it. If it happens anyway, restart
the operator once and retry:

```bash
kubectl rollout restart deploy/<release>-opentelemetry-operator -n <ns>
```

## helm upgrade fails on the Loki gateway

```
resource Deployment/inside-man-loki-gateway not ready.
status: Failed, message: Progress deadline exceeded
```

The gateway carries required pod anti-affinity on the hostname, and a
Deployment's default rolling update creates the replacement before removing the
old pod. On a single node the replacement can never schedule, so the rollout
stalls until the deadline. The small profile sets `maxSurge: 0` so the old pod
goes first. The `singleBinary` StatefulSet is unaffected, because a StatefulSet
deletes before it creates.

`make upgrade-test` covers this, and CI runs it on every change.

## A chart bump appears to do nothing

Helm renders from the vendored tarballs in `charts/` and ignores the version in
`Chart.yaml`, so a bump without the matching tarball installs the old chart
while claiming the new one. `make lint` catches it. The fix is `make deps`,
which updates the tarballs and then checks. See
[decisions/0009](decisions/0009-grafana-helm-chart-repo-split.md).

## Prometheus rejects Alloy's writes

```
server returned HTTP status 404 Not Found: remote write receiver needs to be
enabled with --web.enable-remote-write-receiver
```

The flag lives in `prometheus.server.extraFlags`. It cannot go in `extraArgs`,
because a null value in a values file is a delete instruction to Helm's
coalescing and the key never reaches the template.

## Span metric cardinality is growing

The budget is the four intrinsic dimensions plus at most four Kubernetes ones,
and the e2e suite asserts the series count stays under it. If it grows, a
high-cardinality attribute has been added to
`applicationObservability.connectors.spanMetrics.dimensions`. Anything
unbounded per request, such as a route containing an id, will do it. See the
cardinality budget in [join-key.md](join-key.md).

## Retention

One knob, `insideMan.retention`, holds traces, logs and metrics. Each store
reads its own copy from subchart values, because Helm does not template a
values file, and `templates/retention.yaml` fails the render if the two
disagree and names the value to change.

## Footprint

The default profile measures **1050m CPU and 2.37 GiB of requests**, against
the design doc's target of 4 CPU and 8 GiB, and the e2e suite asserts it.
`values-ha.yaml` trades that for Mimir and distributed Tempo, which need an
external Kafka broker and object storage.

## Getting the numbers yourself

```bash
make cluster   # single-node kind cluster
make up        # install
make demo      # a three-service app that generates real telemetry
make e2e       # 51 assertions across the whole path
make upgrade-test   # install, upgrade, roll back, verify data survived
```
