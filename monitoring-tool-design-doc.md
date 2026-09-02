# Service-Centric Observability Platform — Design Document

**Status:** Draft v0.1 — for review before any code is written
**Working name:** _TBD_ (see Open Decisions)

---

## 1. Problem statement

Every project needs the same observability stack (Grafana, Loki, Prometheus/Mimir, Tempo) and every project spends days wiring it: datasources, correlations, agents, dashboards, alert rules. The result is usually a half-finished Grafana that engineers avoid. Tools like Instana solve this with a zero-config install and a **service-centric** UI, but they are expensive and closed.

**Goal:** an open, Helm-installable platform that gives a Kubernetes cluster Instana-like observability in one install, with a purpose-built UI that answers "is my service healthy, and if not, why?" in three clicks.

## 2. Goals and non-goals

### Goals
1. **One install.** `helm install` → traces, logs, metrics flowing within minutes. No manual datasource setup.
2. **Zero-touch instrumentation.** A pod annotation (or namespace label) is enough to get traces from Java, Node, Python, .NET, Go apps.
3. **Service-centric UI.** Home page is a list of services with health. Every screen pivots: service → pods → traces → logs → alerts.
4. **Backend-swappable.** All ingest is OpenTelemetry; storage is the Grafana LGTM stack today, replaceable later.
5. **Grafana stays available.** The UI covers the 80% workflow; Grafana is one click away for raw PromQL/LogQL/TraceQL.

### Non-goals (v1)
- Building our own storage or ingest pipeline. We query Loki/Tempo/Mimir; we never store data ourselves.
- Multi-cluster or multi-tenant SaaS. One install per cluster (or per project) in v1.
- Synthetic monitoring, RUM / browser monitoring, profiling.
- Replacing Grafana dashboards for infra deep-dives (node-level, etcd, etc.).
- Running outside Kubernetes (Docker Compose may come later as a dev profile).

## 3. Users and context

| Who | Needs |
|---|---|
| Backend developer | "Is my service erroring? Show me a failing trace and the logs around it." |
| Team lead / on-call | "What's broken right now, and since when?" |
| Platform engineer | Installs it once per cluster, tunes retention and resource limits, never touches per-service config. |

Assumed environment: Kubernetes ≥ 1.28, cluster-admin available for install, apps deployed as Deployments/StatefulSets, teams may or may not already emit OTel.

## 4. Architecture

### 4.1 Component overview

