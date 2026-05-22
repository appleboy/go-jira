# Changelog

## v1.0.0

### ⚠️ BREAKING CHANGE

- `go-jira` now **requires a subcommand**. The previous bare-command action
  behavior moved to `go-jira run`. Update any workflow that invoked `go-jira`
  directly to call `go-jira run` instead. All action flags and `INPUT_*`
  environment variables are otherwise unchanged. See
  [docs/migration-v1.md](docs/migration-v1.md).

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
