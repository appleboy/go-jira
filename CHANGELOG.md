# Changelog

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
