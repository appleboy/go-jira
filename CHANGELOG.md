# Changelog

## v0.12.1 - 2026-06-05

### Fixes

- Point the auth-failure recovery hint at the token refresh and login ladder
  instead of the command that just failed, and classify an expired or revoked
  refresh token (`invalid_grant`) centrally so its hint sends users straight to
  `go-jira login`.
- Document the recovery steps in `whoami` help and `token` group help, and keep
  the OAuth recovery advice accurate for token and basic auth users.

## v0.12.0 - 2026-05-31

### Features

- **`version` subcommand.** Prints the version, commit, Go version, and
  platform, defaulting to human-readable text with an `--output json` option
  for agents.

### Fixes

- Convert ordered Markdown lists to Jira numbered lists instead of bullets, and
  emit valid Jira image markup instead of treating alt text as the filename.
- Stop rewriting email addresses as user mentions.
- Guard a nil-pointer panic when transitioning issues with missing fields, and
  error on an unknown resolution name instead of silently dropping it.
- Preserve the underlying error detail when concurrent issue operations fail,
  and stop scanning transitions after the first successful match.

### Internal

- Extract a shared goroutine/error-collection scaffold for the assignee,
  comment, and transition commands, and consolidate nil-safe issue summary and
  status accessors.

## v0.11.0 - 2026-05-26

### Features

- **One-line installer.** Added a `curl | bash` install script that downloads
  the right release binary for the host platform, with an authenticated version
  lookup and clearer messaging when the GitHub API rate limit is hit.

### Internal

- The `epics` command now lists board epics through the typed
  `Board.GetEpicsWithContext` library method instead of a hand-rolled request,
  dropping the bespoke envelope structs (bumps go-jira-lib to v1.16.4).

## v0.10.0 - 2026-05-26

### Changed

- **`--version` now prints only the semver string** (e.g. `v0.10.0`), dropping
  the `Version: ` prefix and `Commit: <sha>` suffix. This is a breaking change
  for scripts that parsed the old format: read the version token directly, and
  get the build commit from `go-jira schema` (the `commit` field).

### Features

- **Agent-friendly CLI surface.** New `schema` command exposes the full command
  and flag tree as JSON or text, every subcommand gained an Examples section,
  errors surface actionable hints (including "Did you mean" suggestions), and
  subcommands are grouped into named categories in the root help.
- **Structured errors and exit codes.** Failures exit with distinct codes per
  class (usage, auth, rate limit) and write a structured JSON error object to
  stderr, surfacing the HTTP status and `Retry-After` hint on rate limits.
- **Composable I/O.** Added a global `--quiet`/`-q` flag and `--no-color`
  (also honoring `NO_COLOR` and auto-disabling when stderr is not a terminal),
  plus `-` stdin support for `--ref`, `--comment`, `--description`, and `--jql`.
- **Time budgets.** Added a global `--timeout` flag (a persistent root flag
  inherited by every subcommand) so agents can cap how long any Jira operation
  runs, resolved through a shared per-command default.

### Internal

- Reject arguments containing control characters before any command runs while
  still allowing tab, newline, and carriage return for multi-line text flags.

## v0.9.0 - 2026-05-25

### Changed

- **Removed the OAuth client secret entirely.** go-jira is a public PKCE client,
  so no secret is needed for either interactive login or CI refresh-token
  injection (`oauth-env` mode). This is a breaking change: the
  `JIRA_OAUTH_CLIENT_SECRET` environment variable, the `--client-secret` flag,
  and the `DefaultOAuthClientSecret` build-time ldflag are gone. Drop them from
  your build, env, and CI configuration; nothing else about the OAuth flows
  changes.

## v0.8.0 - 2026-05-25

### Commands

- `go-jira` is invoked through subcommands. The Action behavior that transitions
  issues and posts comments lives under `go-jira run`; all action flags and
  `INPUT_*` environment variables are read there.

### Features

- **OAuth 2.0 support for Jira Data Center** (Authorization Code + PKCE for
  local login, refresh-token injection for CI/CD), built on
  `golang.org/x/oauth2`.
- New subcommands: `run`, `login`, `logout`, `whoami`,
  `token print|status|refresh`, and `config show`.
- Local token storage in the OS keyring, with an AES-256-GCM encrypted-file
  fallback (PBKDF2-HMAC-SHA256, 600k iterations).
- Automatic access-token refresh with rotation write-back, plus a single forced
  refresh + retry on a 401.
- Build-time embedding of a company-wide OAuth client via `-ldflags`
  (`DefaultOAuthClientID` / `DefaultOAuthClientSecret`); resolution order is
  env var > flag > embedded default.

### Internal

- Authentication refactored behind an `auth.Authenticator` strategy interface
  (`pkg/auth`) with `oauth-env > oauth-storage > bearer > basic` resolution.
- New packages: `pkg/auth`, `pkg/oauth`, `pkg/storage`.
