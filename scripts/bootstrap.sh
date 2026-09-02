#!/usr/bin/env bash
# Installs the pinned toolchain into .tools/bin.
# Re-running is a no-op once each tool reports its pinned version.
# Bump versions here and nowhere else.
set -euo pipefail

KUBECTL_VERSION="v1.37.0"
KIND_VERSION="v0.33.0"
HELM_VERSION="v4.2.4"
GO_VERSION="1.27.1"
GITLEAKS_VERSION="8.30.1"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="${REPO_ROOT}/.tools"
BIN_DIR="${TOOLS_DIR}/bin"
mkdir -p "${BIN_DIR}"

case "$(uname -m)" in
  x86_64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
skip() { printf '    %s already at pinned version\n' "$*"; }

# Verify $1 (file) against expected sha256 $2.
verify() {
  local file="$1" expected="$2" actual
  actual="$(sha256sum "${file}" | cut -d' ' -f1)"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "checksum mismatch for ${file}" >&2
    echo "  expected: ${expected}" >&2
    echo "  actual:   ${actual}" >&2
    exit 1
  fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

# --- kubectl -----------------------------------------------------------------
if [[ "$("${BIN_DIR}/kubectl" version --client -o json 2>/dev/null | grep -o "\"gitVersion\": *\"${KUBECTL_VERSION}\"" || true)" ]]; then
  skip kubectl
else
  log "installing kubectl ${KUBECTL_VERSION}"
  base="https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${OS}/${ARCH}"
  curl -fsSL -o "${tmp}/kubectl" "${base}/kubectl"
  verify "${tmp}/kubectl" "$(curl -fsSL "${base}/kubectl.sha256")"
  install -m 0755 "${tmp}/kubectl" "${BIN_DIR}/kubectl"
fi

# --- kind --------------------------------------------------------------------
if [[ "$("${BIN_DIR}/kind" version 2>/dev/null | grep -o "${KIND_VERSION}" || true)" ]]; then
  skip kind
else
  log "installing kind ${KIND_VERSION}"
  base="https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}"
  curl -fsSL -o "${tmp}/kind" "${base}/kind-${OS}-${ARCH}"
  verify "${tmp}/kind" "$(curl -fsSL "${base}/kind-${OS}-${ARCH}.sha256sum" | cut -d' ' -f1)"
  install -m 0755 "${tmp}/kind" "${BIN_DIR}/kind"
fi

# --- helm --------------------------------------------------------------------
if [[ "$("${BIN_DIR}/helm" version --short 2>/dev/null | grep -o "${HELM_VERSION}" || true)" ]]; then
  skip helm
else
  log "installing helm ${HELM_VERSION}"
  tarball="helm-${HELM_VERSION}-${OS}-${ARCH}.tar.gz"
  curl -fsSL -o "${tmp}/${tarball}" "https://get.helm.sh/${tarball}"
  verify "${tmp}/${tarball}" "$(curl -fsSL "https://get.helm.sh/${tarball}.sha256sum" | cut -d' ' -f1)"
  tar -xzf "${tmp}/${tarball}" -C "${tmp}"
  install -m 0755 "${tmp}/${OS}-${ARCH}/helm" "${BIN_DIR}/helm"
fi

# --- go ----------------------------------------------------------------------
# Unpacked under .tools/go with a shim on PATH so it cannot collide with a
# system Go install.
if [[ "$("${BIN_DIR}/go" version 2>/dev/null | grep -o "go${GO_VERSION}" || true)" ]]; then
  skip go
else
  log "installing go ${GO_VERSION}"
  tarball="go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
  sha="$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' | python3 -c '
import json, sys
want = sys.argv[1]
for rel in json.load(sys.stdin):
    for f in rel.get("files", []):
        if f.get("filename") == want:
            print(f["sha256"]); sys.exit(0)
sys.exit(1)
' "${tarball}" || true)"
  if [[ -z "${sha}" ]]; then echo "could not resolve checksum for ${tarball}" >&2; exit 1; fi
  curl -fsSL -o "${tmp}/${tarball}" "https://go.dev/dl/${tarball}"
  verify "${tmp}/${tarball}" "${sha}"
  rm -rf "${TOOLS_DIR}/go"
  tar -xzf "${tmp}/${tarball}" -C "${TOOLS_DIR}"
  for cmd in go gofmt; do
    printf '#!/usr/bin/env bash\nexec "%s/go/bin/%s" "$@"\n' "${TOOLS_DIR}" "${cmd}" > "${BIN_DIR}/${cmd}"
    chmod 0755 "${BIN_DIR}/${cmd}"
  done
fi

# --- gitleaks ----------------------------------------------------------------
# The devcontainer declares GITLEAKS_VERSION as a build arg without putting the
# binary on PATH.
if [[ "$("${BIN_DIR}/gitleaks" version 2>/dev/null | grep -o "${GITLEAKS_VERSION}" || true)" ]]; then
  skip gitleaks
else
  log "installing gitleaks ${GITLEAKS_VERSION}"
  case "${ARCH}" in amd64) gl_arch=x64 ;; arm64) gl_arch=arm64 ;; esac
  base="https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}"
  tarball="gitleaks_${GITLEAKS_VERSION}_${OS}_${gl_arch}.tar.gz"
  curl -fsSL -o "${tmp}/${tarball}" "${base}/${tarball}"
  verify "${tmp}/${tarball}" \
    "$(curl -fsSL "${base}/gitleaks_${GITLEAKS_VERSION}_checksums.txt" | grep " ${tarball}\$" | cut -d' ' -f1)"
  tar -xzf "${tmp}/${tarball}" -C "${tmp}" gitleaks
  install -m 0755 "${tmp}/gitleaks" "${BIN_DIR}/gitleaks"
fi

# --- git hooks ---------------------------------------------------------------
# The workspace .env holds a real GitHub token for the devcontainer's gh auth,
# so the secret-scanning hook matters here.
if [[ -d "${REPO_ROOT}/.git" ]]; then
  install -m 0755 "${REPO_ROOT}/scripts/pre-commit" "${REPO_ROOT}/.git/hooks/pre-commit"
fi

log "toolchain ready in ${BIN_DIR}"
"${BIN_DIR}/kubectl" version --client 2>/dev/null | head -1
"${BIN_DIR}/kind" version
"${BIN_DIR}/helm" version --short
"${BIN_DIR}/go" version
"${BIN_DIR}/gitleaks" version
