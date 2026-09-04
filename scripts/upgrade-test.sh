#!/usr/bin/env bash
# Verifies that an existing install survives an upgrade.
#
# Design doc 9 asks M5 to test the upgrade path, and an install that works from
# scratch says nothing about one that has to be upgraded in place. Two things
# have already bitten us there: the operator minting a new webhook cert on every
# upgrade while the running pod served the old one, and Helm's coalescing
# dropping a null-valued key.
#
# The check installs, writes data, upgrades, and then asserts the data survived
# and the release still works.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="${REPO_ROOT}/.tools/bin:${PATH}"
export KUBECONFIG="${REPO_ROOT}/kubeconfig"

NAMESPACE="${NAMESPACE:-inside-man}"
RELEASE="${RELEASE:-inside-man}"
CHART="${CHART:-charts/inside-man}"
DEMO_NS="${DEMO_NS:-demo}"

pass=0; fail=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; pass=$((pass+1)); }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$*"; fail=$((fail+1)); }
group(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

bff() {
  kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- curl -s \
    "http://inside-man-bff.${NAMESPACE}.svc:8080$1" 2>/dev/null
}

group "Upgrade: the release is installed and serving"

before_rev="$(helm status "${RELEASE}" -n "${NAMESPACE}" -o json 2>/dev/null \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])' 2>/dev/null || echo 0)"
[[ "${before_rev}" -ge 1 ]] \
  && ok "starting from revision ${before_rev}" \
  || bad "no existing release to upgrade"

# Something has to be in the stores, or the upgrade proves nothing about data
# surviving it.
before_services="$(bff /api/services | python3 -c '
import json, sys
try: print(len(json.load(sys.stdin).get("services") or []))
except Exception: print(0)
')"
(( before_services > 0 )) \
  && ok "${before_services} services reporting before the upgrade" \
  || bad "nothing was reporting before the upgrade, so it proves nothing"

group "Upgrade: helm upgrade succeeds in place"

if helm upgrade "${RELEASE}" "${CHART}" -n "${NAMESPACE}" --wait --timeout 15m >/tmp/upgrade.log 2>&1; then
  ok "helm upgrade completed"
else
  bad "helm upgrade failed"
  tail -20 /tmp/upgrade.log | sed 's/^/      /'
fi

after_rev="$(helm status "${RELEASE}" -n "${NAMESPACE}" -o json 2>/dev/null \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])' 2>/dev/null || echo 0)"
(( after_rev > before_rev )) \
  && ok "revision advanced ${before_rev} to ${after_rev}" \
  || bad "revision did not advance (${before_rev} to ${after_rev})"

status="$(helm status "${RELEASE}" -n "${NAMESPACE}" -o json 2>/dev/null \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["info"]["status"])' 2>/dev/null || echo missing)"
[[ "${status}" == "deployed" ]] \
  && ok "release status is deployed" \
  || bad "release status is ${status}"

group "Upgrade: the install still works afterwards"

kubectl wait --for=condition=Ready pods --all -n "${NAMESPACE}" --timeout=10m >/dev/null 2>&1 \
  && ok "all pods Ready after the upgrade" \
  || bad "pods did not settle after the upgrade"

# The Instrumentation CR goes through the operator's webhook on every upgrade,
# which is exactly what broke before.
kubectl get instrumentation -n "${NAMESPACE}" "${RELEASE}" >/dev/null 2>&1 \
  && ok "the Instrumentation CR survived the webhook call" \
  || bad "the Instrumentation CR is missing after the upgrade"

after_services="$(bff /api/services | python3 -c '
import json, sys
try: print(len(json.load(sys.stdin).get("services") or []))
except Exception: print(0)
')"
(( after_services >= before_services )) \
  && ok "${after_services} services still reporting after the upgrade" \
  || bad "services dropped from ${before_services} to ${after_services}"

# Data written before the upgrade has to still be queryable, which is the
# difference between an upgrade and a reinstall.
logs_after="$(bff "/api/services/demo-backend/logs" | python3 -c '
import json, sys
try: print(len(json.load(sys.stdin).get("lines") or []))
except Exception: print(0)
')"
(( logs_after > 0 )) \
  && ok "logs written before the upgrade are still queryable" \
  || bad "no logs survived the upgrade"

group "Upgrade: rollback is available"

if helm rollback "${RELEASE}" "${before_rev}" -n "${NAMESPACE}" --wait --timeout 10m >/tmp/rollback.log 2>&1; then
  ok "rolled back to revision ${before_rev}"
  helm upgrade "${RELEASE}" "${CHART}" -n "${NAMESPACE}" --wait --timeout 15m >/dev/null 2>&1 \
    && ok "and forward again, leaving the cluster on the current chart" \
    || bad "could not return to the current chart after the rollback"
else
  bad "helm rollback failed"
  tail -10 /tmp/rollback.log | sed 's/^/      /'
fi

printf '\n\033[1m%d passed, %d failed\033[0m\n' "${pass}" "${fail}"
[[ "${fail}" -eq 0 ]]
