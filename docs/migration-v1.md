# Migrating from v0.x to v1.0

v1.0 introduces OAuth 2.0 support and one **breaking change**: go-jira now
requires a subcommand.

## The breaking change: use `go-jira run`

The bare `go-jira` command no longer performs the action. Running it with no
subcommand now prints the help page listing the available commands (the same as
`go-jira --help`).

**Everything the bare command used to do now lives under `go-jira run`.** All
action flags and the GitHub Actions `INPUT_*` environment variables behave
exactly as before — only the invocation changes.

### Before (v0.x)

```bash
go-jira --ref="ABC-123" --to-transition=Done --resolution=Fixed
```

### After (v1.0)

```bash
go-jira run --ref="ABC-123" --to-transition=Done --resolution=Fixed
```

## Updating CI/CD workflows

Add `run` after `go-jira` in any workflow step.

```diff
- go-jira \
+ go-jira run \
    --ref="${{ github.event.head_commit.message }}" \
    --to-transition=Done \
    --resolution=Fixed
```

If you invoke go-jira through environment variables only (no flags), still add
the subcommand:

```diff
- go-jira
+ go-jira run
```

The `INPUT_BASE_URL`, `INPUT_TOKEN`, `INPUT_REF`, `INPUT_TRANSITION`, etc.
variables are unchanged.

## Authentication is unchanged for existing users

Basic auth (`JIRA_USERNAME` + `JIRA_PASSWORD`) and token auth (`JIRA_TOKEN`)
work exactly as before. No action needed unless you want to adopt OAuth.

The auth selection priority in `go-jira run` is:

1. `oauth-env` — `JIRA_OAUTH_REFRESH_TOKEN` is set
2. `oauth-storage` — a token for this base URL/client exists locally
3. `bearer` — `JIRA_TOKEN` / `--token`
4. `basic` — `JIRA_USERNAME` + `JIRA_PASSWORD`

So existing PAT/basic setups keep working with the same precedence they had,
and OAuth only engages when you opt in.

## New subcommands

| Command                                | Purpose                                   |
| -------------------------------------- | ----------------------------------------- |
| `go-jira run`                          | The former bare-command action            |
| `go-jira login`                        | Interactive OAuth login (stores a token)  |
| `go-jira logout`                       | Remove the stored token for a site        |
| `go-jira whoami`                       | Show the authenticated user and auth mode |
| `go-jira token status\|refresh\|print` | Inspect/manage the stored token           |
| `go-jira config show`                  | Show resolved config and value sources    |

See [docs/oauth-usage.md](oauth-usage.md) for the OAuth setup guide.
