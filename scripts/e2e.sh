#!/usr/bin/env bash
# End-to-end verification against a live cluster.
#
# Assumes the chart is already installed, which `make e2e` handles.
# Assertions are grouped by the milestone whose "done" definition they encode,
# so each milestone adds a block here and reuses this harness.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="${REPO_ROOT}/.tools/bin:${PATH}"
export KUBECONFIG="${REPO_ROOT}/kubeconfig"

NAMESPACE="${NAMESPACE:-inside-man}"
RELEASE="${RELEASE:-inside-man}"

# Design doc 12: the small profile has to fit in 4 CPU and 8 GiB.
BUDGET_CPU_MILLI=4000
BUDGET_MEM_MIB=8192

pass=0; fail=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; pass=$((pass+1)); }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$*"; fail=$((fail+1)); }
group(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

# M0: the bundle installs.
group "M0: the chart installs and comes up"

status="$(helm status "${RELEASE}" -n "${NAMESPACE}" -o json 2>/dev/null \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["info"]["status"])' 2>/dev/null || echo "missing")"
[[ "${status}" == "deployed" ]] \
  && ok "helm release ${RELEASE} is deployed" \
  || bad "helm release ${RELEASE} status is '${status}', expected 'deployed'"

# Wait for readiness, since pods take time to settle.
if kubectl wait --for=condition=Ready pods --all -n "${NAMESPACE}" --timeout=10m >/dev/null 2>&1; then
  ok "all pods in ${NAMESPACE} reached Ready"
else
  bad "not all pods reached Ready within 10m"
  kubectl get pods -n "${NAMESPACE}" --no-headers | awk '$2!~/^([0-9]+)\/\1$/ {print "      " $0}'
fi

# A pod can be Ready and still be crash-looping.
restarts="$(kubectl get pods -n "${NAMESPACE}" -o json | python3 -c '
import json, sys
noisy = []
for pod in json.load(sys.stdin)["items"]:
    name = pod["metadata"]["name"]
    for c in pod["status"].get("containerStatuses") or []:
        if c["restartCount"] > 2:
            noisy.append("%s/%s=%d" % (name, c["name"], c["restartCount"]))
print(",".join(noisy))
')"
[[ -z "${restarts}" ]] \
  && ok "no container has restarted more than twice" \
  || bad "containers restarting: ${restarts}"

group "M0: footprint stays within budget"
read -r cpu mem <<<"$(kubectl get pods -n "${NAMESPACE}" -o json | python3 -c '
import json,sys
def cpu(v):
    if not v: return 0
    return int(v[:-1]) if v.endswith("m") else int(float(v)*1000)
def mem(v):
    if not v: return 0.0
    for suf,mult in (("Ki",1/1024),("Mi",1),("Gi",1024),("K",1/1024),("M",0.954),("G",954)):
        if v.endswith(suf): return float(v[:-len(suf)])*mult
    return float(v)/1048576
c=m=0
for p in json.load(sys.stdin)["items"]:
    for k in p["spec"]["containers"]:
        r=k.get("resources",{}).get("requests",{})
        c+=cpu(r.get("cpu")); m+=mem(r.get("memory"))
print(c, round(m))')"
(( cpu < BUDGET_CPU_MILLI )) \
  && ok "CPU requests ${cpu}m < ${BUDGET_CPU_MILLI}m" \
  || bad "CPU requests ${cpu}m exceeds ${BUDGET_CPU_MILLI}m"
(( mem < BUDGET_MEM_MIB )) \
  && ok "memory requests ${mem}Mi < ${BUDGET_MEM_MIB}Mi" \
  || bad "memory requests ${mem}Mi exceeds ${BUDGET_MEM_MIB}Mi"

# M1: telemetry flows and the join key holds.
DEMO_NS="${DEMO_NS:-demo}"
# Overridable so the assertions themselves can be negative-tested.
read -r -a DEMO_SERVICES <<< "${DEMO_SERVICES:-demo-frontend demo-api demo-backend}"

if kubectl get ns "${DEMO_NS}" >/dev/null 2>&1; then
  group "M1: the demo app produces telemetry"

  kubectl wait --for=condition=Available deploy --all -n "${DEMO_NS}" --timeout=5m >/dev/null 2>&1 \
    && ok "demo app is running" \
    || bad "demo app did not become available"

  # Agents come from a pod annotation alone, so the init container is the
  # evidence that zero-touch instrumentation actually happened.
  injected="$(kubectl get pods -n "${DEMO_NS}" -o json | python3 -c '
import json, sys
found = set()
for pod in json.load(sys.stdin)["items"]:
    for c in pod["spec"].get("initContainers") or []:
        if c["name"].startswith("opentelemetry-auto-instrumentation-"):
            found.add(c["name"].rsplit("-", 1)[-1])
print(",".join(sorted(found)))
')"
  [[ "${injected}" == "java,nodejs,python" ]] \
    && ok "agents auto-injected for java, nodejs and python" \
    || bad "expected java,nodejs,python agents, got '\''${injected}'\''"

  # ---- metrics ----
  promql() {
    kubectl exec -n "${NAMESPACE}" deploy/inside-man-prometheus -c prometheus-server -- \
      wget -qO- "http://localhost:9090/api/v1/query?query=$(python3 -c '
import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$1")" 2>/dev/null
  }

  # Wait on the exact query the assertions use. count() goes non-empty on the
  # first sample, but rate() over 5m needs two samples spread across the window,
  # so a young cluster reports metrics that no rate query can yet evaluate.
  printf '    waiting for span metrics to be rateable'
  for _ in $(seq 1 40); do
    ready="$(promql 'sum by (service_name) (rate(traces_spanmetrics_calls_total[5m]))' | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
print(",".join(sorted(r["metric"].get("service_name", "") for r in res)))
')"
    missing=0
    for svc in "${DEMO_SERVICES[@]}"; do
      [[ "${ready}" == *"${svc}"* ]] || missing=1
    done
    (( missing == 0 )) && break
    printf '.'
    sleep 15
  done
  printf '\n'

  seen="$(promql 'sum by (service_name) (rate(traces_spanmetrics_calls_total[5m]))' | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: print(""); raise SystemExit
print(",".join(sorted(r["metric"].get("service_name", "?") for r in res)))
')"
  for svc in "${DEMO_SERVICES[@]}"; do
    [[ "${seen}" == *"${svc}"* ]] \
      && ok "span metrics present for ${svc}" \
      || bad "no span metrics for ${svc} (saw: ${seen:-none})"
  done

  # Dependencies, which drive the service map in M4.
  edges="$(promql 'sum by (client, server) (rate(traces_service_graph_request_total[5m]))' | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
pairs = []
for r in res:
    m = r["metric"]
    pairs.append(m.get("client", "?") + ">" + m.get("server", "?"))
print(",".join(sorted(pairs)))
')"
  [[ "${edges}" == *"demo-frontend>demo-api"* && "${edges}" == *"demo-api>demo-backend"* ]] \
    && ok "service graph shows the frontend to api to backend chain" \
    || bad "service graph missing expected edges (saw: ${edges:-none})"

  # ---- the join key ----
  group "M1: the join key holds across all three signals"

  loki() {
    kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- curl -s -G \
      "http://inside-man-loki-gateway.${NAMESPACE}.svc:80/loki/api/v1/query_range" \
      --data-urlencode "query=$1" \
      --data-urlencode "start=$(( $(date +%s) - 1800 ))000000000" \
      --data-urlencode "end=$(date +%s)000000000" \
      --data-urlencode "limit=${2:-200}" 2>/dev/null
  }

  log_svcs="$(loki '{k8s_namespace_name="'"${DEMO_NS}"'"}' | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
print(",".join(sorted({s["stream"].get("service_name", "?") for s in res})))
')"
  for svc in "${DEMO_SERVICES[@]}"; do
    [[ "${log_svcs}" == *"${svc}"* ]] \
      && ok "logs indexed under service_name=${svc}" \
      || bad "no logs under service_name=${svc} (saw: ${log_svcs:-none})"
  done

  # The pivot the whole product rests on: one trace id, logs from every service
  # it passed through.
  trace_id="$(loki '{k8s_namespace_name="'"${DEMO_NS}"'"}' | python3 -c '
import json, re, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
seen = {}
for stream in res:
    svc = stream["stream"].get("service_name", "?")
    for _, line in stream["values"]:
        m = re.search(r"\"trace_id\":\s*\"([a-f0-9]{32})\"", line)
        if m: seen.setdefault(m.group(1), set()).add(svc)
multi = [t for t, s in seen.items() if len(s) >= 3]
print(multi[0] if multi else "")
')"
  if [[ -n "${trace_id}" ]]; then
    ok "found a trace id spanning all three services in logs"
    hit_svcs="$(loki '{k8s_namespace_name="'"${DEMO_NS}"'"} |= "'"${trace_id}"'"' 50 | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
print(",".join(sorted({s["stream"].get("service_name", "?") for s in res})))
')"
    [[ "${hit_svcs}" == *"demo-frontend"* && "${hit_svcs}" == *"demo-api"* && "${hit_svcs}" == *"demo-backend"* ]] \
      && ok "logs for that trace resolve across all three services" \
      || bad "trace pivot returned only: ${hit_svcs:-none}"
  else
    bad "no trace id correlated logs from all three services"
  fi

  # Correlation is only real if a trace id found in logs resolves to a trace in
  # Tempo carrying the whole chain. That is the pivot the product is built on.
  group "M1: trace and logs resolve to each other"

  grafana() {
    local pw
    pw="$(kubectl get secret inside-man-grafana -n "${NAMESPACE}" -o jsonpath='{.data.admin-password}' | base64 -d)"
    kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- curl -s -u "admin:${pw}" "$@" 2>/dev/null
  }
  GRAF="http://inside-man-grafana.${NAMESPACE}.svc:80"

  # Grafana 13 ships a distroless image with no shell, so its API is reached
  # from a pod that does have curl.
  ds="$(grafana "${GRAF}/api/datasources" | python3 -c '
import json, sys
try: res = json.load(sys.stdin)
except Exception: res = []
print(",".join(sorted(d["uid"] for d in res)))
')"
  [[ "${ds}" == "loki,prometheus,tempo" ]] \
    && ok "datasources provisioned: ${ds}" \
    || bad "expected loki,prometheus,tempo datasources, got '\''${ds}'\''"

  links="$(grafana "${GRAF}/api/datasources/uid/tempo" | python3 -c '
import json, sys
try: j = json.load(sys.stdin)["jsonData"]
except Exception: j = {}
out = []
if j.get("tracesToLogsV2", {}).get("datasourceUid") == "loki": out.append("logs")
if j.get("tracesToMetrics", {}).get("datasourceUid") == "prometheus": out.append("metrics")
if j.get("serviceMap", {}).get("datasourceUid") == "prometheus": out.append("servicemap")
print(",".join(out))
')"
  [[ "${links}" == "logs,metrics,servicemap" ]] \
    && ok "trace correlations wired to logs, metrics and service map" \
    || bad "Tempo correlations incomplete: '\''${links}'\''"

  if [[ -n "${trace_id}" ]]; then
    chain="$(grafana "${GRAF}/api/datasources/proxy/uid/tempo/api/v2/traces/${trace_id}" | python3 -c '
import json, sys
try: d = json.load(sys.stdin)
except Exception: print(""); raise SystemExit
d = d.get("trace", d)
svcs = set()
for b in d.get("resourceSpans", []):
    for a in b.get("resource", {}).get("attributes", []):
        if a["key"] == "service.name":
            svcs.add(a["value"].get("stringValue"))
print(",".join(sorted(svcs)))
')"
    [[ "${chain}" == "demo-api,demo-backend,demo-frontend" ]] \
      && ok "that trace resolves in Tempo across all three services" \
      || bad "trace ${trace_id} in Tempo covers only: ${chain:-nothing}"
  fi

  group "M1: span metric cardinality stays inside the budget"
  series="$(promql 'count(traces_spanmetrics_calls_total)' | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
print(res[0]["value"][1] if res else "0")
')"
  # A three-service demo has no business producing hundreds of series. A jump
  # here means a high-cardinality dimension crept into the connector config.
  (( series < 100 )) \
    && ok "traces_spanmetrics_calls_total has ${series} series, under 100" \
    || bad "traces_spanmetrics_calls_total has ${series} series, cardinality budget breached"
  # M2: the services list, its health model, and the UI in front of it.
  group "M2: the API answers with services joined to workloads"

  bff() {
    kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- curl -s \
      "http://inside-man-bff.${NAMESPACE}.svc:8080$1" 2>/dev/null
  }
  ui() {
    kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- curl -s \
      "http://inside-man-ui.${NAMESPACE}.svc:80$1" 2>/dev/null
  }
  ui_status() {
    kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- curl -s -o /dev/null \
      -w '%{http_code}' "http://inside-man-ui.${NAMESPACE}.svc:80$1" 2>/dev/null
  }

  listed="$(bff /api/services | python3 -c '
import json, sys
try: svcs = json.load(sys.stdin)["services"]
except Exception: svcs = []
print(",".join(sorted(s["name"] for s in svcs)))
')"
  for svc in "${DEMO_SERVICES[@]}"; do
    [[ "${listed}" == *"${svc}"* ]] \
      && ok "/api/services lists ${svc}" \
      || bad "/api/services is missing ${svc} (saw: ${listed:-none})"
  done

  joined="$(bff /api/services | python3 -c '
import json, sys
try: svcs = json.load(sys.stdin)["services"]
except Exception: svcs = []
print(sum(1 for s in svcs if s.get("workload") and s["workload"].get("desired", 0) > 0))
')"
  (( joined >= 3 )) \
    && ok "all three services joined to a Deployment with pod counts" \
    || bad "only ${joined} services carried workload detail"

  # A number the user cannot explain is worse than no number at all.
  explained="$(bff /api/services | python3 -c '
import json, sys
try: svcs = json.load(sys.stdin)["services"]
except Exception: svcs = []
bad = [s["name"] for s in svcs
       if s["health"]["status"] != "healthy" and not s["health"].get("reasons")]
print(",".join(bad))
')"
  [[ -z "${explained}" ]] \
    && ok "every non-healthy status carries a reason" \
    || bad "status without a reason: ${explained}"

  group "M2: diagnostics and the UI"

  failing="$(bff /api/diagnostics | python3 -c '
import json, sys
try: checks = json.load(sys.stdin)["checks"]
except Exception: checks = []
print(",".join(c["name"] for c in checks if c["status"] == "fail"))
')"
  [[ -z "${failing}" ]] \
    && ok "diagnostics reports no failing checks" \
    || bad "diagnostics failing: ${failing}"

  [[ "$(ui_status /)" == "200" ]] \
    && ok "UI serves its shell" \
    || bad "UI did not return 200"

  # Client-side routes have no file behind them, so this proves the fallback.
  [[ "$(ui_status /diagnostics)" == "200" ]] \
    && ok "UI serves client-side routes" \
    || bad "UI has no SPA fallback for /diagnostics"

  via_ui="$(ui /api/services | python3 -c '
import json, sys
try: print(len(json.load(sys.stdin)["services"]))
except Exception: print(0)
')"
  (( via_ui >= 3 )) \
    && ok "UI proxies the API on its own origin" \
    || bad "UI proxy returned ${via_ui} services"

  # The health model has to react to a real outage, which is M2's whole point.
  group "M2: health reacts to a broken service"

  # Errors from an earlier run stay in the query window. Waiting for them to
  # age out keeps this independent of whatever ran before.
  printf '    waiting for a healthy baseline'
  before=""
  for _ in $(seq 1 28); do
    before="$(bff /api/services | python3 -c '
import json, sys
try: svcs = json.load(sys.stdin)["services"]
except Exception: svcs = []
print(",".join(sorted({s["health"]["status"] for s in svcs})))
')"
    [[ "${before}" == "healthy" ]] && break
    printf '.'
    sleep 15
  done
  printf '\n'

  [[ "${before}" == "healthy" ]] \
    && ok "every service is healthy before the outage" \
    || bad "no healthy baseline within 7m, saw: ${before:-none}"

  # Scaling to zero gives a sustained outage. Deleting a single pod recovers so
  # fast that the error ratio hovers on the threshold and flaps.
  kubectl scale deploy/demo-backend -n "${DEMO_NS}" --replicas=0 >/dev/null 2>&1
  printf '    backend scaled to zero, watching health'
  degraded=""
  for _ in $(seq 1 18); do
    sleep 15
    printf '.'
    degraded="$(bff /api/services | python3 -c '
import json, sys
try: svcs = json.load(sys.stdin)["services"]
except Exception: svcs = []
print(",".join(sorted(s["name"] for s in svcs if s["health"]["status"] == "critical")))
')"
    [[ -n "${degraded}" ]] && break
  done
  printf '\n'

  [[ -n "${degraded}" ]] \
    && ok "a service turned critical after the outage: ${degraded}" \
    || bad "no service turned critical within 4m of the backend going away"

  kubectl scale deploy/demo-backend -n "${DEMO_NS}" --replicas=1 >/dev/null 2>&1
  kubectl rollout status deploy/demo-backend -n "${DEMO_NS}" --timeout=3m >/dev/null 2>&1 \
    && ok "backend restored" \
    || bad "backend did not come back after the test"

  # Error spans are recorded, which the outage above has now guaranteed.
  errs="$(promql 'sum(rate(traces_spanmetrics_calls_total{status_code="STATUS_CODE_ERROR"}[5m]))' | python3 -c '
import json, sys
try: res = json.load(sys.stdin)["data"]["result"]
except Exception: res = []
print(res[0]["value"][1] if res else "0")
')"
  python3 -c "import sys; sys.exit(0 if float('${errs}') > 0 else 1)" 2>/dev/null \
    && ok "error-path spans are recorded" \
    || bad "no error spans recorded, the health model is unexercised"
else
  group "M1 and M2: skipped, no ${DEMO_NS} namespace"
fi

printf '\n\033[1m%d passed, %d failed\033[0m\n' "${pass}" "${fail}"
[[ "${fail}" -eq 0 ]]