```
┌──────────────────────────── Kubernetes cluster ────────────────────────────┐
│                                                                            │
│  App pods ──(auto-injected OTel agents)──► OTel Collector (Deployment)     │
│      │                                          │        │        │        │
│      │ stdout logs                              ▼        ▼        ▼        │
│      ▼                                       Tempo    Mimir     Loki       │
│  Alloy DaemonSet ──(logs, cAdvisor, kube-state)────────►  ▲       ▲        │
│                                                  │        │       │        │
│                                    metrics-generator (RED + service graph) │
│                                                  │        │       │        │
│                                          ┌───────┴────────┴───────┴─────┐  │
│                                          │  BFF / API (our code)        │  │
│                                          │  + Kubernetes API client     │  │
│                                          └──────────────┬───────────────┘  │
│                                                         ▼                  │
│                                             Web UI (our code)   Grafana    │
│                                                         ▲          ▲       │
│                                          oauth2-proxy (single login)       │
└────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Components

| Component | Ours? | Role |
|---|---|---|
| OpenTelemetry Operator | upstream | Auto-injects language agents via `Instrumentation` CR; manages Collector. |
| OTel Collector | upstream (config ours) | Single OTLP endpoint; enriches with k8s attributes (`k8sattributes` processor); fans out to Tempo/Mimir/Loki. |
| Grafana Alloy (DaemonSet) | upstream (config ours) | Pod log tailing, cAdvisor/kubelet metrics, kube-state-metrics scraping, node exporter. |
| Tempo | upstream | Trace storage; **metrics-generator** produces `traces_spanmetrics_*` and `traces_service_graph_*`. |
| Mimir (or Prometheus) | upstream | Metrics storage. Mimir chosen for HA + long retention; Prometheus single-node as a "small" profile. |
| Loki | upstream | Log storage. |
| Grafana | upstream (provisioning ours) | Pre-provisioned datasources, correlations, baseline dashboards, alert rules. |
| Alertmanager (via Mimir ruler) | upstream (rules ours) | Baseline alert rules; UI reads alert state. |
| **BFF / API** | **ours** | Normalizes the three signals + k8s topology around service identity. Exposes a clean JSON API for the UI. |
| **Web UI** | **ours** | Service-centric front end. |
| **Helm umbrella chart** | **ours** | Wires everything; the actual "product" for the platform engineer. |
| oauth2-proxy | upstream | Single OIDC login in front of UI + Grafana. |

### 4.3 Signal flow and the join key

Everything is joined on **service identity**:

- `service.name` (OTel resource attribute) — the primary key.
- `service.namespace` — optional grouping (team / bounded context).
- `k8s.namespace.name`, `k8s.deployment.name` (or statefulset/daemonset), `k8s.pod.name` — attached by the Collector's `k8sattributes` processor to traces and metrics, and by Alloy to log labels.

Rules:
1. If an app does not set `service.name`, the Operator/Collector defaults it to the Deployment name. Services always have a name.
2. Logs must carry the same `service_name` label as traces carry `service.name`, so the UI can pivot without a mapping table.
3. Trace IDs are extracted from logs where present (JSON `trace_id` field or regex) so "logs for this trace" works.

**This join convention is the most important design decision in the project.** Document it, enforce it in the Collector config, and validate it with a `doctor` check.

### 4.4 What the BFF actually queries

| UI need | Source | Query shape |
|---|---|---|
| Service list + RED | Mimir | `sum by (service) (rate(traces_spanmetrics_calls_total[5m]))`, error ratio, `histogram_quantile(0.95, …)` |
| Service dependencies | Mimir | `traces_service_graph_request_total{client=…}` / `{server=…}` |
| Pods / rollout status | Kubernetes API | List pods by label selector derived from the deployment |
| Pod CPU / memory | Mimir | `container_cpu_usage_seconds_total`, `container_memory_working_set_bytes` |
| Recent slow/error traces | Tempo | TraceQL: `{resource.service.name="x" && status=error}` |
| Trace detail | Tempo | `GET /api/traces/{id}` |
| Logs for service / trace | Loki | `{service_name="x"}`, `{service_name="x"} |= "<traceid>"` |
| Active alerts | Alertmanager API | filter by `service` label |

## 5. UI product specification

### 5.1 Screens (priority order)

1. **Services** (home) — table: name, namespace, req/s, error %, p95, health badge, sparkline. Sort by health. Time-range picker global.
2. **Service detail** — RED charts; dependencies (in/out); pods with status & restarts; CPU/mem; recent error traces; live log tail; active alerts. Deep links to Grafana Explore with the equivalent query pre-filled.
3. **Trace detail** — waterfall; span attributes; "logs for this span". _v1: embed Grafana's trace view in an iframe with URL params. Own waterfall is a v2 item._
4. **Service map** — force-directed graph from service-graph edges, node color = health, edge label = req/s and error %.
5. **Issues** — list of firing alerts grouped by service, with "since" and links.

### 5.2 Health model (v1, simple)

| Health | Rule |
|---|---|
| Critical | error rate > 5% over 5m, or any `Critical` alert firing |
| Warning | error rate > 1%, or p95 > configured SLO, or any `Warning` alert |
| Healthy | otherwise |
| Unknown | no spans in the time window |

Thresholds are global defaults, overridable per service via annotation (`obs.io/slo-p95: 300ms`). Keep it dumb in v1; anomaly detection is explicitly out of scope.

### 5.3 UX principles
- Every number is a link to the query that produced it (either in our UI or Grafana).
- Time range is global and in the URL. Every view is shareable.
- Empty states teach: "No traces for this service — add annotation X to the pod spec."

## 6. Technology choices

| Area | Choice | Why | Alternative considered |
|---|---|---|---|
| UI | React + TypeScript (Vite) | Full-stack familiarity; huge ecosystem for charts/graphs. | Next.js (unneeded SSR; BFF is separate anyway) |
| Charts | ECharts or uPlot | Fast with dense time series. | Recharts (too slow at scale) |
| Service map | React Flow (small graphs) → Cytoscape (large) | Start simple. | D3 custom (more work) |
| BFF | Go | Single static binary, first-class k8s client, matches the rest of the ecosystem, trivial to ship in the chart. | Node/TS (shared types with UI — real benefit; weaker k8s client). **Open decision.** |
| Auth | oauth2-proxy + OIDC | Off-the-shelf, covers UI + Grafana. | Built-in auth (no) |
| Packaging | Helm umbrella chart depending on upstream charts | Standard for platform engineers. | Kustomize, Operator (later maybe) |
| Metrics store | Mimir (default), Prometheus ("small" profile) | HA / long retention vs simplicity. | VictoriaMetrics (compatible, could be a profile) |
| CI | GitHub Actions; kind cluster for e2e | Free, standard. | — |

## 7. Helm chart design

```
charts/
  platform/            # umbrella chart
    Chart.yaml         # deps: tempo, loki, mimir|prometheus, grafana, opentelemetry-operator, alloy, oauth2-proxy
    values.yaml
    values-small.yaml  # single-node Prometheus, filesystem storage
    values-ha.yaml     # Mimir, object storage
    templates/
      collector.yaml               # OpenTelemetryCollector CR
      instrumentation.yaml         # Instrumentation CR (agents per language)
      alloy-config.yaml
      grafana-provisioning/        # datasources, correlations, dashboards, alert rules
      bff-deployment.yaml
      ui-deployment.yaml
      rbac.yaml                    # read-only ClusterRole for BFF
```

Key `values.yaml` knobs (keep the surface small):

```yaml
profile: small | ha
storage:
  backend: filesystem | s3 | gcs
  retention: { traces: 7d, logs: 14d, metrics: 30d }
