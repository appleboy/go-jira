# go-jira

[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)

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
      - [Show version](#show-version)
      - [Use custom environment file](#use-custom-environment-file)

## Motivation

Since there isn't an official Jira API integration available for GitHub Actions, and considering that Jira offers both [Cloud][5] and [Data Center][6] versions with different API implementations, this project will initially focus on the [Data Center][6] API version. This will help those who have the enterprise version to automatically integrate Jira Issue status updates through CI/CD.

The goal of this project is to make it easy to integrate Jira with GitHub or Gitea Actions for Jira Data Center.

## Configuration

### Authentication

- **Basic Auth**: Set `JIRA_USERNAME` and `JIRA_PASSWORD`
- **Token Auth**: Set `JIRA_TOKEN`
- **Skip SSL Verification**: Set `JIRA_INSECURE=true` (not recommended for production)

### Environment Variables

| Variable         | Description                                      |
|------------------|--------------------------------------------------|
| JIRA_BASE_URL    | Jira instance base URL (e.g. `https://jira.example.com`) |
| JIRA_USERNAME    | Jira username (for basic auth)                   |
| JIRA_PASSWORD    | Jira password (for basic auth)                   |
| JIRA_TOKEN       | Jira API token (for token auth)                  |
| JIRA_INSECURE    | Set to `true` to skip SSL certificate verification |
| REF              | Reference string (e.g. git ref/tag/commit message) |
| ISSUE_FORMAT     | Custom regex for issue key matching (optional)   |
| TRANSITION       | Target status name for issue transition          |
| RESOLUTION       | Resolution name (e.g. `Fixed`, optional)         |
| ASSIGNEE         | Username to assign the issue to (optional)       |
| COMMENT          | Comment to add to the issue (optional)           |
| MARKDOWN         | Set to `true` to convert comment from Markdown to Jira format |
| DEBUG            | Set to `true` to enable debug output             |

### Usage

#### Transition issue status and set resolution

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_api_token"
export TRANSITION="Done"
export RESOLUTION="Fixed"
export REF="refs/tags/v1.0.0"
go run cmd/go-jira/main.go
```

#### Assign issue and add Markdown comment

```bash
export ASSIGNEE="johndoe"
export COMMENT="## Issue fixed\n* Added tests\n* Improved performance"
export MARKDOWN="true"
go run cmd/go-jira/main.go
```

#### Show version

```bash
go run cmd/go-jira/main.go -version
```

#### Use custom environment file

```bash
go run cmd/go-jira/main.go -env-file=custom.env
```

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
