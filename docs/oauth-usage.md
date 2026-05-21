# OAuth 2.0 usage guide

go-jira supports the **Jira Data Center** OAuth 2.0 provider. It does **not**
support Jira Cloud (a different OAuth flow on atlassian.com).

Two flows are available:

- **Authorization Code + PKCE** — interactive login for local development.
- **Refresh-token injection** — non-interactive use in CI/CD.

## 1. Register the OAuth client in Jira

A Jira administrator registers an *Application link* / OAuth 2.0 client and
configures a redirect URI. go-jira defaults to:

```
http://127.0.0.1:8765/callback
```

Override the port with `--callback-port` (the redirect URI is derived from it).
Only one redirect URI needs to be registered.

You then have a **client ID** and **client secret**. These can be:

1. embedded into the binary at build time (see [migration & build](#building-with-an-embedded-client)), or
2. supplied at runtime via `JIRA_OAUTH_CLIENT_ID` / `JIRA_OAUTH_CLIENT_SECRET`, or
3. passed as `--client-id` / `--client-secret`.

Resolution order is **env var > flag > embedded default**.

## 2. Scopes

| Scope          | Grants                                            |
|----------------|---------------------------------------------------|
| `READ`         | View projects/issues/profile                      |
| `WRITE`        | Create/update issues, comments, transitions (+READ) |
| `ADMIN`        | Admin operations (+READ, WRITE)                   |
| `SYSTEM_ADMIN` | Full system administration (+ADMIN)               |

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

| Backend  | When used                              | Notes |
|----------|----------------------------------------|-------|
| keyring  | default, when an OS keyring is available | macOS Keychain, Secret Service, Windows Credential Manager |
| file     | fallback (e.g. headless Linux without D-Bus) | AES-256-GCM, key derived with PBKDF2-HMAC-SHA256 (600k iterations) |

The file backend requires a master password via `JIRA_MASTER_PASSWORD`. Tokens
are keyed by `sha256(baseURL:clientID)`, so multiple Jira sites and clients
coexist without clobbering each other. Switch sites with `--base-url`.

## 5. CI/CD (refresh-token injection)

Set `JIRA_OAUTH_REFRESH_TOKEN` to enter `oauth-env` mode. go-jira exchanges the
refresh token for an access token at startup.

| Env var | Required | Purpose |
|---------|----------|---------|
| `JIRA_OAUTH_CLIENT_ID` | yes | OAuth client ID |
| `JIRA_OAUTH_CLIENT_SECRET` | yes | OAuth client secret |
| `JIRA_OAUTH_REFRESH_TOKEN` | yes | Triggers `oauth-env` mode |
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
make build \
  JIRA_OAUTH_CLIENT_ID="$JIRA_OAUTH_CLIENT_ID" \
  JIRA_OAUTH_CLIENT_SECRET="$JIRA_OAUTH_CLIENT_SECRET"
```

The release pipeline injects the same values via goreleaser ldflags from CI
secrets. PKCE protects the actual authorization flow, so the embedded secret is
treated as a *soft* secret; a Jira admin can revoke the client at any time.
