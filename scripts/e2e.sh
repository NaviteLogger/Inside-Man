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

# M1 adds: span metrics exist per demo service, logs exist under the matching
# service_name label, a trace_id taken from a log resolves in Tempo, and active
# series stay under the cardinality budget.

printf '\n\033[1m%d passed, %d failed\033[0m\n' "${pass}" "${fail}"
[[ "${fail}" -eq 0 ]]
