# Kubernetes Deployment

CCC can run the Jira ticket watcher in Kubernetes with the bundled Helm chart.
The watcher polls Jira, claims eligible tickets, resolves the configured repo,
and starts detached CCC sessions from inside the pod.

## Prerequisites

- A container image built from this repo.
- A Telegram bot token and group configured for CCC topics.
- Jira API credentials.
- A provider runtime available in the image or mounted into the pod. The base
  image includes CCC, tmux, git, and SSH tools; extend it if your sessions
  need a specific Claude Code or Codex CLI installation.

## Install

Create a values file:

~~~yaml
image:
  repository: ghcr.io/tuannvm/ccc
  tag: "1.7.0"

ccc:
  core:
    botToken: "123456:telegram-token"
    chatId: 123456789
    groupId: -1001234567890
    projectsDir: /home/ccc/Projects
  providers:
    activeProvider: codex

jira:
  configFromEnv: true
  baseUrl: https://example.atlassian.net
  authMethod: basic
  authToken: "jira-api-token"
  authEmail: "user@example.com"
  jql: 'project = ABC AND status = "Ready for Dev" ORDER BY priority DESC'
  claimStatus: In Progress
  repoField: customfield_10001
  acceptanceCriteriaField: customfield_10002
  pollInterval: 1m
  maxTicketsPerCycle: 1
~~~

Install the chart:

~~~bash
helm upgrade --install ccc-jira-watcher ./charts/ccc-jira-watcher -f values.yaml
~~~

Check logs:

~~~bash
kubectl logs deploy/ccc-jira-watcher -f
~~~

## Configuration Modes

The chart supports both environment-variable configuration and values-rendered
config files.

### Environment Variables

Set jira.configFromEnv: true to configure the watcher through environment
variables. The chart renders these from values.yaml:

| Value | Environment variable |
| --- | --- |
| jira.baseUrl | CCC_JIRA_BASE_URL |
| jira.authMethod | CCC_JIRA_AUTH_METHOD |
| jira.authToken or jira.existingSecret | CCC_JIRA_AUTH_TOKEN |
| jira.authEmail or jira.existingSecret | CCC_JIRA_AUTH_EMAIL |
| jira.jql | CCC_JIRA_JQL |
| jira.claimTransition | CCC_JIRA_CLAIM_TRANSITION |
| jira.claimStatus | CCC_JIRA_CLAIM_STATUS |
| jira.repoField | CCC_JIRA_REPO_FIELD |
| jira.repoFallback | CCC_JIRA_REPO_FALLBACK |
| jira.acceptanceCriteriaField | CCC_JIRA_ACCEPTANCE_CRITERIA_FIELD |
| jira.pollInterval | CCC_JIRA_POLL_INTERVAL |
| jira.maxTicketsPerCycle | CCC_JIRA_MAX_TICKETS_PER_CYCLE |

For production, store credentials in an existing Secret:

~~~yaml
jira:
  existingSecret: ccc-jira-credentials
  authTokenKey: token
  authEmailKey: email
~~~

### Helm-Rendered Config Files

Set jira.configFromValues: true to render jira.json into the seeded CCC config
Secret. CCC core config is always rendered unless config.existingSecret is set.

Use config.existingSecret when you want to manage all config files yourself.
The Secret should contain any of these keys:

- config.core.json
- config.sessions.json
- config.providers.json
- jira.json

The chart copies these files into a writable PVC before the watcher starts so
CCC can update sessions and watcher state at runtime. On upgrades,
config.core.json, config.providers.json, and jira.json are refreshed from the
Secret; config.sessions.json is only seeded when missing so active session state
is not overwritten.

When `jira.configFromValues` is enabled without `jira.configFromEnv`, the chart still exports
`CCC_JIRA_REPO_FALLBACK` when `jira.repoFallback` is set, because the rendered `jira.json`
references that value through `repo_fallback_env_var`.

## Persistence

By default the chart creates one PVC mounted at /home/ccc. It stores:

- /home/ccc/.config/ccc: CCC config, sessions, and watcher state.
- /home/ccc/Projects: cloned or resolved repositories.

Use persistence.existingClaim to attach an existing PVC.

## Operations

Use a narrow JQL query and `maxTicketsPerCycle: 1` until you have validated the full claim, clone, session-start, and Jira-comment path.

The watcher state file is stored at:

~~~text
/home/ccc/.config/ccc/watch-state.json
~~~

If a ticket was started intentionally, leave the entry in place so later polls do not create duplicate sessions. If a ticket should run from scratch again, edit that state file and restart the pod.
