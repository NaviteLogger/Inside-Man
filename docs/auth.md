# Authentication

Inside Man ships with authentication off, so `helm install` gives a working
stack with no identity provider. That suits a laptop cluster and it is what the
e2e suite runs against. Turn it on for anything shared.

## What it looks like when enabled

`oauth2-proxy` sits in front of the UI and Grafana, so one OIDC login covers
both. The UI serves the API on its own origin, so there is one cookie domain
and no CORS.

```
browser -> oauth2-proxy -> UI (nginx) -> /api -> BFF
                        -> Grafana
```

## Turning it on

Create a secret with the three values oauth2-proxy needs, so none of them sit in
a values file or in git:

```bash
kubectl create secret generic inside-man-oidc \
  --namespace inside-man \
  --from-literal=client-id="<your client id>" \
  --from-literal=client-secret="<your client secret>" \
  --from-literal=cookie-secret="$(openssl rand -base64 32 | head -c 32 | base64)"
```

Then enable it:

```yaml
oauth2-proxy:
  enabled: true
  config:
    existingSecret: inside-man-oidc
  extraArgs:
    oidc-issuer-url: https://your-issuer.example.com
```

The cookie secret has to be exactly 16, 24 or 32 bytes. oauth2-proxy fails to
start with an unhelpful error when it is not.

## Provider notes

- **Keycloak**: issuer is `https://<host>/realms/<realm>`. Add a client with
  the standard flow enabled and a redirect URI of
  `https://<inside-man-host>/oauth2/callback`.
- **Microsoft Entra ID**: issuer is
  `https://login.microsoftonline.com/<tenant>/v2.0`. The `groups` claim needs
  configuring on the app registration if you plan to restrict by group.
- **Google**: issuer is `https://accounts.google.com`. Set `email-domain` to
  your own domain, since the `*` default admits any Google account.

## Grafana

Grafana accepts the forwarded identity through `auth.proxy`, which the chart
wires when oauth2-proxy is enabled. Users land signed in with the role in
`auth.proxy.auto_sign_up`.

## Gotchas found while verifying this

Each of these cost time, and none of them says what is wrong in its error.

**The cookie secret has to be exactly 16, 24 or 32 bytes.** oauth2-proxy exits
on any other length without naming the setting.

**Redirects off the proxy's own host need a whitelist.** Without
`--whitelist-domain`, oauth2-proxy silently drops the redirect back to your
upstream after sign-in.

**The issuer URL has to be the address everyone uses.** oauth2-proxy verifies
the `iss` claim against `--oidc-issuer-url`, so a provider reachable at two
addresses will fail verification on the one it was not configured with.

**A consent screen appears on every sign-in.** oauth2-proxy sends
`approval_prompt=force` and keeps sending it even when the flag is set empty,
which overrides a provider's own skip-consent setting. Harmless, and worth
knowing before you go looking for the cause.

**An unauthenticated API call gets a 403 where you might expect a redirect.**
oauth2-proxy only redirects requests it takes for browser navigation, so the
UI's `fetch` calls surface as 403 once a session expires.

## Status

Verified end to end. `make verify-oidc` runs Dex in the cluster as a throwaway
provider, walks the authorization code flow the way a browser would, and
asserts that an unauthenticated call is refused, the provider issues a code,
the UI and the API both answer once signed in, and the signed-in identity
reaches the proxy. CI runs it on every change.

Dex and its static credentials live in `examples/dev-oidc` and are not part of
the chart. They exist to be logged into by a script, and no real install should
point at them.

The Keycloak and Entra examples above follow the same shape and have not been
run against those providers.
