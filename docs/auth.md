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

## Status

The wiring above is shipped and renders, and the no-auth path is covered by the
e2e suite. **The OIDC path has not been exercised against a real identity
provider yet.** It is on the M5 hardening list, along with the documented
Keycloak and Entra examples the design doc asks for.
