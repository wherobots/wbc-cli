# OAuth provisioning for the Wherobots CLI

The CLI signs in with the **OAuth 2.0 Device Authorization Grant (RFC 8628)**
against WorkOS AuthKit — WorkOS documents this as "CLI Auth". The resulting
access token is a WorkOS AuthKit session bearer that studio-backend already
accepts on every REST route (the same validation path the web app uses), so
the CLI sends `Authorization: Bearer <token>` directly. There is no
token-exchange or API-key-minting step.

## Flow summary

1. `wherobots auth login` POSTs `{domain}/oauth2/device_authorization` with
   `client_id` and `scope=openid profile email offline_access`.
2. The CLI prints the `user_code`, opens `verification_uri_complete` in the
   browser, and polls `{domain}/oauth2/token` with
   `grant_type=urn:ietf:params:oauth:grant-type:device_code`, honoring
   `interval`/`slow_down`.
3. Endpoints are resolved via OIDC discovery
   (`{domain}/.well-known/openid-configuration`), falling back to the
   conventional `/oauth2/*` paths.
4. Tokens persist in `os.UserConfigDir()/wherobots/credentials.json` (0600),
   keyed by OAuth domain. Refresh uses the rotating-refresh-token grant;
   `invalid_grant` deletes the session and asks the user to sign in again.

## WorkOS dashboard checklist (per environment)

1. Create a Connect OAuth client named **"Wherobots CLI"** — the same way as
   the MCP and VS Code extension clients.
2. **Enable the PKCE / public-client toggle.** This marks the client as a
   public client (token-endpoint auth method `none`) and is what enables the
   device authorization grant. Without it, `/oauth2/device_authorization`
   returns `401 unauthorized` even though `/oauth2/authorize` works.
3. No redirect URI is needed for the device flow. If the dashboard requires
   one anyway, set the environment's web console URL — it is never exercised.
4. No custom audience/resource is required (see below).

## No studio-backend change required

The CLI's device-flow tokens are AuthKit **session** tokens: signed by the
AuthKit tenant key, with `iss` = the AuthKit domain and `aud` = the AuthKit
session-app client ID. studio-backend's `WorkOSTokenValidator` selects a
validation profile by matching the token's signing-key `kid`, and these
tokens resolve to the existing **AuthKit session profile** (issuer-checked,
no audience check) — exactly like the web frontend and the VS Code extension.

So the CLI needs **no** `WORKOS_CONNECT_CLIENTS` entry. (The per-client
Connect profiles use JWKS at `api.workos.com/sso/jwks/{client_id}`, which
these AuthKit Connect apps do not expose, so that audience-scoped path is not
what validates them.) A dedicated CLI Connect client is still worthwhile for
per-surface control in the WorkOS dashboard (disable/rotate CLI logins
independently), even though the backend treats the token as a session bearer.

## CLI configuration

The production domain and client ID are baked into
`internal/config/config.go` (`defaultOAuthDomain`, `defaultOAuthClientID`).
Other environments are selected with env vars, mirroring `WHEROBOTS_API_URL`:

```sh
export WHEROBOTS_API_URL='https://api.staging.wherobots.com'
export WHEROBOTS_OAUTH_DOMAIN='<staging-authkit-domain>'
export WHEROBOTS_OAUTH_CLIENT_ID='<staging-client-id>'
```

## Environment reference

| | prod | staging |
|---|---|---|
| AuthKit domain | `https://login.cloud.wherobots.com` | `https://valid-knowledge-14-staging.authkit.app` |
| CLI client ID | `client_01KWJ9Z6GR1HQJCQZ71NWC28JM` | `client_01KWJA08MWADJFSW65A4P8GHTX` |
| API | `https://api.cloud.wherobots.com` | `https://api.staging.wherobots.com` |
