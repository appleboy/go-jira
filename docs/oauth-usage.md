# OAuth 2.0 usage guide

go-jira supports the **Jira Data Center** OAuth 2.0 provider. It does **not**
support Jira Cloud (a different OAuth flow on atlassian.com).

Two flows are available:

- **Authorization Code + PKCE** — interactive login for local development.
- **Refresh-token injection** — non-interactive use in CI/CD.

## 1. Register the OAuth client in Jira

A Jira administrator registers an *Application link* / OAuth 2.0 client and
configures a redirect URI. go-jira defaults to:

```txt
http://127.0.0.1:8765/callback
```

Override the port with `--callback-port` (or `JIRA_OAUTH_CALLBACK_PORT`); the
redirect URI is derived from it, so the value registered in Jira must match.
Only one redirect URI needs to be registered.

### HTTPS callback (when Jira rejects an `http` redirect URI)

Jira DC matches the redirect URI exactly and many instances reject the `http`
scheme, returning `invalid redirect_uri`. In that case make the local callback
server serve HTTPS by pointing it at a TLS cert + key for the loopback address:

```bash
# e.g. a cert covering 127.0.0.1 generated with mkcert
mkcert localhost 127.0.0.1

go-jira login \
  --callback-cert=./localhost+1.pem \
  --callback-key=./localhost+1-key.pem
```

When both are set the redirect URI becomes `https://127.0.0.1:<port>/callback`
(register that exact value in Jira). The cert must cover `127.0.0.1`; `mkcert`
installs a local CA so the browser redirect is trusted without a warning. The
equivalent env vars are `JIRA_OAUTH_CALLBACK_CERT` / `JIRA_OAUTH_CALLBACK_KEY`.
Both must be set together, or neither.

#### Zero-setup HTTPS callback (`--callback-https`)

If you just want an https callback to work without provisioning any cert files —
useful when **sharing one binary across a team** — pass `--callback-https` (or
set `JIRA_OAUTH_CALLBACK_HTTPS=true`):

```bash
go-jira login --callback-https
```

This mints a self-signed certificate for `127.0.0.1` **in memory** at login
time. No `mkcert`, no cert/key files, and nothing secret is baked into the
binary. The redirect URI still becomes `https://127.0.0.1:<port>/callback`, so
register that exact value in Jira.

Trade-off: because the cert is self-signed (not signed by a CA your machine
already trusts), the browser shows a one-time security warning. On `127.0.0.1`
you can click **“Proceed to 127.0.0.1 (unsafe)”** to continue; the login then
completes normally. If you want to avoid the warning entirely, use the
`mkcert`-based `--callback-cert` / `--callback-key` approach above instead.

When both `--callback-https` and an explicit `--callback-cert` / `--callback-key`
pair are given, the supplied files win.

You then have a **client ID**. go-jira uses a public PKCE client, so **no client
secret is needed**. The client ID can be:

