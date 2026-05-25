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

## Building with an embedded client

To ship a binary with the company-wide client baked in:

```bash
make build JIRA_OAUTH_CLIENT_ID="$JIRA_OAUTH_CLIENT_ID"
```

The release pipeline injects the same value via goreleaser ldflags from CI.
There is no client secret — PKCE protects the authorization flow, and the client
ID is not sensitive; a Jira admin can revoke the client at any time.
