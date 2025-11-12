# go-jira

[![Trivy Security Scan](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml/badge.svg)](https://github.com/appleboy/go-jira/actions/workflows/trivy.yml)

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
  - [配置说明](#配置说明)
    - [认证方式](#认证方式)
    - [环境变量](#环境变量)
    - [使用示例](#使用示例)
      - [转移问题状态并设置解决方案](#转移问题状态并设置解决方案)
      - [分配处理人并添加 Markdown 评论](#分配处理人并添加-markdown-评论)
      - [显示版本](#显示版本)
      - [使用自定义环境文件](#使用自定义环境文件)

## 动机

由于 GitHub Actions 目前没有官方的 Jira API 整合，且考虑到 Jira 同时提供 [Cloud][5] 和 [Data Center][6] 版本，两者具有不同的 API 实现方式，因此本项目将优先专注于 [Data Center][6] API 版本。这将帮助企业版用户通过 CI/CD 自动更新 Jira Issue 状态。

本项目的目标是让 Jira Data Center 能够轻松地与 GitHub 或 Gitea Actions 整合。

> **⚠️ 重要提示**：本项目目前**仅支持 Jira Data Center**。由于两个版本的 API 实现方式不同，目前**不支持 Jira Cloud**。

## 配置说明

### 认证方式

- **基本认证**：设置 `JIRA_USERNAME` 和 `JIRA_PASSWORD`
- **Token 认证**：设置 `JIRA_TOKEN`
- **跳过 SSL 验证**：设置 `JIRA_INSECURE=true`（生产环境不建议）

### 环境变量

| 变量              | 说明                                               |
|-------------------|----------------------------------------------------|
| JIRA_BASE_URL     | Jira 实例基础地址（如 `https://jira.example.com`）  |
| JIRA_USERNAME     | Jira 用户名（用于基本认证）                        |
| JIRA_PASSWORD     | Jira 密码（用于基本认证）                          |
| JIRA_TOKEN        | Jira API Token（用于 Token 认证）                  |
| JIRA_INSECURE     | 设为 `true` 跳过 SSL 证书验证                      |
| REF               | 引用字符串（如 git ref/tag/commit message）         |
| ISSUE_FORMAT      | 自定义 issue key 匹配正则（可选）                  |
| TRANSITION        | 问题要转移到的目标状态名称                         |
| RESOLUTION        | 问题解决方案名称（如 `Fixed`，可选）                |
| ASSIGNEE          | 要分配的处理人用户名（可选）                        |
| COMMENT           | 要添加到问题的评论内容（可选）                      |
| MARKDOWN          | 设为 `true` 时将评论从 Markdown 转为 Jira 格式      |
| DEBUG             | 设为 `true` 启用调试输出                            |

### 使用示例

#### 转移问题状态并设置解决方案

```bash
export JIRA_BASE_URL="https://jira.example.com"
export JIRA_TOKEN="your_api_token"
export TRANSITION="Done"
export RESOLUTION="Fixed"
export REF="refs/tags/v1.0.0"
go run cmd/go-jira/main.go
```

#### 分配处理人并添加 Markdown 评论

```bash
export ASSIGNEE="johndoe"
export COMMENT="## 问题已修复\n* 新增测试用例\n* 优化性能"
export MARKDOWN="true"
go run cmd/go-jira/main.go
```

#### 显示版本

```bash
go run cmd/go-jira/main.go -version
```

#### 使用自定义环境文件

```bash
go run cmd/go-jira/main.go -env-file=custom.env
```

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
