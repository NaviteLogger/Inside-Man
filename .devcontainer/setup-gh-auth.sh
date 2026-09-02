#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="/workspaces/Dedicated-Server-Project/.env"
SHELL_RCS=("$HOME/.zshrc" "$HOME/.bashrc")
SNIPPET_MARKER="# >>> gh-auth-from-workspace-env >>>"

snippet() {
  cat <<'EOF'
# >>> gh-auth-from-workspace-env >>>
if [ -r /workspaces/Dedicated-Server-Project/.env ]; then
  _GITHUB_TOKEN_FROM_ENV=$(awk -F= '/^GITHUB_TOKEN=/ { sub(/^GITHUB_TOKEN=/, ""); print; exit }' /workspaces/Dedicated-Server-Project/.env)
  if [ -n "$_GITHUB_TOKEN_FROM_ENV" ]; then
    export GITHUB_TOKEN="$_GITHUB_TOKEN_FROM_ENV"
    export GITHUB_TOKEN="$_GITHUB_TOKEN_FROM_ENV"
  fi
  unset _GITHUB_TOKEN_FROM_ENV
fi
# <<< gh-auth-from-workspace-env <<<
EOF
}

for rc in "${SHELL_RCS[@]}"; do
  [ -f "$rc" ] || touch "$rc"
  if ! grep -qF "$SNIPPET_MARKER" "$rc"; then
    printf "\n%s\n" "$(snippet)" >> "$rc"
    echo "added GITHUB_TOKEN export snippet to $rc"
  else
    echo "GITHUB_TOKEN export snippet already present in $rc — skip"
  fi
done

if [ -r "$ENV_FILE" ]; then
  TOKEN=$(awk -F= '/^GITHUB_TOKEN=/ { sub(/^GITHUB_TOKEN=/, ""); print; exit }' "$ENV_FILE")
  if [ -n "${TOKEN:-}" ]; then
    GITHUB_TOKEN="$TOKEN" gh auth setup-git
    echo "gh auth setup-git done"

    GH_LOGIN=$(GITHUB_TOKEN="$TOKEN" gh api user --jq '.login' 2>/dev/null || true)
    GH_NAME=$(GITHUB_TOKEN="$TOKEN" gh api user --jq '.name // .login' 2>/dev/null || true)
    GH_ID=$(GITHUB_TOKEN="$TOKEN" gh api user --jq '.id' 2>/dev/null || true)
    GH_EMAIL=$(GITHUB_TOKEN="$TOKEN" gh api user/emails \
      --jq 'map(select(.primary and .verified)) | .[0].email // empty' 2>/dev/null || true)
    case "$GH_EMAIL" in
      \{*|*[$' \t']*|"") GH_EMAIL="" ;;
      *@*) ;;
      *) GH_EMAIL="" ;;
    esac
    if [ -z "${GH_EMAIL:-}" ] && [ -n "${GH_ID:-}" ] && [ -n "${GH_LOGIN:-}" ]; then
      GH_EMAIL="${GH_ID}+${GH_LOGIN}@users.noreply.github.com"
    fi

    if [ -n "${GH_NAME:-}" ]; then
      git config --global user.name "$GH_NAME"
      echo "git user.name  = $GH_NAME"
    fi
    if [ -n "${GH_EMAIL:-}" ]; then
      git config --global user.email "$GH_EMAIL"
      echo "git user.email = $GH_EMAIL"
    fi
  else
    echo "no GITHUB_TOKEN in $ENV_FILE — skip gh auth setup-git + identity (rerun after pasting token)"
  fi
else
  echo "$ENV_FILE missing — skip gh auth setup-git + identity (rerun after creating .env)"
fi

SIGN_KEY="/workspace/.ssh-signing/id_ed25519_signing.pub"
SIGNERS_FILE="/workspace/.ssh-signing/allowed_signers"
if [ -r "$SIGN_KEY" ]; then
  git config --global gpg.format ssh
  git config --global commit.gpgsign true
  git config --global tag.gpgsign true
  git config --global user.signingkey "$SIGN_KEY"
  git config --global gpg.ssh.allowedSignersFile "$SIGNERS_FILE"
  EMAIL=$(git config --global user.email || true)
  if [ -n "$EMAIL" ]; then
    printf '%s %s\n' "$EMAIL" "$(cat "$SIGN_KEY")" > "$SIGNERS_FILE"
    chmod 600 "$SIGNERS_FILE"
  fi
  echo "ssh commit signing wired (key: $SIGN_KEY, signer: ${EMAIL:-<unset>})"
else
  echo "no signing key at $SIGN_KEY — skip ssh signing config (generate one then re-run)"
fi