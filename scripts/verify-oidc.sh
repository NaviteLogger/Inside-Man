#!/usr/bin/env bash
# Signs in through a real OIDC provider and checks the whole path.
#
# docs/auth.md shipped for a while saying the OIDC path was wired but never
# exercised against a real issuer. This exercises it: Dex runs in the cluster as
# a throwaway provider, oauth2-proxy is pointed at it, and the script walks the
# authorization code flow the way a browser would, then confirms the session
# actually reaches the UI and the API.
#
# Dex and its static credentials live in examples/dev-oidc and are not shipped.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="${REPO_ROOT}/.tools/bin:${PATH}"
export KUBECONFIG="${REPO_ROOT}/kubeconfig"

NAMESPACE="${NAMESPACE:-inside-man}"
RELEASE="${RELEASE:-inside-man}"
DEMO_NS="${DEMO_NS:-demo}"
CHART="${CHART:-charts/inside-man}"
OVERLAY="${OVERLAY:-examples/dev-oidc/values-oidc.yaml}"

pass=0; fail=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; pass=$((pass+1)); }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$*"; fail=$((fail+1)); }
group(){ printf '\n\033[1m%s\033[0m\n' "$*"; }

# curl runs from a pod, since the flow has to reach cluster-internal addresses.
runner() { kubectl exec -n "${DEMO_NS}" deploy/demo-loadgen -- sh -c "$1" 2>/dev/null; }

group "OIDC: a provider is running"

kubectl apply -f examples/dev-oidc/dex.yaml >/dev/null 2>&1
kubectl rollout status deploy/dex -n dev-oidc --timeout=3m >/dev/null 2>&1 \
  && ok "Dex is running" \
  || bad "Dex did not start"

issuer="$(runner 'curl -s http://dex.dev-oidc.svc:5556/dex/.well-known/openid-configuration' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin).get("issuer",""))' 2>/dev/null || true)"
[[ "${issuer}" == "http://dex.dev-oidc.svc:5556/dex" ]] \
  && ok "discovery document served, issuer ${issuer}" \
  || bad "discovery failed (issuer: ${issuer:-none})"

group "OIDC: the release comes up with authentication on"

helm upgrade "${RELEASE}" "${CHART}" -n "${NAMESPACE}" -f "${OVERLAY}" --wait --timeout 12m >/tmp/oidc-up.log 2>&1 \
  && ok "helm upgrade with oauth2-proxy enabled" \
  || { bad "upgrade with authentication on failed"; tail -5 /tmp/oidc-up.log | sed 's/^/      /'; }

kubectl rollout status deploy/"${RELEASE}"-oauth2-proxy -n "${NAMESPACE}" --timeout=3m >/dev/null 2>&1 \
  && ok "oauth2-proxy is running and reached the issuer" \
  || bad "oauth2-proxy did not become ready"

group "OIDC: the authorization code flow"

# The whole flow runs in one shell so the cookie jar persists across steps.
result="$(runner '
PROXY="http://inside-man-oauth2-proxy.'"${NAMESPACE}"'.svc:80"
DEX="http://dex.dev-oidc.svc:5556"
JAR=$(mktemp); B="-s -c $JAR -b $JAR"

before=$(curl $B -o /dev/null -w "%{http_code}" "$PROXY/api/services")

auth=$(curl $B -o /dev/null -w "%{redirect_url}" "$PROXY/oauth2/start")
login=$(curl $B -o /dev/null -w "%{redirect_url}" "$auth")
lp=$(curl $B -o /dev/null -w "%{redirect_url}" "$login")
st=$(echo "$lp" | sed -n "s/.*state=\([^&]*\).*/\1/p")

approval=$(curl $B -o /dev/null -w "%{redirect_url}" \
  --data-urlencode "login=engineer@example.com" --data-urlencode "password=password" \
  "$DEX/dex/auth/local/login?back=&state=$st")
req=$(echo "$approval" | sed -n "s/.*req=\([^&]*\).*/\1/p")

# oauth2-proxy sends approval_prompt=force, so the consent screen appears on
# every sign-in and a browser would click through it.
cb=$(curl $B -o /dev/null -w "%{redirect_url}" \
  --data-urlencode "req=$req" --data-urlencode "approval=approve" "$approval")
has_code=$(echo "$cb" | grep -c "code=" || true)

curl $B -o /dev/null "$cb"

ui=$(curl $B -o /dev/null -w "%{http_code}" "$PROXY/")
api_code=$(curl $B -o /tmp/api.json -w "%{http_code}" "$PROXY/api/services")
api_type=$(curl $B -o /dev/null -w "%{content_type}" "$PROXY/api/services")
email=$(curl $B -s "$PROXY/oauth2/userinfo" | sed -n "s/.*\"email\":\"\([^\"]*\)\".*/\1/p")

echo "$before|$has_code|$ui|$api_code|$api_type|$email"
')"

IFS='|' read -r before has_code ui api_code api_type email <<<"${result}"

[[ "${before}" == "403" ]] \
  && ok "an unauthenticated API call is refused (${before})" \
  || bad "unauthenticated call returned ${before:-nothing}, expected 403"

[[ "${has_code}" == "1" ]] \
  && ok "the provider issued an authorization code" \
  || bad "no authorization code came back from the provider"

[[ "${ui}" == "200" ]] \
  && ok "the UI is served once signed in (${ui})" \
  || bad "signed-in UI returned ${ui:-nothing}"

[[ "${api_code}" == "200" && "${api_type}" == application/json* ]] \
  && ok "the API answers JSON through the proxy (${api_code})" \
  || bad "signed-in API returned ${api_code:-nothing} as ${api_type:-unknown}"

[[ "${email}" == "engineer@example.com" ]] \
  && ok "the signed-in identity reaches the proxy as ${email}" \
  || bad "userinfo returned '${email:-nothing}', expected engineer@example.com"

group "OIDC: putting the cluster back"

helm upgrade "${RELEASE}" "${CHART}" -n "${NAMESPACE}" --wait --timeout 12m >/dev/null 2>&1 \
  && ok "authentication turned back off" \
  || bad "could not restore the no-auth install"
kubectl delete -f examples/dev-oidc/dex.yaml --ignore-not-found >/dev/null 2>&1 || true

printf '\n\033[1m%d passed, %d failed\033[0m\n' "${pass}" "${fail}"
[[ "${fail}" -eq 0 ]]
