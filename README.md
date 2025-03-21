# go-jira

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

## Motivation

Since there isn't an official Jira API integration available for GitHub Actions, and considering that Jira offers both [Cloud][5] and [Data Center][6] versions with different API implementations, this project will initially focus on the [Data Center][6] API version. This will help those who have the enterprise version to automatically integrate Jira Issue status updates through CI/CD.

The goal of this project is to make it easy to integrate Jira with GitHub or Gitea Actions for Jira Data Center.

[5]: https://developer.atlassian.com/cloud/jira/platform/
[6]: https://developer.atlassian.com/server/jira/platform/
