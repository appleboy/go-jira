# go-jira

[![Lint and Testing](https://github.com/appleboy/go-jira/actions/workflows/testing.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/testing.yml)
[![CodeQL](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml)
[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)
[![Docker Image](https://github.com/appleboy/go-jira/actions/workflows/docker.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/docker.yml)

[繁體中文](./README.zh-tw.md) | [簡體中文](./README.zh-cn.md)

[Jira][1] integration with [GitHub][2] or [Gitea Action][3] for [JIRA Data Center][4].

- [Integrating Gitea with Jira Software Development Workflow][01]
- [Gitea 與 Jira 軟體開發流程整合][02]

[01]: https://blog.wu-boy.com/2025/03/gitea-jira-integration-en/
[02]: https://blog.wu-boy.com/2025/03/gitea-jira-integration-zh-tw/
[1]: https://www.atlassian.com/software/jira
[2]: https://docs.github.com/en/actions
[3]: https://docs.gitea.com/usage/actions/overview
[4]: https://www.atlassian.com/enterprise/data-center/jira

## Table of Contents

- [go-jira](#go-jira)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Configuration](#configuration)
    - [Authentication](#authentication)
    - [Environment Variables](#environment-variables)
    - [Usage](#usage)
      - [Transition issue status and set resolution](#transition-issue-status-and-set-resolution)
      - [Assign issue and add Markdown comment](#assign-issue-and-add-markdown-comment)
      - [Log in with OAuth (local development)](#log-in-with-oauth-local-development)
      - [Show version](#show-version)
      - [Use custom environment file](#use-custom-environment-file)
  - [Data subcommands](#data-subcommands)
  - [OAuth 2.0](#oauth-20)

## Motivation

Since there isn't an official Jira API integration available for GitHub Actions, and considering that Jira offers both [Cloud][5] and [Data Center][6] versions with different API implementations, this project will initially focus on the [Data Center][6] API version. This will help those who have the enterprise version to automatically integrate Jira Issue status updates through CI/CD.

The goal of this project is to make it easy to integrate Jira with GitHub or Gitea Actions for Jira Data Center.

> **⚠️ Important Note**: This project currently **only supports Jira Data Center**. Jira Cloud is **not supported** at this time due to different API implementations between the two versions.

## Configuration

> **⚠️ Breaking change in v1.0**: `go-jira` now requires a subcommand. The
> previous bare-command behavior moved to `go-jira run`. See the
> [migration guide](docs/migration-v1.md).

### Authentication

go-jira supports four authentication modes:

| Mode              | Best for                          | How to configure                               |
| ----------------- | --------------------------------- | ---------------------------------------------- |
| **Basic Auth**    | Legacy Jira or dev/test           | `JIRA_USERNAME` + `JIRA_PASSWORD`              |
| **Bearer / PAT**  | Recommended CI/CD default         | `JIRA_TOKEN` (a Personal Access Token)         |
| **OAuth (local)** | Interactive developer login       | `go-jira login`                                |
| **OAuth (CI/CD)** | Fine-grained scopes in automation | `JIRA_OAUTH_REFRESH_TOKEN` + rotation handling |

- **Skip SSL Verification**: Set `JIRA_INSECURE=true` (not recommended for production)

> **OAuth in CI/CD is more work than a PAT.** Jira DC rotates the refresh token
> on every refresh, so a CI run must write the new token back to its secret
> store. If you can't automate that, prefer a Personal Access Token
> (`JIRA_TOKEN`). See [docs/oauth-usage.md](docs/oauth-usage.md) for the full
> OAuth guide.

### Environment Variables

| Variable                        | Description                                                                |
| ------------------------------- | -------------------------------------------------------------------------- |
| JIRA_BASE_URL                   | Jira instance base URL (e.g. `https://jira.example.com`)                   |
| JIRA_USERNAME                   | Jira username (for basic auth)                                             |
| JIRA_PASSWORD                   | Jira password (for basic auth)                                             |
| JIRA_TOKEN                      | Jira API token (for token auth)                                            |
| JIRA_INSECURE                   | Set to `true` to skip SSL certificate verification                         |
| REF                             | Reference string (e.g. git ref/tag/commit message)                         |
| ISSUE_FORMAT                    | Custom regex for issue key matching (optional)                             |
| TRANSITION                      | Target status name for issue transition                                    |
| RESOLUTION                      | Resolution name (e.g. `Fixed`, optional)                                   |
| ASSIGNEE                        | Username to assign the issue to (optional)                                 |
| COMMENT                         | Comment to add to the issue (optional)                                     |
| MARKDOWN                        | Set to `true` to convert comment from Markdown to Jira format              |
| DEBUG                           | Set to `true` to enable debug output                                       |
| OUTPUT                          | Output format for the data subcommands: `json` (default) or `text`         |
| EPIC_FIELD                      | Epic Link custom field ID used by `create`/`update`/`search` (default `customfield_10101`) |
| SPRINT_FIELD                    | Sprint custom field ID used by `create`/`update`/`search` (default `customfield_10100`)    |
| JIRA_OAUTH_CLIENT_ID            | OAuth client ID (overrides the embedded default)                           |
| JIRA_OAUTH_CLIENT_SECRET        | OAuth client secret (overrides the embedded default)                       |
| JIRA_OAUTH_REFRESH_TOKEN        | Injected refresh token; triggers CI `oauth-env` mode                       |
| JIRA_OAUTH_REFRESH_TOKEN_OUTPUT | File path to write the rotated refresh token                               |
| JIRA_OAUTH_CALLBACK_PORT        | Local OAuth callback port (default `8765`)                                 |
| JIRA_OAUTH_CALLBACK_CERT        | TLS cert file for an HTTPS login callback (with `JIRA_OAUTH_CALLBACK_KEY`) |
| JIRA_OAUTH_CALLBACK_KEY         | TLS key file for an HTTPS login callback (with `JIRA_OAUTH_CALLBACK_CERT`) |
| JIRA_MASTER_PASSWORD            | Master password for the encrypted file token store (when no keyring)       |

### Usage

> As of v1.0 the action runs under the `run` subcommand. Replace any previous
> bare `go-jira` invocation with `go-jira run`.

#### Transition issue status and set resolution

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_api_token"
export TRANSITION="Done"
export RESOLUTION="Fixed"
export REF="refs/tags/v1.0.0"
go run ./cmd/go-jira run
```

#### Assign issue and add Markdown comment

```bash
export ASSIGNEE="johndoe"
export COMMENT="## Issue fixed\n* Added tests\n* Improved performance"
export MARKDOWN="true"
go run ./cmd/go-jira run
```

#### Log in with OAuth (local development)

```bash
export JIRA_BASE_URL="https://jira.example.com"
go run ./cmd/go-jira login --client-id="$JIRA_OAUTH_CLIENT_ID"
# then run normally — the stored token is used automatically:
go run ./cmd/go-jira run --ref="ABC-123" --to-transition=Done
```

#### Show version

```bash
go run ./cmd/go-jira --version
```

#### Use custom environment file

```bash
go run ./cmd/go-jira run --env-file=custom.env
```

## Data subcommands

Beyond `run`, go-jira exposes a set of issue/board subcommands for scripting and
automation. They share the same authentication (OAuth / Bearer / Basic), base
URL, and `.env` resolution as every other command, and print machine-readable
JSON to stdout by default. Pass `--output text` for a concise human-readable
summary; errors go to stderr with a non-zero exit code.

| Command   | Purpose                                  | Key flags                                                                            |
| --------- | ---------------------------------------- | ------------------------------------------------------------------------------------ |
| `search`  | Run a JQL query                          | `--jql` (required), `--fields`, `--limit`                                            |
| `get`     | Fetch summary + status of one issue      | `--key` (required)                                                                   |
| `create`  | Create a Task issue                      | `--project`, `--summary` (required), `--assignee`, `--description`, `--components`, `--labels`, `--epic`, `--sprint` |
| `update`  | Partially update an issue's fields       | `--key` (required) + any of `--summary`, `--description`, `--assignee`, `--components`, `--labels`, `--epic`, `--sprint` |
| `sprints` | List sprints for a board (Agile API)     | `--board-id` (required), `--state`, `--limit`                                        |
| `boards`  | Discover boards for a project (Agile API)| `--project` (required), `--type`, `--limit`                                          |
| `link`    | Link two issues                          | `--from`, `--to` (required), `--link-type`                                           |

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_personal_access_token"

# Search (JSON to stdout)
go run ./cmd/go-jira search --jql "project = GAIA AND status = Open" --limit 10

# Human-readable summary
go run ./cmd/go-jira get --key GAIA-123 --output text

# Create a Task, attaching it to an epic and a sprint
go run ./cmd/go-jira create --project GAIA --summary "Investigate flaky test" \
  --epic GAIA-42 --sprint 55 --labels ci,flaky

# Partial update — only the flags you pass are changed
go run ./cmd/go-jira update --key GAIA-123 \
  --summary "Reworded title" --assignee jdoe --labels triaged

# Link two issues
go run ./cmd/go-jira link --from GAIA-1 --to GAIA-2 --link-type Blocks
```

The epic-link and sprint custom field IDs vary per Jira instance. They default
to `customfield_10101` / `customfield_10100`; override them with
`--epic-field` / `--sprint-field` (or `EPIC_FIELD` / `SPRINT_FIELD`).

## OAuth 2.0

go-jira supports the Jira Data Center OAuth 2.0 provider via the Authorization
Code + PKCE flow for local use and refresh-token injection for CI/CD.

Subcommands:

- `go-jira login` — interactive browser login; stores the token in your OS
  keyring (or an AES-256-GCM encrypted file when no keyring is available).
- `go-jira logout` — remove the stored token for a site.
- `go-jira whoami` — show the authenticated user and active auth mode.
- `go-jira token status|refresh|print` — inspect or refresh the stored token.
- `go-jira config show` — show resolved config and where each value came from.

See **[docs/oauth-usage.md](docs/oauth-usage.md)** for setup (registering the
client in Jira, scopes, storage backends) and
**[docs/migration-v1.md](docs/migration-v1.md)** for upgrading from v0.x.

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
