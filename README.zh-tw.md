# go-jira

[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)

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
  - [設定說明](#設定說明)
    - [認證方式](#認證方式)
    - [環境變數](#環境變數)
    - [使用範例](#使用範例)
      - [轉移問題狀態並設定解決方案](#轉移問題狀態並設定解決方案)
      - [分配處理人並新增 Markdown 評論](#分配處理人並新增-markdown-評論)
      - [顯示版本](#顯示版本)
      - [使用自訂環境檔](#使用自訂環境檔)

## 動機

由於 GitHub Actions 目前沒有官方的 Jira API 整合，且考慮到 Jira 同時提供 [Cloud][5] 和 [Data Center][6] 版本，兩者具有不同的 API 實作方式，因此本專案將優先專注於 [Data Center][6] API 版本。這將幫助企業版用戶透過 CI/CD 自動更新 Jira Issue 狀態。

本專案的目標是讓 Jira Data Center 能夠輕鬆地與 GitHub 或 Gitea Actions 整合。

> **⚠️ 重要提醒**：本專案目前**僅支援 Jira Data Center**。由於兩個版本的 API 實作方式不同，目前**不支援 Jira Cloud**。

## 設定說明

### 認證方式

- **基本認證**：設定 `JIRA_USERNAME` 和 `JIRA_PASSWORD`
- **Token 認證**：設定 `JIRA_TOKEN`
- **跳過 SSL 驗證**：設定 `JIRA_INSECURE=true`（不建議於正式環境）

### 環境變數

| 變數              | 說明                                               |
|-------------------|----------------------------------------------------|
| JIRA_BASE_URL     | Jira 實例基礎網址（如 `https://jira.example.com`）  |
| JIRA_USERNAME     | Jira 使用者名稱（用於基本認證）                    |
| JIRA_PASSWORD     | Jira 密碼（用於基本認證）                          |
| JIRA_TOKEN        | Jira API Token（用於 Token 認證）                  |
| JIRA_INSECURE     | 設為 `true` 跳過 SSL 憑證驗證                      |
| REF               | 參考字串（如 git ref/tag/commit message）           |
| ISSUE_FORMAT      | 自訂 issue key 匹配正則（可選）                    |
| TRANSITION        | 問題要轉移到的目標狀態名稱                         |
| RESOLUTION        | 問題解決方案名稱（如 `Fixed`，可選）                |
| ASSIGNEE          | 要分配的處理人使用者名稱（可選）                    |
| COMMENT           | 要新增到問題的評論內容（可選）                      |
| MARKDOWN          | 設為 `true` 時將評論從 Markdown 轉為 Jira 格式      |
| DEBUG             | 設為 `true` 啟用除錯輸出                            |

### 使用範例

#### 轉移問題狀態並設定解決方案

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_api_token"
export TRANSITION="Done"
export RESOLUTION="Fixed"
export REF="refs/tags/v1.0.0"
go run cmd/go-jira/main.go
```

#### 分配處理人並新增 Markdown 評論

```bash
export ASSIGNEE="johndoe"
export COMMENT="## 問題已修復\n* 新增測試案例\n* 優化效能"
export MARKDOWN="true"
go run cmd/go-jira/main.go
```

#### 顯示版本

```bash
go run cmd/go-jira/main.go -version
```

#### 使用自訂環境檔

```bash
go run cmd/go-jira/main.go -env-file=custom.env
```

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
