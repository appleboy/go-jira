# go-jira

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

## 動機

由於 GitHub Actions 目前沒有官方的 Jira API 整合，且考慮到 Jira 同時提供 [Cloud][5] 和 [Data Center][6] 版本，兩者具有不同的 API 實作方式，因此本專案將優先專注於 [Data Center][6] API 版本。這將幫助企業版用戶透過 CI/CD 自動更新 Jira Issue 狀態。

本專案的目標是讓 Jira Data Center 能夠輕鬆地與 GitHub 或 Gitea Actions 整合。

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
