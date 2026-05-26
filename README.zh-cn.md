# go-jira

[![Lint and Testing](https://github.com/appleboy/go-jira/actions/workflows/testing.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/testing.yml)
[![CodeQL](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml)
[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)
[![Docker Image](https://github.com/appleboy/go-jira/actions/workflows/docker.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/docker.yml)

[English](./README.md) | [繁體中文](./README.zh-tw.md)

整合 [Jira][1] 与 [GitHub][2] 或 [Gitea Action][3] 用于 [JIRA Data Center][4]。

- [Integrating Gitea with Jira Software Development Workflow][01]
- [Gitea 与 Jira 软件开发流程整合][02]

[01]: https://blog.wu-boy.com/2025/03/gitea-jira-integration-en/
[02]: https://blog.wu-boy.com/2025/03/gitea-jira-integration-zh-tw/
[1]: https://www.atlassian.com/software/jira
[2]: https://docs.github.com/en/actions
[3]: https://docs.gitea.com/usage/actions/overview
[4]: https://www.atlassian.com/enterprise/data-center/jira

## 目录

- [go-jira](#go-jira)
  - [目录](#目录)
  - [动机](#动机)
  - [安装](#安装)
  - [配置说明](#配置说明)
    - [认证方式](#认证方式)
    - [环境变量](#环境变量)
    - [使用示例](#使用示例)
      - [转移问题状态并设置解决方案](#转移问题状态并设置解决方案)
      - [分配处理人并添加 Markdown 评论](#分配处理人并添加-markdown-评论)
      - [使用 OAuth 登录（本地开发）](#使用-oauth-登录本地开发)
      - [显示版本](#显示版本)
      - [使用自定义环境文件](#使用自定义环境文件)
  - [可组合性（管道、安静、颜色）](#可组合性管道安静颜色)
  - [OAuth 2.0](#oauth-20)

## 动机

由于 GitHub Actions 目前没有官方的 Jira API 整合，且考虑到 Jira 同时提供 [Cloud][5] 和 [Data Center][6] 版本，两者具有不同的 API 实现方式，因此本项目将优先专注于 [Data Center][6] API 版本。这将帮助企业版用户通过 CI/CD 自动更新 Jira Issue 状态。

本项目的目标是让 Jira Data Center 能够轻松地与 GitHub 或 Gitea Actions 整合。

> **⚠️ 重要提示**：本项目目前**仅支持 Jira Data Center**。由于两个版本的 API 实现方式不同，目前**不支持 Jira Cloud**。

## 安装

**使用安装脚本（预编译可执行文件）** — 无需 Go 工具链。此脚本会下载对应你
操作系统／架构的最新发行版可执行文件，依照发行版的 `checksums.txt` 校验其
SHA256，并安装到 `~/.go-jira/bin`：

```bash
curl -fsSL https://raw.githubusercontent.com/appleboy/go-jira/main/install.sh | bash
```

可通过环境变量覆盖默认值，例如指定版本或更改安装目录：

```bash
curl -fsSL https://raw.githubusercontent.com/appleboy/go-jira/main/install.sh | VERSION=0.10.0 INSTALL_DIR=/usr/local/bin bash
```

支持平台：macOS（amd64/arm64）、Linux（amd64/arm64/armv5-7）、Windows
（amd64）以及 FreeBSD（amd64）。

查询版本时会调用 GitHub API，未认证请求每个 IP 每小时上限为 60 次。在共享
NAT 环境下可能遇到 `rate limit exceeded`——可设置 `GITHUB_TOKEN` 提高上限，
或指定 `VERSION` 直接跳过查询。

**使用 `go install` 安装**（需 Go 1.25 以上）。可执行文件会放到
`$(go env GOPATH)/bin`，请确认该目录已加入 `PATH`：

```bash
go install github.com/appleboy/go-jira/cmd/go-jira@latest
```

随后即可直接运行：

```bash
go-jira --version
```

**从源码构建：**

```bash
git clone https://github.com/appleboy/go-jira.git
cd go-jira
go install ./cmd/go-jira
```

**使用 Docker 运行** — 已发布镜像位于 `ghcr.io/appleboy/go-jira`，无需安装本地 Go 环境：

```bash
docker run --rm ghcr.io/appleboy/go-jira:latest --version
```

> **备注**：下方使用示例为了方便从源码操作，均以 `go run ./cmd/go-jira`
> 调用工具。若你已通过 `go install` 安装可执行文件，请将示例中的
> `go run ./cmd/go-jira` 替换为 `go-jira`。

## 配置说明

> **说明**：`go-jira` 通过子命令操作。转移 issue 状态与发布评论的 Action 行为
> 位于 `go-jira run`，详见下方 [使用示例](#使用示例)。

### 认证方式

go-jira 支持四种认证模式：

| 模式               | 适用场景                  | 配置方式                              |
| ------------------ | ------------------------- | ------------------------------------- |
| **基本认证**       | 旧版 Jira 或开发/测试     | `JIRA_USERNAME` + `JIRA_PASSWORD`     |
| **Bearer / PAT**   | 推荐的 CI/CD 默认         | `JIRA_TOKEN`（个人访问令牌）          |
| **OAuth（本地）**  | 开发者交互式登录          | `go-jira login`                       |
| **OAuth（CI/CD）** | 需要细粒度 scope 的自动化 | `JIRA_OAUTH_REFRESH_TOKEN` + 轮换处理 |

- **跳过 SSL 验证**：设置 `JIRA_INSECURE=true`（生产环境不建议）

> **OAuth 在 CI/CD 比 PAT 麻烦。** Jira DC 每次 refresh 都会轮换 refresh token，
> CI 必须把新 token 写回 secret 存储。若无法自动化，建议改用 PAT（`JIRA_TOKEN`）。
> 完整说明见 [docs/oauth-usage.md](docs/oauth-usage.md)。

### 环境变量

| 变量                            | 说明                                               |
| ------------------------------- | -------------------------------------------------- |
| JIRA_BASE_URL                   | Jira 实例基础地址（如 `https://jira.example.com`） |
| JIRA_USERNAME                   | Jira 用户名（用于基本认证）                        |
| JIRA_PASSWORD                   | Jira 密码（用于基本认证）                          |
| JIRA_TOKEN                      | Jira API Token（用于 Token 认证）                  |
| JIRA_INSECURE                   | 设为 `true` 跳过 SSL 证书验证                      |
| REF                             | 引用字符串（如 git ref/tag/commit message）        |
| ISSUE_FORMAT                    | 自定义 issue key 匹配正则（可选）                  |
| TRANSITION                      | 问题要转移到的目标状态名称                         |
| RESOLUTION                      | 问题解决方案名称（如 `Fixed`，可选）               |
| ASSIGNEE                        | 要分配的处理人用户名（可选）                       |
| COMMENT                         | 要添加到问题的评论内容（可选）                     |
| MARKDOWN                        | 设为 `true` 时将评论从 Markdown 转为 Jira 格式     |
| DEBUG                           | 设为 `true` 启用调试输出                           |
| JIRA_OAUTH_CLIENT_ID            | OAuth client ID（覆盖内嵌默认值）                  |
| JIRA_OAUTH_REFRESH_TOKEN        | 注入的 refresh token；触发 CI `oauth-env` 模式     |
| JIRA_OAUTH_REFRESH_TOKEN_OUTPUT | 写入轮换后 refresh token 的文件路径                |
| JIRA_MASTER_PASSWORD            | 加密文件 token 存储的主密码（无 keyring 时）       |

### 使用示例

> Action 行为在 `run` 子命令下执行。所有动作标志与 GitHub Actions 的 `INPUT_*`
> 环境变量均由 `go-jira run` 读取。

#### 转移问题状态并设置解决方案

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_api_token"
export TRANSITION="Done"
export RESOLUTION="Fixed"
export REF="refs/tags/v1.0.0"
go run ./cmd/go-jira run
```

#### 分配处理人并添加 Markdown 评论

```bash
export ASSIGNEE="johndoe"
export COMMENT="## 问题已修复\n* 新增测试用例\n* 优化性能"
export MARKDOWN="true"
go run ./cmd/go-jira run
```

#### 使用 OAuth 登录（本地开发）

```bash
export JIRA_BASE_URL="https://jira.example.com"
go run ./cmd/go-jira login --client-id="$JIRA_OAUTH_CLIENT_ID"
# 之后正常执行，会自动使用已存储的 token：
go run ./cmd/go-jira run --ref="ABC-123" --to-transition=Done
```

#### 显示版本

```bash
go run ./cmd/go-jira --version
```

#### 使用自定义环境文件

```bash
go run ./cmd/go-jira run --env-file=custom.env
```

## 可组合性（管道、安静、颜色）

go-jira 设计上可直接嵌入 shell 管道与 agent 工具链：

- **stdout 与 stderr 分离** — 结果输出到 **stdout**，所有诊断信息输出到
  **stderr**，因此 `go-jira search ... > issues.json` 只会捕获 JSON。
- **`--quiet` / `-q`** — 隐藏 stderr 上的信息性日志（`authenticated`、
  `user account` 等行），只保留警告、错误与结果。为全局标志，适用于所有子命令。
- **`--no-color` / `NO_COLOR`** — 禁用 stderr 日志的 ANSI 颜色。当 stderr 不是
  终端时也会自动禁用（遵循 [no-color.org](https://no-color.org)）。
- **`--timeout`** — 限制单次操作的执行时间，例如 `--timeout 30s` 或
  `--timeout 2m`，让代理（agent）能设定时间预算。`0`（默认值）会使用各命令的
  默认超时。所有与 Jira 通信的子命令均支持。
- **控制字符防护** — 含有控制字符（ASCII < `0x20`，但允许 tab／换行／回车）的
  参数会在任何命令执行前被拒绝并返回退出码 `2`，以防止终端转义序列与日志注入。
- **stdin 输入** — 自由文本标志 `--ref`、`--comment`、`--description`、`--jql`
  均可传入 `-` 从 stdin 读取其值：

```bash
# 将最新一条 commit 消息喂给 run 命令
git log -1 --format=%B | go-jira run --ref - --to-transition Done

# 将 Markdown 内容通过管道作为新 issue 的描述
cat body.md | go-jira create --project GAIA --summary "New bug" --description -

# 安静模式，仅输出机器可读内容以便脚本处理
go-jira --quiet search --jql "project = GAIA" > issues.json
```

## OAuth 2.0

go-jira 通过 Authorization Code + PKCE 流程支持 Jira Data Center 的 OAuth 2.0
provider，支持本地交互式登录与 CI/CD refresh token 注入。子命令包括
`login` / `logout` / `whoami` / `token` / `config show`。完整配置（在 Jira
注册 client、scope、token 存储后端、CI/CD refresh token 轮换）见
**[docs/oauth-usage.md](docs/oauth-usage.md)**。

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
