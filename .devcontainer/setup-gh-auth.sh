#!/usr/bin/env bash
# Wires gh, git identity and commit signing from the workspace .env.
# Safe to re-run: it replaces what it wrote last time.
set -euo pipefail

# Derived from this script's own location, so the workspace can be renamed or
# cloned elsewhere with no edits here.
WORKSPACE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-${WORKSPACE_DIR}/.env}"
SIGN_DIR="${SIGN_DIR:-${WORKSPACE_DIR}/.ssh-signing}"

SHELL_RCS=("$HOME/.zshrc" "$HOME/.bashrc")
BEGIN_MARKER="# >>> gh-auth-from-workspace-env >>>"
END_MARKER="# <<< gh-auth-from-workspace-env <<<"

read_token() {
  [ -r "$1" ] || return 0
  awk -F= '/^GITHUB_TOKEN=/ { sub(/^GITHUB_TOKEN=/, ""); print; exit }' "$1"
}

# The path is baked in at install time, since the snippet runs at shell startup
# where there is no script to derive it from.
snippet() {
  sed "s|@@ENV_FILE@@|${ENV_FILE}|g" <<'EOF'
# >>> gh-auth-from-workspace-env >>>
if [ -r "@@ENV_FILE@@" ]; then
  _gh_token=$(awk -F= '/^GITHUB_TOKEN=/ { sub(/^GITHUB_TOKEN=/, ""); print; exit }' "@@ENV_FILE@@")
  if [ -n "$_gh_token" ]; then
    export GITHUB_TOKEN="$_gh_token"
    export GH_TOKEN="$_gh_token"
  fi
  unset _gh_token
fi
# <<< gh-auth-from-workspace-env <<<
EOF
}

for rc in "${SHELL_RCS[@]}"; do
  [ -f "$rc" ] || touch "$rc"
  if grep -qF "$BEGIN_MARKER" "$rc"; then
    # Drop the previous block so a moved workspace picks up the new path.
    # Skipping would leave a stale one in place.
    awk -v b="$BEGIN_MARKER" -v e="$END_MARKER" '
      $0 == b { skip = 1 }
      !skip   { print }
      $0 == e { skip = 0 }
    ' "$rc" > "$rc.tmp" && mv "$rc.tmp" "$rc"
    echo "refreshed GITHUB_TOKEN snippet in $rc"
  else
    echo "added GITHUB_TOKEN snippet to $rc"
  fi
  printf '\n%s\n' "$(snippet)" >> "$rc"
done

TOKEN="$(read_token "$ENV_FILE")"
if [ -n "${TOKEN:-}" ]; then
  GITHUB_TOKEN="$TOKEN" gh auth setup-git
  echo "gh auth setup-git done"

  GH_LOGIN=$(GITHUB_TOKEN="$TOKEN" gh api user --jq '.login' 2>/dev/null || true)
  GH_NAME=$(GITHUB_TOKEN="$TOKEN" gh api user --jq '.name // .login' 2>/dev/null || true)
  GH_ID=$(GITHUB_TOKEN="$TOKEN" gh api user --jq '.id' 2>/dev/null || true)
  GH_EMAIL=$(GITHUB_TOKEN="$TOKEN" gh api user/emails \
    --jq 'map(select(.primary and .verified)) | .[0].email // empty' 2>/dev/null || true)

  case "$GH_EMAIL" in
    *@*) ;;
    *) GH_EMAIL="" ;;
  esac
  if [ -z "$GH_EMAIL" ] && [ -n "$GH_ID" ] && [ -n "$GH_LOGIN" ]; then
    GH_EMAIL="${GH_ID}+${GH_LOGIN}@users.noreply.github.com"
  fi

  [ -n "$GH_NAME" ]  && git config --global user.name  "$GH_NAME"  && echo "git user.name  = $GH_NAME"
  [ -n "$GH_EMAIL" ] && git config --global user.email "$GH_EMAIL" && echo "git user.email = $GH_EMAIL"
elif [ -r "$ENV_FILE" ]; then
  echo "no GITHUB_TOKEN in $ENV_FILE, skipping gh auth and identity"
else
  echo "$ENV_FILE missing, skipping gh auth and identity"
fi

SIGN_KEY="${SIGN_DIR}/id_ed25519_signing.pub"
SIGNERS_FILE="${SIGN_DIR}/allowed_signers"
if [ -r "$SIGN_KEY" ]; then
  git config --global gpg.format ssh
  git config --global commit.gpgsign true
  git config --global tag.gpgsign true
  git config --global user.signingkey "$SIGN_KEY"
  git config --global gpg.ssh.allowedSignersFile "$SIGNERS_FILE"
  EMAIL="$(git config --global user.email || true)"
  if [ -n "$EMAIL" ]; then
    printf '%s %s\n' "$EMAIL" "$(cat "$SIGN_KEY")" > "$SIGNERS_FILE"
    chmod 600 "$SIGNERS_FILE"
  fi
  echo "ssh commit signing wired (key: $SIGN_KEY, signer: ${EMAIL:-unset})"
else
  echo "no signing key at $SIGN_KEY, skipping ssh signing config"
fi
