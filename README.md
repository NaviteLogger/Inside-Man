# Inside Man

Service-centric observability for Kubernetes. One `helm install` gives a cluster
traces, logs and metrics with no manual Grafana wiring, behind a UI that answers
**"is my service healthy, and if not, why?"** in three clicks.

Every project needs the same observability stack and every project spends days
wiring it — datasources, correlations, agents, dashboards, alert rules — and
ends up with a half-finished Grafana that engineers avoid. Inside Man is the
opinionated bundle plus the service-centric layer that Grafana keeps behind
Grafana Cloud.

> **Status: early.** M0 (foundations) is done and installs cleanly. See
> [Milestones](#milestones).

## Quick start

```bash
make cluster   # single-node kind cluster with the pinned toolchain
make up        # install the umbrella chart
make down      # tear it all down
```

`make bootstrap` installs pinned kubectl, kind, helm, Go and gitleaks into
`.tools/bin`. Nothing is installed system-wide, and re-running is a no-op.

Run `make help` for everything else.

## What you get

| Component | Ours? | Role |
|---|---|---|
| [k8s-monitoring](https://github.com/grafana/k8s-monitoring-helm) (Alloy) | upstream, our values | OTLP gateway, `k8sattributes` enrichment, span metrics, service graph, pod logs, cluster metrics and events |
| OpenTelemetry Operator | upstream, our `Instrumentation` CR | zero-touch agent injection for Java, Node, Python, .NET |
| Prometheus (or Mimir) | upstream | metrics |
| Loki | upstream | logs |
| Tempo | upstream | traces |
| Grafana | upstream, our provisioning | datasources, correlations, dashboards, and the bundled Drilldown apps |
| **BFF, UI, umbrella chart** | **ours** | the service-centric layer |

We write as little as possible. Ingest is one upstream chart configured through
values — not a hand-rolled Alloy config and not a second Collector.

## The join key

Everything pivots on `service.name`. Traces carry it as a resource attribute,
span metrics as a `service` label, and logs as a `service_name` index label —
the last for free, because Loki promotes OTLP resource attributes to labels.
`trace_id` lands as structured metadata, so "logs for this trace" is a filter
rather than a regex.

**[docs/join-key.md](docs/join-key.md) is normative.** Read it before changing
anything in the ingest path.

## Profiles

Default is the small profile: Prometheus, monolithic Loki, monolithic Tempo,
filesystem storage.

Measured footprint of a default install — **1050m CPU / 2.37 GiB of requests**,
against the < 4 CPU / 8 GiB target.

`values-ha.yaml` switches to Mimir and `tempo-distributed`. Note that Tempo 3.0
requires an external Kafka broker and object storage in distributed mode.

## Authentication

Off by default so a fresh install works with no identity provider. Turn on
`oauth2-proxy` for anything shared — it fronts both the UI and Grafana with a
single OIDC login.

## Milestones

| | | Status |
|---|---|---|
| M0 | Foundations — repo, CI, kind bootstrap, chart installs | **done** |
| M1 | The bundle works — trace → logs → RED metrics in Grafana, zero config | in progress |
| M2 | Service list — `/api/services`, UI home screen, diagnostics page | |
| M3 | Service detail — pods, dependencies, traces, logs | |
| M4 | Service map and issues | |

Non-goals for v1 and the parked backlog are in [docs/v2-backlog.md](docs/v2-backlog.md).

## Decisions

Every significant choice is recorded in [docs/decisions/](docs/decisions/).
Two worth reading before touching the chart:

- [0009 — chart repo split](docs/decisions/0009-grafana-helm-chart-repo-split.md):
  `grafana`, `loki` and `tempo` come from `grafana-community`, not
  `grafana.github.io`. The old `loki` chart is silently the Enterprise Logs one.
- [0003 — use k8s-monitoring](docs/decisions/0003-use-k8s-monitoring-chart.md):
  why we own no Alloy or Collector config.

## Licence

Apache-2.0 — see [LICENSE](LICENSE) and
[ADR 0006](docs/decisions/0006-apache-2-license.md).

Inside Man installs Grafana, Loki, Mimir and Tempo, which are AGPL-3.0. Our code
queries them over HTTP and does not link against them, but operators receive
that software and carry its obligations for those components.
