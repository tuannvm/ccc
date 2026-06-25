# Jira Ticket Watcher

The Jira watcher turns Jira into a queue for CCC sessions. It polls a configured Jira query, claims eligible tickets, resolves the repository from a Jira field, and starts a detached CCC session with a ticket-specific prompt.

The watcher does not run a separate agent runtime. It uses the same detached session system as Telegram-triggered CCC work; the watcher owns intake and session creation, and the agent inside the session owns implementation, validation, Jira progress comments, and ticket handoff.

## Commands

Run one dry check before enabling automation:

```bash
ccc watch jira --once --dry-run
```

Run one real polling pass:

```bash
ccc poll
```

Run continuously:

```bash
ccc watch jira
```

`ccc poll` is equivalent to one `ccc watch jira --once` pass.

## Configuration

The watcher reads this file by default:

```text
~/.config/ccc/jira.json
```

It can also run entirely from `CCC_JIRA_*` environment variables. Environment variables override file values, which is the preferred mode for container and Kubernetes deployments.

Minimal `jira.json`:

```json
{
  "base_url": "https://example.atlassian.net",
  "auth_method": "basic",
  "auth_env_var": "CCC_JIRA_AUTH_TOKEN",
  "auth_email_env_var": "CCC_JIRA_AUTH_EMAIL",
  "jql": "project = ABC AND status = \"Ready for Dev\" ORDER BY priority DESC",
  "claim_status": "In Progress",
  "repo_field": "customfield_10001",
  "acceptance_criteria_field": "customfield_10002",
  "poll_interval": "1m",
  "max_tickets_per_cycle": 1
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `base_url` | string | Jira base URL, for example `https://example.atlassian.net` |
| `auth_method` | string | `bearer` or `basic`; defaults to `basic` when an email is configured, otherwise `bearer` |
| `auth_env_var` | string | Environment variable containing the Jira API token |
| `auth_email_env_var` | string | Environment variable containing the Jira email for basic auth |
| `env_file` | string | Optional env file path; defaults to `~/.config/ccc/.env` |
| `jql` | string | Jira query used to find candidate tickets |
| `claim_transition` | string | Jira transition ID or transition name to claim a ticket |
| `claim_status` | string | Destination status name to claim a ticket, for example `In Progress` |
| `repo_field` | string | Jira field containing a repo path, repo name, or Git URL |
| `repo_fallback_env_var` | string | Optional environment variable used when the repo field is empty |
| `acceptance_criteria_field` | string | Optional Jira field copied into the session prompt |
| `poll_interval` | duration | Continuous polling interval, for example `30s` or `1m`; default is `1m` |
| `max_tickets_per_cycle` | integer | Maximum tickets to start per polling cycle; default is `1` |

Either `claim_transition` or `claim_status` is required. `repo_field` is required. `repo_fallback_env_var` is used only when the configured Jira field is empty on a ticket.

### Environment Variables

| Variable | Maps to |
|----------|---------|
| `CCC_JIRA_BASE_URL` | `base_url` |
| `CCC_JIRA_AUTH_TOKEN` | Direct Jira token value |
| `CCC_JIRA_AUTH_ENV_VAR` | `auth_env_var` |
| `CCC_JIRA_AUTH_EMAIL` | Direct Jira email value |
| `CCC_JIRA_AUTH_EMAIL_ENV_VAR` | `auth_email_env_var` |
| `CCC_JIRA_AUTH_METHOD` | `auth_method` |
| `CCC_JIRA_ENV_FILE` | `env_file` |
| `CCC_JIRA_JQL` | `jql` |
| `CCC_JIRA_CLAIM_TRANSITION` | `claim_transition` |
| `CCC_JIRA_CLAIM_STATUS` | `claim_status` |
| `CCC_JIRA_REPO_FIELD` | `repo_field` |
| `CCC_JIRA_REPO_FALLBACK` | Direct repo fallback value |
| `CCC_JIRA_REPO_FALLBACK_ENV_VAR` | `repo_fallback_env_var` |
| `CCC_JIRA_ACCEPTANCE_CRITERIA_FIELD` | `acceptance_criteria_field` |
| `CCC_JIRA_POLL_INTERVAL` | `poll_interval` |
| `CCC_JIRA_MAX_TICKETS_PER_CYCLE` | `max_tickets_per_cycle` |

## Required Setup

1. Put the Jira API token in the configured token environment variable.
2. For basic auth, put the Jira account email in the configured email environment variable.
3. Make the Jira repo field contain either a local repo path, a repo name under `projects_dir`, or a Git URL.
4. Configure either `claim_transition` or `claim_status` so CCC can claim the ticket before starting work.
5. Choose a default provider, for example `codex`, in CCC provider configuration.

For Kubernetes deployment details, see [Kubernetes](kubernetes.md).

## Technical Design

### Components

| Component | File | Responsibility |
|-----------|------|----------------|
| Watch CLI | `pkg/watch/cli.go` | Implements `ccc watch jira` and `ccc poll` |
| Runner | `pkg/watch/runner.go` | Coordinates polling, duplicate suppression, claim, repo resolution, session startup, and comments |
| Jira Provider | `pkg/watch/jira.go` | Calls Jira REST APIs, normalizes issues into tickets, claims tickets, and posts comments |
| Jira Config | `pkg/watch/jira_config.go` | Loads `jira.json`, env overrides, token/email values, and poll settings |
| Repo Resolver | `pkg/watch/repo.go` | Resolves a Jira repo field into a local repo path, cloning Git URLs when needed |
| Session Starter | `pkg/watch/session.go` | Builds the Jira prompt and starts a detached CCC session |
| State Store | `pkg/watch/state.go` | Stores claimed and started ticket state in `watch-state.json` |

### Flow

```text
Jira JQL
  |
  v
Poll candidates
  |
  v
Skip tickets already in successful local watcher state
  |
  v
Fetch full ticket context
  |
  v
Resolve repo field to path or clone target
  |
  v
Claim ticket through transition ID, transition name, or target status
  |
  v
Persist claimed state
  |
  v
Start detached CCC session with generated prompt
  |
  v
Persist topic/session state and post Jira start comment
```

### Duplicate and Retry Semantics

The watcher uses `~/.config/ccc/watch-state.json` as local idempotency state. A ticket key is namespaced by provider, for example `jira:ABC-123`.

- Tickets with successful started state are skipped on later polls.
- Dry runs do not read or write watcher state.
- If ticket claim succeeds but session startup fails, the entry stores `last_error` and no `started_at`.
- Startup-failed entries are retried on later polls without claiming the Jira ticket again.

## Runtime Behavior

- `--dry-run` lists eligible tickets without claiming tickets or starting sessions.
- On successful startup, CCC posts a short Jira comment with the session name, repo path, and Telegram topic ID when available.
- The generated session prompt includes the Jira key, title, URL, description, acceptance criteria, and instructions to keep Jira updated and move the ticket to In Review when the work is complete.

