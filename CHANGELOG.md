# Changelog

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
