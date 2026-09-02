# Inside Man

Service-centric observability for Kubernetes. One `helm install` gives a cluster
traces, logs and metrics with no manual Grafana wiring, behind a UI that answers
"is my service healthy, and if not, why?" in three clicks.

Every project needs the same observability stack, and every project spends days
wiring up datasources, correlations, agents, dashboards and alert rules before
ending up with a half-finished Grafana that engineers avoid. Inside Man is the
opinionated bundle plus the service-centric layer that Grafana only ships in
Grafana Cloud.

> Early days. M0 is done and the chart installs cleanly. See [Milestones](#milestones).

## Quick start

```bash
make cluster   # single-node kind cluster with the pinned toolchain
make up        # install the umbrella chart
make down      # tear it down
```

`make bootstrap` installs pinned kubectl, kind, helm, Go and gitleaks into
`.tools/bin`. Nothing goes system-wide, and re-running is a no-op.

`make help` lists the rest.

## What you get

| Component | Ours? | Role |
|---|---|---|
| [k8s-monitoring](https://github.com/grafana/k8s-monitoring-helm) (Alloy) | upstream, our values | OTLP gateway, `k8sattributes` enrichment, span metrics, service graph, pod logs, cluster metrics and events |
| OpenTelemetry Operator | upstream, our `Instrumentation` CR | agent injection for Java, Node, Python and .NET |
| Prometheus (or Mimir) | upstream | metrics |
| Loki | upstream | logs |
| Tempo | upstream | traces |
| Grafana | upstream, our provisioning | datasources, correlations, dashboards, and the bundled Drilldown apps |
| BFF, UI, umbrella chart | ours | the service-centric layer |

We write as little as possible. Ingest is one upstream chart configured through
values, in place of a hand-rolled Alloy config and a second Collector.

## The join key

Everything pivots on `service.name`. Traces carry it as a resource attribute,
span metrics as a `service` label, and logs as a `service_name` index label. The
last one comes free, because Loki promotes OTLP resource attributes to labels.
`trace_id` arrives as structured metadata, which makes "logs for this trace" a
filter over a regex.

[docs/join-key.md](docs/join-key.md) is the normative version. Read it before
changing anything in the ingest path.

## Profiles

The default is the small profile: Prometheus, monolithic Loki, monolithic Tempo,
filesystem storage. A default install requests **1050m CPU and 2.37 GiB**,
against a target of 4 CPU and 8 GiB.

`values-ha.yaml` switches to Mimir and `tempo-distributed`. Tempo 3.0 needs an
external Kafka broker and object storage in distributed mode.

## Authentication

Off by default, so a fresh install works without an identity provider. Enable
`oauth2-proxy` for anything shared and it fronts both the UI and Grafana with a
single OIDC login.

## Milestones

| | | Status |
|---|---|---|
| M0 | Foundations: repo, CI, kind bootstrap, chart installs | done |
| M1 | The bundle works: trace to logs to RED metrics, zero config | in progress |
| M2 | Service list: `/api/services`, UI home screen, diagnostics page | |
| M3 | Service detail: pods, dependencies, traces, logs | |
| M4 | Service map and issues | |

Non-goals and the parked backlog are in [docs/v2-backlog.md](docs/v2-backlog.md).

## Decisions

Significant choices are recorded in [docs/decisions/](docs/decisions/). Two are
worth reading before touching the chart:

- [0009](docs/decisions/0009-grafana-helm-chart-repo-split.md): `grafana`, `loki`
  and `tempo` come from `grafana-community`. The `loki` chart left in
  `grafana.github.io` is now the Enterprise Logs one.
- [0003](docs/decisions/0003-use-k8s-monitoring-chart.md): why we own no Alloy or
  Collector config.

## Licence

Apache-2.0, see [LICENSE](LICENSE) and [ADR 0006](docs/decisions/0006-apache-2-license.md).

Inside Man installs Grafana, Loki, Mimir and Tempo, which are AGPL-3.0. Our code
queries them over HTTP without linking against them. Operators still receive
that software and carry its obligations for those components.
