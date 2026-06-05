# go-jira

[![Lint and Testing](https://github.com/appleboy/go-jira/actions/workflows/testing.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/testing.yml)
[![CodeQL](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/codeql.yml)
[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)
[![Docker Image](https://github.com/appleboy/go-jira/actions/workflows/docker.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/docker.yml)

[English](./README.md) | [簡體中文](./README.zh-cn.md)

整合 [Jira][1] 與 [GitHub][2] 或 [Gitea Action][3] 用於 [JIRA Data Center][4]。

- [Integrating Gitea with Jira Software Development Workflow][01]
- [Gitea 與 Jira 軟體開發流程整合][02]

[01]: https://blog.wu-boy.com/2025/01/git-software-development-guide-key-to-improving-team-collaboration-en/
[02]: https://blog.wu-boy.com/2025/03/gitea-integrate-with-jira-issue-tracking-flow-zh-tw/
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
  - [可組合性（管線、安靜、顏色）](#可組合性管線安靜顏色)
  - [Schema 自我描述（供代理程式使用）](#schema-自我描述供代理程式使用)
  - [OAuth 2.0](#oauth-20)

## 動機

由於 GitHub Actions 目前沒有官方的 Jira API 整合，且考慮到 Jira 同時提供 [Cloud][5] 和 [Data Center][6] 版本，兩者具有不同的 API 實作方式，因此本專案將優先專注於 [Data Center][6] API 版本。這將幫助企業版用戶透過 CI/CD 自動更新 Jira Issue 狀態。

本專案的目標是讓 Jira Data Center 能夠輕鬆地與 GitHub 或 Gitea Actions 整合。

> **⚠️ 重要提醒**：本專案目前**僅支援 Jira Data Center**。由於兩個版本的 API 實作方式不同，目前**不支援 Jira Cloud**。

## 安裝

**使用安裝腳本（預編譯執行檔）** — 不需要 Go 工具鏈。此腳本會下載對應你
作業系統／架構的最新發行版執行檔，依照發行版的 `checksums.txt` 驗證其
SHA256，並安裝到 `~/.go-jira/bin`：

```bash
curl -fsSL https://raw.githubusercontent.com/appleboy/go-jira/main/install.sh | bash
```

可透過環境變數覆寫預設值，例如指定版本或變更安裝目錄：

```bash
curl -fsSL https://raw.githubusercontent.com/appleboy/go-jira/main/install.sh | VERSION=0.12.1 INSTALL_DIR=/usr/local/bin bash
```

支援平台：macOS（amd64/arm64）、Linux（amd64/arm64/armv5-7）、Windows
（amd64）以及 FreeBSD（amd64）。

查詢版本時會呼叫 GitHub API，未認證請求每個 IP 每小時上限為 60 次。在共用
NAT 環境下可能遇到 `rate limit exceeded`——可設定 `GITHUB_TOKEN` 提高上限，
或指定 `VERSION` 直接跳過查詢。

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

## 可組合性（管線、安靜、顏色）

go-jira 設計上可直接嵌入 shell 管線與 agent 工具鏈：

- **stdout 與 stderr 分離** — 結果輸出到 **stdout**，所有診斷訊息輸出到
  **stderr**，因此 `go-jira search ... > issues.json` 只會擷取 JSON。
- **`--quiet` / `-q`** — 隱藏 stderr 上的資訊性日誌（`authenticated`、
  `user account` 等行），只保留警告、錯誤與結果。為全域旗標，適用於所有子命令。
- **`--no-color` / `NO_COLOR`** — 停用 stderr 日誌的 ANSI 顏色。當 stderr 不是
  終端機時也會自動停用（遵循 [no-color.org](https://no-color.org)）。
- **`--timeout`** — 限制單一操作的執行時間，例如 `--timeout 30s` 或
  `--timeout 2m`，讓代理（agent）能設定時間預算。`0`（預設值）會使用各命令的
  預設逾時。所有與 Jira 通訊的子命令皆支援。
- **控制字元防護** — 含有控制字元（ASCII < `0x20`，但允許 tab／換行／歸位）的
  參數會在任何命令執行前被拒絕並回傳結束碼 `2`，以防止終端機跳脫序列與日誌注入。
- **stdin 輸入** — 自由文字旗標 `--ref`、`--comment`、`--description`、`--jql`
  皆可傳入 `-` 從 stdin 讀取其值：

```bash
# 將最新一筆 commit 訊息餵給 run 命令
git log -1 --format=%B | go-jira run --ref - --to-transition Done

# 將 Markdown 內容透過管線作為新 issue 的描述
cat body.md | go-jira create --project GAIA --summary "New bug" --description -

# 安靜模式，僅輸出機器可讀內容以利腳本處理
go-jira --quiet search --jql "project = GAIA" > issues.json
```

## Schema 自我描述（供代理程式使用）

`go-jira schema` 會印出完整的指令與旗標樹，讓代理程式（或腳本）不必爬梳
`--help` 就能探索整個 CLI 介面。使用 `--output json` 取得機器可讀的描述；
該 JSON 也包含建置的 `version` 與 `commit`：

```bash
# 機器可讀的指令／旗標 schema，含建置中繼資料
go-jira schema --output json

# 人類可讀、列出每個指令與其旗標的樹狀結構
go-jira schema --output text
```

這也是讀取建置 commit 的建議方式——`--version`（刻意只輸出單一 semver 字串）
已不再印出 commit。

## OAuth 2.0

go-jira 透過 Authorization Code + PKCE 流程支援 Jira Data Center 的 OAuth 2.0
provider，本機互動式登入與 CI/CD refresh token 注入皆可。子命令包括
`login` / `logout` / `whoami` / `token` / `config show`。完整設定（在 Jira
註冊 client、scope、token 儲存後端、CI/CD refresh token 輪換）見
**[docs/oauth-usage.md](docs/oauth-usage.md)**。

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