1. embedded into the binary at build time (see [Building with an embedded client](#building-with-an-embedded-client)), or
2. supplied at runtime via `JIRA_OAUTH_CLIENT_ID`, or
3. passed as `--client-id`.

Resolution order is **env var > flag > embedded default**.

## 2. Scopes

| Scope          | Grants                                              |
| -------------- | --------------------------------------------------- |
| `READ`         | View projects/issues/profile                        |
| `WRITE`        | Create/update issues, comments, transitions (+READ) |
| `ADMIN`        | Admin operations (+READ, WRITE)                     |
| `SYSTEM_ADMIN` | Full system administration (+ADMIN)                 |

go-jira requests `WRITE` by default (enough for transition / comment /
assignee). Change it with `--scope`. Effective permissions are still bounded by
the user's own Jira permissions.

## 3. Local login

```bash
export JIRA_BASE_URL="https://jira.example.com"
go-jira login --client-id="$JIRA_OAUTH_CLIENT_ID"
```

This opens your browser, waits for authorization on the local callback server,
exchanges the code for tokens, and stores them. On success it prints the base
URL, scopes, expiry, and storage backend.

Then run normally — the stored token is selected automatically and refreshed
before expiry:

```bash
go-jira run --ref="ABC-123 fix bug" --to-transition=Done --resolution=Fixed
```

Other commands:

```bash
go-jira whoami                 # who am I + auth mode
go-jira token status           # expiry, scopes, storage backend
go-jira token refresh          # force a refresh now
go-jira token print --confirm  # print the access token (sensitive)
go-jira logout                 # delete the stored token for this site
```

## 4. Token storage

| Backend | When used                                    | Notes                                                              |
| ------- | -------------------------------------------- | ------------------------------------------------------------------ |
| keyring | default, when an OS keyring is available     | macOS Keychain, Secret Service, Windows Credential Manager         |
| file    | fallback (e.g. headless Linux without D-Bus) | AES-256-GCM, key derived with PBKDF2-HMAC-SHA256 (600k iterations) |

The file backend requires a master password via `JIRA_MASTER_PASSWORD`. Tokens
are keyed by `sha256(baseURL:clientID)`, so multiple Jira sites and clients
coexist without clobbering each other. Switch sites with `--base-url`.

## 5. CI/CD (refresh-token injection)

Set `JIRA_OAUTH_REFRESH_TOKEN` to enter `oauth-env` mode. go-jira exchanges the
refresh token for an access token at startup.

| Env var                           | Required             | Purpose                                 |
| --------------------------------- | -------------------- | --------------------------------------- |
| `JIRA_BASE_URL`                   | yes                  | Jira instance base URL                  |
| `JIRA_OAUTH_CLIENT_ID`            | yes                  | OAuth client ID                         |
| `JIRA_OAUTH_REFRESH_TOKEN`        | yes                  | Triggers `oauth-env` mode               |
| `JIRA_OAUTH_REFRESH_TOKEN_OUTPUT` | strongly recommended | File to write the rotated refresh token |

> **⚠️ Rotation is the hard part.** Jira DC invalidates **both** the old access
> token and the old refresh token on every refresh and returns a **new** refresh
> token. go-jira writes that new token to `JIRA_OAUTH_REFRESH_TOKEN_OUTPUT`; your
> pipeline **must** persist it back to its secret store, or the next run fails
> with `invalid_grant`. If you can't automate that, **use a PAT (`JIRA_TOKEN`)
> instead** — it does not rotate.

A complete GitHub Actions example, including the secret write-back step, is in
[`.github/workflows/example-oauth-ci.yml`](../.github/workflows/example-oauth-ci.yml).

## 6. Token refresh broker (confidential clients)

Some Jira DC OAuth applications are **confidential clients**: their token
endpoint **requires `client_secret` on the `grant_type=refresh_token` step**. A
published binary must never embed that secret (`strings` would reveal it), so
go-jira can route **only the refresh step** through a server-side **token refresh
broker** that holds the secret. **Login is unchanged** — it stays a direct public
PKCE flow; the broker is involved on refresh only.

```
go-jira login    ──(public PKCE, no secret)──────────────────────▶ Jira DC   [unchanged]

go-jira refresh  ──refresh_token──────────────▶ broker ──refresh_token + client_id + client_secret ─▶ Jira DC
                 ◀────── new token pair ───────        ◀───────────── rotated token pair ────────────
```

When `JIRA_TOKEN_BROKER_URL` is unset, behaviour is **identical to today** (direct
refresh). The CLI **never** holds the secret.

### Client setup

| Env var                 | Flag             | Required | Purpose                                                  |
| ----------------------- | ---------------- | -------- | -------------------------------------------------------- |
| `JIRA_TOKEN_BROKER_URL` | `--broker-url`   | to use   | Broker base URL; routes refresh through the broker       |
| `JIRA_BROKER_TOKEN`     | `--broker-token` | optional | Caller bearer token (see security model)                 |

All three refresh paths use the broker once `JIRA_TOKEN_BROKER_URL` is set:
`go-jira token refresh`, the automatic refresh after a `401`, and the `oauth-env`
(CI) initial refresh. `go-jira config show` reports `broker_url` (and a redacted
`broker_token`) with their source.

### Broker deployment (Kubernetes + Vault)

Run the **same binary** as a subcommand: `go-jira broker serve`. The secret is
injected from a **K8s Secret sourced from Vault** (Vault Agent, Secrets Store
CSI, or external-secrets) — never built into the image, never in ldflags.

| Env var                    | Required | Purpose                                                          |
| -------------------------- | -------- | --------------------------------------------------------------- |
| `JIRA_BASE_URL`            | yes      | Jira instance base URL                                          |
| `JIRA_OAUTH_CLIENT_ID`     | yes      | OAuth client ID (also matched against a request's `client_id`)  |
| `JIRA_OAUTH_CLIENT_SECRET` | yes      | Confidential client secret — **read only here, only from env**  |
| `JIRA_BROKER_TOKEN`        | optional | Required caller bearer token; enforced **only when set**        |
| `JIRA_BROKER_LISTEN`       | optional | Listen address (default `:8080`); flag `--listen`               |
| `JIRA_BROKER_TLS_CERT/KEY` | optional | Serve HTTPS directly; otherwise terminate TLS at the ingress    |

The broker **fails fast** if `JIRA_BASE_URL`, `JIRA_OAUTH_CLIENT_ID`, or
`JIRA_OAUTH_CLIENT_SECRET` is missing.

Endpoints:

- `POST /v1/refresh` — body `{"refresh_token":"…"}` (optional `client_id`,
  verified against the broker's own); returns the rotated OAuth token pair, or an
  OAuth-style error: `400 invalid_grant` (token expired/revoked), `502
  invalid_client` (the **broker's** secret is misconfigured), `401`
  (caller-token check failed), `503` (upstream timeout / 5xx).
- `GET /healthz` — liveness; never touches the secret.
- `GET /readyz` — readiness; reports not-ready (`503`) until the secret is present.

**Refresh-token rotation race.** Jira DC invalidates the old refresh token on
every successful refresh, so concurrent refreshes of the same token would race.
The broker coalesces concurrent calls for one refresh token into a **single**
upstream call (per-key request coalescing) and reuses the result for a short TTL
(default 60s). The cache is **purely in-memory, never persisted, never logged**;
the broker **does not persist tokens at rest**. With multiple replicas the cache
is per-replica, so a rare cross-replica race just makes a client retry — use
sticky routing if that matters.

### Security model

- **Primary control: the network.** Put the broker behind a **K8s NetworkPolicy
  + internal-only ingress** so only approved sources can reach it, always over
  TLS. A leaked refresh token alone is useless without network access to the
  broker — a control a CLI cannot steal.
- **Optional caller token (`JIRA_BROKER_TOKEN`).** Defence in depth, enforced
  only when set. ⚠️ It is **only meaningful when it comes from a different trust
  source than the refresh token** (e.g. a CI secret store). If it sits in the
  **same `.env` as the refresh token**, it adds **no** protection against host/
  file compromise — do not treat it as the primary control.
- **Upgrade to mTLS** (certs issued/rotated by the service mesh or ingress) when
  you need a real caller identity rather than a shared string.
- **Accepted residual risk.** Without a caller token, "reach the broker + hold a
  valid refresh token" is enough to refresh. Mitigate with network restriction,
  short-lived refresh tokens + Jira's existing rotation, and broker-side logging
  / rate limiting / anomaly alerts.
- **Rotation needs no rebuild.** Rotate `client_secret` (update Vault/Secret →
  rolling restart) or `JIRA_BROKER_TOKEN` (update the Secret + re-issue to
  callers → rolling restart) **without** rebuilding or republishing the binary.

### Observability

Each request emits a structured log with the refresh token's **sha256 prefix**
(never the token), cache hit/miss, upstream status, and `latency_ms` — **no
secret or token is ever logged**. In-process counters track `refresh_total` by
result, `upstream_calls_total`, and `cache_hits_total`.

## Building with an embedded client

To ship a binary with the company-wide client baked in:

```bash
make build JIRA_OAUTH_CLIENT_ID="$JIRA_OAUTH_CLIENT_ID"
```

The release pipeline injects the same value via goreleaser ldflags from CI.
There is no client secret — PKCE protects the authorization flow, and the client
ID is not sensitive; a Jira admin can revoke the client at any time.
