# go-jira

[![Lint and Testing](https://github.com/appleboy/go-jira/actions/workflows/testing.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/testing.yml)
[![CodeQL](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml)
[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)
[![Docker Image](https://github.com/appleboy/go-jira/actions/workflows/docker.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/docker.yml)

[English](./README.md) | [簡體中文](./README.zh-cn.md)

整合 [Jira][1] 與 [GitHub][2] 或 [Gitea Action][3] 用於 [JIRA Data Center][4]。

- [Integrating Gitea with Jira Software Development Workflow][01]
- [Gitea 與 Jira 軟體開發流程整合][02]

[01]: https://blog.wu-boy.com/2025/03/gitea-jira-integration-en/
[02]: https://blog.wu-boy.com/2025/03/gitea-jira-integration-zh-tw/
[1]: https://www.atlassian.com/software/jira
[2]: https://docs.github.com/en/actions
[3]: https://docs.gitea.com/usage/actions/overview
[4]: https://www.atlassian.com/enterprise/data-center/jira

## 目錄

- [go-jira](#go-jira)
  - [目錄](#目錄)
  - [動機](#動機)
  - [安裝](#安裝)
  - [設定說明](#設定說明)
    - [認證方式](#認證方式)
    - [環境變數](#環境變數)
    - [使用範例](#使用範例)
      - [轉移問題狀態並設定解決方案](#轉移問題狀態並設定解決方案)
      - [分配處理人並新增 Markdown 評論](#分配處理人並新增-markdown-評論)
      - [使用 OAuth 登入（本機開發）](#使用-oauth-登入本機開發)
      - [顯示版本](#顯示版本)
      - [使用自訂環境檔](#使用自訂環境檔)
  - [OAuth 2.0](#oauth-20)

## 動機

由於 GitHub Actions 目前沒有官方的 Jira API 整合，且考慮到 Jira 同時提供 [Cloud][5] 和 [Data Center][6] 版本，兩者具有不同的 API 實作方式，因此本專案將優先專注於 [Data Center][6] API 版本。這將幫助企業版用戶透過 CI/CD 自動更新 Jira Issue 狀態。

本專案的目標是讓 Jira Data Center 能夠輕鬆地與 GitHub 或 Gitea Actions 整合。

> **⚠️ 重要提醒**：本專案目前**僅支援 Jira Data Center**。由於兩個版本的 API 實作方式不同，目前**不支援 Jira Cloud**。

## 安裝

**使用 `go install` 安裝**（需 Go 1.25 以上）。執行檔會放到
`$(go env GOPATH)/bin`，請確認該目錄已加入 `PATH`：

```bash
go install github.com/appleboy/go-jira/cmd/go-jira@latest
```

接著即可直接執行：

```bash
go-jira --version
```

**從原始碼建置：**

```bash
git clone https://github.com/appleboy/go-jira.git
cd go-jira
go install ./cmd/go-jira
```

**使用 Docker 執行** — 已發佈映像檔位於 `ghcr.io/appleboy/go-jira`，無需安裝本機 Go 環境：

```bash
docker run --rm ghcr.io/appleboy/go-jira:latest --version
```

> **備註**：下方使用範例為了方便從原始碼操作，皆以 `go run ./cmd/go-jira`
> 呼叫工具。若你已透過 `go install` 安裝執行檔，請將範例中的
> `go run ./cmd/go-jira` 改為 `go-jira`。

## 設定說明

> **說明**：`go-jira` 透過子命令操作。轉移 issue 狀態與張貼評論的 Action 行為
> 位於 `go-jira run`，詳見下方 [使用範例](#使用範例)。

### 認證方式

go-jira 支援四種認證模式：

| 模式               | 適用場景                  | 設定方式                              |
| ------------------ | ------------------------- | ------------------------------------- |
| **基本認證**       | 舊版 Jira 或開發/測試     | `JIRA_USERNAME` + `JIRA_PASSWORD`     |
| **Bearer / PAT**   | 建議的 CI/CD 預設         | `JIRA_TOKEN`（個人存取權杖）          |
| **OAuth（本機）**  | 開發者互動式登入          | `go-jira login`                       |
| **OAuth（CI/CD）** | 需要細粒度 scope 的自動化 | `JIRA_OAUTH_REFRESH_TOKEN` + 輪換處理 |

- **跳過 SSL 驗證**：設定 `JIRA_INSECURE=true`（不建議於正式環境）

> **OAuth 在 CI/CD 比 PAT 麻煩。** Jira DC 每次 refresh 都會輪換 refresh token，
> CI 必須把新 token 寫回 secret 儲存。若無法自動化，建議改用 PAT（`JIRA_TOKEN`）。
> 完整說明見 [docs/oauth-usage.md](docs/oauth-usage.md)。

### 環境變數

| 變數                            | 說明                                               |
| ------------------------------- | -------------------------------------------------- |
| JIRA_BASE_URL                   | Jira 實例基礎網址（如 `https://jira.example.com`） |
| JIRA_USERNAME                   | Jira 使用者名稱（用於基本認證）                    |
| JIRA_PASSWORD                   | Jira 密碼（用於基本認證）                          |
| JIRA_TOKEN                      | Jira API Token（用於 Token 認證）                  |
| JIRA_INSECURE                   | 設為 `true` 跳過 SSL 憑證驗證                      |
| REF                             | 參考字串（如 git ref/tag/commit message）          |
| ISSUE_FORMAT                    | 自訂 issue key 匹配正則（可選）                    |
| TRANSITION                      | 問題要轉移到的目標狀態名稱                         |
| RESOLUTION                      | 問題解決方案名稱（如 `Fixed`，可選）               |
| ASSIGNEE                        | 要分配的處理人使用者名稱（可選）                   |
| COMMENT                         | 要新增到問題的評論內容（可選）                     |
| MARKDOWN                        | 設為 `true` 時將評論從 Markdown 轉為 Jira 格式     |
| DEBUG                           | 設為 `true` 啟用除錯輸出                           |
| JIRA_OAUTH_CLIENT_ID            | OAuth client ID（覆寫內嵌預設值）                  |
| JIRA_OAUTH_REFRESH_TOKEN        | 注入的 refresh token；觸發 CI `oauth-env` 模式     |
| JIRA_OAUTH_REFRESH_TOKEN_OUTPUT | 寫入輪換後 refresh token 的檔案路徑                |
| JIRA_MASTER_PASSWORD            | 加密檔 token 儲存的主密碼（無 keyring 時）         |

### 使用範例

> Action 行為在 `run` 子命令下執行。所有動作旗標與 GitHub Actions 的 `INPUT_*`
> 環境變數皆由 `go-jira run` 讀取。

#### 轉移問題狀態並設定解決方案

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_api_token"
export TRANSITION="Done"
export RESOLUTION="Fixed"
export REF="refs/tags/v1.0.0"
go run ./cmd/go-jira run
```

#### 分配處理人並新增 Markdown 評論

```bash
export ASSIGNEE="johndoe"
export COMMENT="## 問題已修復\n* 新增測試案例\n* 優化效能"
export MARKDOWN="true"
go run ./cmd/go-jira run
```

#### 使用 OAuth 登入（本機開發）

```bash
export JIRA_BASE_URL="https://jira.example.com"
go run ./cmd/go-jira login --client-id="$JIRA_OAUTH_CLIENT_ID"
# 之後正常執行，會自動使用已儲存的 token：
go run ./cmd/go-jira run --ref="ABC-123" --to-transition=Done
```

#### 顯示版本

```bash
go run ./cmd/go-jira --version
```

#### 使用自訂環境檔

```bash
go run ./cmd/go-jira run --env-file=custom.env
```

## OAuth 2.0

go-jira 透過 Authorization Code + PKCE 流程支援 Jira Data Center 的 OAuth 2.0
provider，本機互動式登入與 CI/CD refresh token 注入皆可。子命令包括
`login` / `logout` / `whoami` / `token` / `config show`。完整設定（在 Jira
註冊 client、scope、token 儲存後端、CI/CD refresh token 輪換）見
**[docs/oauth-usage.md](docs/oauth-usage.md)**。

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
