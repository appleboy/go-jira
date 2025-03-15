# go-jira

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

## 动机

由于 GitHub Actions 目前没有官方的 Jira API 整合，且考虑到 Jira 同时提供 [Cloud][5] 和 [Data Center][6] 版本，两者具有不同的 API 实现方式，因此本项目将优先专注于 [Data Center][6] API 版本。这将帮助企业版用户通过 CI/CD 自动更新 Jira Issue 状态。

本项目的目标是让 Jira Data Center 能够轻松地与 GitHub 或 Gitea Actions 整合。

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
