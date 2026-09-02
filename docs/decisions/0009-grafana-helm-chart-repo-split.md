# 0009. Source grafana, loki and tempo charts from grafana-community

**Status:** Accepted
**Date:** 2026-09-02

## Context

The design doc assumed `https://grafana.github.io/helm-charts` for the whole
Grafana stack. That is no longer correct, and the failure mode is silent.

Checked against both repositories' `index.yaml` on 2026-09-02:

| Chart | Old repo | Community repo |
|---|---|---|
| `grafana` | 10.5.15 / app 12.3.1, `deprecated: true`, frozen 2026-01-30 | 13.0.1 / app 13.2.0 |
| `tempo` | 1.24.4 / app 2.9.0, `deprecated: true`, frozen | 2.3.0 / app 2.10.8 |
| `tempo-distributed` | 1.61.3 / app 2.9.0, `deprecated: true`, frozen | 3.5.1 / app 3.0.3 |
| `loki` | 7.3.0 / app 3.6.12, no deprecation flag | 18.11.7 / app 3.7.7 |

`loki` is the dangerous one. It has no deprecation flag and recent timestamps,
but its `description` changed to "Helm chart for Grafana Enterprise Logs".
Upstream is explicit: as of 2026-03-16 the OSS chart moved to grafana-community,
forked at 6.55.0, and what remains is for GEL users only.

This was a hand-off, and no kind of abandonment. The charts were always
maintained by community volunteers, and the old `grafana` chart listed zanhsieh,
rtluckie, maorfr, Xtigyro, torstenwalter, jkroepke and QuentinBisson. The
community chart is maintained by jkroepke and QuentinBisson, two of the same
people. Both declare the same upstream project, and Grafana itself went from
12.3.1 to 13.2.0 throughout.

`mimir-distributed`, `alloy`, `alloy-operator`, `k8s-monitoring` and `pyroscope`
are still first-party and stay where they are.

## Decision

Take `grafana`, `loki`, `tempo` and `tempo-distributed` from
`https://grafana-community.github.io/helm-charts`, and keep the ingest path on
`https://grafana.github.io/helm-charts`.

Pin exact versions and vendor the resolved chart tarballs into
`charts/inside-man/charts/` alongside `Chart.lock`.

We do not write our own Grafana, Loki or Tempo manifests to avoid the fork. The
community charts ship no images of their own and reference
`docker.io/grafana/grafana`, `docker.io/grafana/loki` and
`docker.io/grafana/tempo`, all first-party. The exposure is templates only,
which pinning and diff review cover. Writing our own manifests would trade a
small reviewable risk for a large permanent maintenance burden.

## Consequences

- The old repo would pin Grafana to app 12.3.1, losing the bundled Drilldown
  apps that [0005](0005-embed-grafana-drilldown-for-traces-and-logs.md) depends
  on, which makes this more than a cosmetic version lag.
- Three of nine dependencies come from a two-maintainer community org. Exact
  pins, vendored tarballs and kind e2e in CI are the mitigations.
- Renovate has to require manual review for these three. Loki's renumbering from
  6.55.0 to 18.x is a repository change dressed up as a version bump, and no bot
  will bridge it correctly.
- Provenance verification against pre-split releases fails, since they were
  signed with a different key.
- Escape hatch, available but not taken: Grafana, Loki-monolithic and
  Tempo-monolithic are each roughly a Deployment, a ConfigMap and a Service, and
  the images are unaffected.