instrumentation:
  languages: [java, nodejs, python, dotnet, go]
  autoInject: { byNamespaceLabel: true, byPodAnnotation: true }
health:
  errorRateWarn: 0.01
  errorRateCrit: 0.05
auth:
  oidc: { issuer: "", clientId: "", clientSecretRef: "" }
resources: { ... per component }
```

## 8. Repository layout

```
/charts/platform          Helm umbrella chart
/bff                      Go (or TS) API server
/ui                       React app
/collector-config         OTel Collector + Alloy configs (rendered into chart)
/grafana                  dashboards, alert rules, correlations as code
/examples/demo-app        multi-service sample app (2–3 languages) for e2e + demos
/docs                     architecture, join-key convention, runbook
/scripts                  kind-based local cluster bootstrap
```

## 9. Milestones

Each milestone has a hard "done" definition. Don't start the next before the current is demoable.

### M0 — Foundations (planning + skeleton)
- Repo, CI, kind bootstrap script, this doc finalized.
- Decision log started (`/docs/decisions/`).
- **Done:** `make cluster` produces an empty kind cluster with the umbrella chart installing successfully (no BFF/UI yet).

### M1 — The bundle works
- Umbrella chart installs LGTM + Operator + Alloy + Grafana with provisioned datasources and correlations.
- Demo app (e.g. Node → Python → Java) deployed with auto-instrumentation.
- Tempo metrics-generator emitting span metrics and service graph.
- **Done:** In Grafana, from a demo-app trace you can jump to logs and to RED metrics without configuring anything by hand.

### M2 — Service list
- BFF with `/api/services` (RED + health) backed by Mimir and k8s.
- UI home screen.
- Single login via oauth2-proxy.
- **Done:** Fresh install + demo app → services screen shows three healthy services; kill a pod → one turns critical within a minute.

### M3 — Service detail + traces + logs
- `/api/services/{name}` (pods, deps, resources, alerts), `/api/traces`, `/api/logs`.
- Service detail screen; trace view embedded from Grafana; log tail.
- **Done:** From home, three clicks to a failing trace and the logs of that request.

### M4 — Service map + issues
- Service graph screen, alerts screen, baseline alert rules shipped in chart.
- **Done:** Map reflects demo-app topology and colors by health; alert fires when a demo service is broken.

### M5 — Hardening / "usable on a real project"
- Resource profiles and retention tuned; upgrade path tested; docs; `doctor` command or UI diagnostics page that verifies the join key is intact.
- **Done:** Installed on one real project's cluster and used for a week without touching Grafana for daily work.

## 10. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Span-metrics cardinality explodes Mimir | Cluster cost, slow queries | Restrict metrics-generator dimensions to service, span name, status; drop high-cardinality attributes in Collector. |
| Join key breaks (apps set odd `service.name`, logs unlabeled) | UI shows nothing / mismatched | Enforce defaults in Collector; diagnostics page; documented convention. |
| Upstream chart churn (Loki/Mimir/Tempo change fast) | Install breaks on upgrade | Pin dependency versions; renovate bot; e2e on kind in CI. |
| Scope creep into "own trace waterfall", "anomaly detection" | Never ships | Non-goals list; milestones with demos; v2 backlog file. |
| Resource footprint too big for small clusters | Adoption blocked | `small` profile from day one; publish measured footprint. |
| Auth complexity per project | Install friction | oauth2-proxy with documented Keycloak/Entra/Google examples; "no-auth" dev mode. |
| Overlaps with Grafana's own `docker-otel-lgtm` / k8s-monitoring chart | Reinventing | Evaluate depending on the k8s-monitoring chart for the Alloy layer instead of writing it. |

## 11. Open decisions (decide before M0 ends)

1. **BFF language:** Go vs TypeScript. Go = better k8s client and single binary; TS = shared types with UI.
2. **Mimir vs Prometheus as default.** Mimir is heavier; Prometheus may be enough for the target audience.
3. **Depend on Grafana's `k8s-monitoring` Helm chart** for the Alloy/infra-metrics layer, or own that config?
4. **Trace view:** embed Grafana (iframe + URL params) in v1, or build a minimal waterfall immediately?
5. **Tenancy model:** one install per cluster with namespace-scoped views, or one install per project? (v1 recommendation: per project, single tenant.)
6. **Name and license** (Apache-2.0 vs AGPL, given Grafana components are AGPL).
7. **Demo app languages:** which 2–3 stacks to prove auto-instrumentation.

## 12. Success criteria for v1

- Clean cluster → services screen with live data in **< 15 minutes**, no manual Grafana configuration.
- A developer with no Grafana knowledge can find the cause of a failing request (trace + logs) in **< 2 minutes**.
- Total footprint on the `small` profile fits in **< 4 CPU / 8 GiB**.
- Used on **one real project** for two weeks with the team preferring it over raw Grafana for daily checks.

## 13. Explicit v2 backlog (parked)

Own trace waterfall · anomaly/baseline detection · multi-cluster · SLO objects and error budgets · profiling (Pyroscope) · Docker Compose profile · notification routing UI · RBAC per namespace.
