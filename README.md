# go-jira

[Jira][1] integration with [GitHub][2] or [Gitea Action][3] for [JIRA Data Center][4].

[1]: https://www.atlassian.com/software/jira
[2]: https://docs.github.com/en/actions
[3]: https://docs.gitea.com/usage/actions/overview
[4]: https://www.atlassian.com/enterprise/data-center/jira

## Motivation

Since there is no official Jira API integration with GitHub Action available online, and considering that Jira now has both [Cloud][5] and [Data Center][1] versions with different API implementations, this project will initially focus on the [Data Center][1] version. This will allow friends who have purchased the enterprise version to achieve automatic integration of Jira Issue status adjustments through CI/CD.

The motivation behind this project is to provide a simple way to integrate Jira with GitHub or Gitea Actions for JIRA Data Center.

[5]: https://developer.atlassian.com/cloud/jira/platform/
