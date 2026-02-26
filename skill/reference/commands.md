# datadog-cli Command Reference

Complete flag reference for every command group and subcommand.

## Global Flags

Apply to all commands.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | `-j` | false | Output as JSON |
| `--plaintext` | `-p` | false | Plain text output (no color, no borders) |
| `--no-color` | | false | Disable color output |
| `--debug` | | false | Enable debug logging |
| `--verbose` | `-v` | false | Verbose output |
| `--limit` | `-l` | 100 | Maximum number of results to return |
| `--fields` | | | Comma-separated list of columns to display |
| `--jq` | | | JQ expression to filter JSON output |
| `--profile` | | | Config profile name (from ~/.datadog-cli/config.yaml) |
| `--site` | | datadoghq.com | Datadog site |
| `--api-key` | | | Datadog API key (overrides env/config) |
| `--app-key` | | | Datadog Application key (overrides env/config) |

---

## auth

Authentication utilities. No API call is made.

### auth scopes

Display the Datadog API/App key scopes required by each command group.
Use this to create a minimally-scoped Application Key.

```bash
datadog-cli auth scopes
datadog-cli auth scopes --json
```

No command-specific flags.

---

## api-keys

### api-keys list

List API keys in your Datadog account (metadata only, no key values exposed).

```bash
datadog-cli api-keys list
datadog-cli api-keys list --json
datadog-cli api-keys list --limit 20
```

No command-specific flags (uses global `--limit`).

Required scope: `api_keys_read`

---

## logs

Query and aggregate logs from Datadog Log Explorer.

### logs search

Search logs matching a query. Supports cursor-based pagination up to `--limit`.

```bash
datadog-cli logs search --query "service:my-app status:error"
datadog-cli logs search -q "service:api-gateway" --from 1h --to now
datadog-cli logs search -q "@http.status_code:>=500" --limit 50
datadog-cli logs search -q "*" --from 2024-01-15T00:00:00Z --to 2024-01-16T00:00:00Z
datadog-cli logs search -q "service:api" --from 1h --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Log search query |
| `--from` | | `15m` | Start time (relative, ISO-8601, or unix) |
| `--to` | | `now` | End time (relative, ISO-8601, or unix) |
| `--sort` | | `-timestamp` | Sort order: `timestamp` (asc) or `-timestamp` (desc) |
| `--indexes` | | all | Log indexes to search (comma-separated) |

Required scopes: `logs_read_data`, `logs_read_index_data`

### logs aggregate

Aggregate logs using Datadog Log Analytics.

```bash
datadog-cli logs aggregate -q "service:*" --group-by service --compute count
datadog-cli logs aggregate -q "status:error" --group-by host --compute count --from 1h
datadog-cli logs aggregate -q "*" --from 1d --group-by status --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Log search query |
| `--from` | | `15m` | Start time |
| `--to` | | `now` | End time |
| `--compute` | | `count` | Aggregation type: `count`, `sum`, `avg`, `min`, `max` |
| `--group-by` | | | Field to group by (e.g. `service`, `host`, `status`) |

Required scopes: `logs_read_data`, `logs_read_index_data`

### logs indexes

List all log indexes configured in your Datadog account.

```bash
datadog-cli logs indexes
datadog-cli logs indexes --json
```

No command-specific flags.

Required scope: `logs_read_config`

---

## traces

Query APM spans. Rate limit: 300 requests/hour on search and aggregate.

### traces search

Search APM spans using Datadog query syntax.

```bash
datadog-cli traces search --query "service:my-app"
datadog-cli traces search -q "service:api @duration:>1s" --from 1h
datadog-cli traces search -q "service:api env:production" --from 2h --to 1h
datadog-cli traces search -q "service:api" --sort -duration
datadog-cli traces search -q "*" --limit 50 --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Span search query |
| `--from` | | `15m` | Start time |
| `--to` | | `now` | End time |
| `--sort` | | | Sort field: `timestamp`, `-timestamp`, `duration`, `-duration` |
| `--filter-query` | | | Additional filter query |

Required scope: `apm_read`

### traces aggregate

Aggregate APM spans by fields.

```bash
datadog-cli traces aggregate -q "service:api" --group-by service --compute count
datadog-cli traces aggregate -q "env:production" --group-by resource_name
datadog-cli traces aggregate -q "service:api" --compute avg --from 2h --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Span search query |
| `--from` | | `15m` | Start time |
| `--to` | | `now` | End time |
| `--compute` | | `count` | Aggregation type: `count`, `sum`, `avg`, `min`, `max` |
| `--group-by` | | | Field to group by (e.g. `service`, `resource_name`) |

Required scope: `apm_read`

### traces get

Get a specific span by its span ID.

```bash
datadog-cli traces get abc123def456
datadog-cli traces get abc123def456 --json
```

Argument: `<span_id>` (required, positional)

Required scope: `apm_read`

---

## apm

Query APM service catalog.

### apm services

List all APM services.

```bash
datadog-cli apm services
datadog-cli apm services --json
```

No command-specific flags.

Required scope: `apm_read`

### apm definitions

List APM service definitions from the Service Catalog.

```bash
datadog-cli apm definitions
datadog-cli apm definitions --json
```

No command-specific flags.

Required scope: `apm_service_catalog_read`

### apm dependencies

List APM service dependencies.

```bash
datadog-cli apm dependencies
datadog-cli apm deps          # alias
```

No command-specific flags.

Required scope: `apm_read`

---

## metrics

Query Datadog metrics.

### metrics list

List active metric names.

```bash
datadog-cli metrics list
datadog-cli metrics list --limit 50
```

No command-specific flags (uses global `--limit`).

Required scope: `metrics_read`

### metrics query

Query metric timeseries data.

```bash
datadog-cli metrics query "avg:system.cpu.user{*}" --from 1h
datadog-cli metrics query "sum:aws.ec2.cpuutilization{env:production}" --from 2h --to 1h
datadog-cli metrics query "avg:system.mem.used{*} by {host}" --json
```

Argument: `<query>` (required, positional — Datadog metrics query string)

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--from` | | `1h` | Start time |
| `--to` | | `now` | End time |

Required scope: `timeseries_query`

### metrics search

Search metric names by prefix or substring.

```bash
datadog-cli metrics search --query cpu
datadog-cli metrics search -q system.mem --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Metric name search term |

Required scope: `metrics_read`

---

## hosts

Query infrastructure hosts.

### hosts list

List infrastructure hosts.

```bash
datadog-cli hosts list
datadog-cli hosts list --filter "env:production"
datadog-cli hosts list --sort-field cpu --sort-dir desc
datadog-cli hosts list --count 50 --start 0
datadog-cli hosts list --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--filter` | | | Filter by name, alias, or tag (e.g. `env:production`) |
| `--sort-field` | | | Field to sort by (e.g. `cpu`, `name`) |
| `--sort-dir` | | | Sort direction: `asc` or `desc` |
| `--count` | | 0 (uses --limit) | Number of hosts to return |
| `--start` | | 0 | Starting offset for pagination |

Required scope: `hosts_read`

### hosts totals

Show total and active host counts.

```bash
datadog-cli hosts totals
datadog-cli hosts totals --json
```

No command-specific flags.

Required scope: `hosts_read`

---

## containers

### containers list

List containers.

```bash
datadog-cli containers list
datadog-cli containers list --limit 50
datadog-cli containers list --json
```

No command-specific flags (uses global `--limit`).

Required scope: `containers_read`

---

## processes

### processes list

List processes.

```bash
datadog-cli processes list
datadog-cli processes list --limit 50
datadog-cli processes list --json
```

No command-specific flags (uses global `--limit`).

Required scope: `processes_read`

---

## dashboards

### dashboards list

List dashboards.

```bash
datadog-cli dashboards list
datadog-cli dashboards list --json
datadog-cli dashboards list --limit 50
```

No command-specific flags (uses global `--limit`).

Required scope: `dashboards_read`

### dashboards get

Get a dashboard by ID.

```bash
datadog-cli dashboards get abc-123-def
datadog-cli dashboards get abc-123-def --json
```

Argument: `<dashboard_id>` (required, positional)

Required scope: `dashboards_read`

### dashboards search

Search dashboards by name or description.

```bash
datadog-cli dashboards search --query "infrastructure"
datadog-cli dashboards search -q "my team" --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Search query |

Required scope: `dashboards_read`

---

## monitors

### monitors list

List monitors.

```bash
datadog-cli monitors list
datadog-cli monitors list --tags "env:production"
datadog-cli monitors list --name "CPU"
datadog-cli monitors list --group-states "alert,warn"
datadog-cli monitors list --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--group-states` | | | Filter by group states: `alert`, `warn`, `no data`, `ok` (comma-separated) |
| `--name` | | | Filter by monitor name (substring match) |
| `--tags` | | | Filter by tags (comma-separated, e.g. `env:prod,team:backend`) |

Required scope: `monitors_read`

### monitors get

Get monitor details by ID.

```bash
datadog-cli monitors get 12345
datadog-cli monitors get 12345 --json
```

Argument: `<monitor_id>` (required, positional)

Required scope: `monitors_read`

### monitors search

Search monitors by name, tags, or query string.

```bash
datadog-cli monitors search --query "cpu"
datadog-cli monitors search -q "env:production"
datadog-cli monitors search --query "disk" --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Search query |

Required scope: `monitors_read`

---

## events

### events list

List events from the event stream.

```bash
datadog-cli events list
datadog-cli events list --from 2h
datadog-cli events list --start 1d --end now --priority normal
datadog-cli events list --tags "env:production" --sources "nagios,chef"
datadog-cli events list --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--start` | | `1d` | Start time (relative, ISO-8601, or Unix seconds) |
| `--end` | | `now` | End time |
| `--priority` | | | Filter by priority: `low`, `normal`, `all` |
| `--sources` | | | Filter by source names (comma-separated) |
| `--tags` | | | Filter by tags (comma-separated) |

Required scope: `events_read`

### events get

Get a specific event by ID.

```bash
datadog-cli events get 12345678
datadog-cli events get 12345678 --json
```

Argument: `<event_id>` (required, positional)

Required scope: `events_read`

---

## downtimes

### downtimes list

List downtime schedules.

```bash
datadog-cli downtimes list
datadog-cli downtimes list --json
```

No command-specific flags (uses global `--limit`).

Required scope: `monitors_downtime`

### downtimes get

Get a downtime by ID.

```bash
datadog-cli downtimes get 12345
datadog-cli downtimes get 12345 --json
```

Argument: `<downtime_id>` (required, positional)

Required scope: `monitors_downtime`

---

## incidents

### incidents list

List incidents.

```bash
datadog-cli incidents list
datadog-cli incidents list --json
datadog-cli incidents list --limit 20
```

No command-specific flags (uses global `--limit`).

Required scope: `incident_read`

### incidents get

Get an incident by ID.

```bash
datadog-cli incidents get abc123
datadog-cli incidents get abc123 --json
```

Argument: `<incident_id>` (required, positional)

Required scope: `incident_read`

---

## notebooks

### notebooks list

List notebooks.

```bash
datadog-cli notebooks list
datadog-cli notebooks list --json
```

No command-specific flags (uses global `--limit`).

Required scope: `notebooks_read`

### notebooks get

Get a notebook by ID.

```bash
datadog-cli notebooks get 12345
datadog-cli notebooks get 12345 --json
```

Argument: `<notebook_id>` (required, positional)

Required scope: `notebooks_read`

---

## rum

Query Real User Monitoring events.

### rum search

Search RUM events.

```bash
datadog-cli rum search --query "service:my-app"
datadog-cli rum search -q "@error.type:NetworkError" --from 1h
datadog-cli rum search -q "application.id:abc123" --from 2h --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | RUM search query |
| `--from` | | `15m` | Start time |
| `--to` | | `now` | End time |

Required scope: `rum_read`

### rum aggregate

Aggregate RUM events by fields.

```bash
datadog-cli rum aggregate -q "service:*" --group-by application.name --compute count
datadog-cli rum aggregate -q "@error.type:*" --group-by @error.type --from 1h
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | RUM search query |
| `--from` | | `15m` | Start time |
| `--to` | | `now` | End time |
| `--compute` | | `count` | Aggregation type: `count`, `sum`, `avg`, `min`, `max` |
| `--group-by` | | | Field to group by |

Required scope: `rum_read`

---

## slos

### slos list

List Service Level Objectives.

```bash
datadog-cli slos list
datadog-cli slos list --ids "abc123,def456"
datadog-cli slos list --tags-query "env:production"
datadog-cli slos list --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--ids` | | | Comma-separated list of SLO IDs to filter |
| `--tags-query` | | | Filter by tags (e.g. `env:production`) |

Required scope: `slos_read`

### slos get

Get an SLO by ID.

```bash
datadog-cli slos get abc123def456
datadog-cli slos get abc123def456 --json
```

Argument: `<slo_id>` (required, positional)

Required scope: `slos_read`

### slos history

Get SLO history for a time window.

```bash
datadog-cli slos history --id abc123def456
datadog-cli slos history --id abc123def456 --from 30d
datadog-cli slos history --id abc123def456 --from 7d --to now --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--id` | | (required) | SLO ID |
| `--from` | | `7d` | Start of history window (relative or Unix seconds) |
| `--to` | | `now` | End of history window |

Required scope: `slos_read`

---

## tags

### tags list

List all infrastructure tags.

```bash
datadog-cli tags list
datadog-cli tags list --json
```

No command-specific flags.

Required scope: `hosts_read`

### tags get

Get tags for a specific host.

```bash
datadog-cli tags get --host my-hostname
datadog-cli tags get --host my-hostname --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--host` | | (required) | Hostname to get tags for |

Required scope: `hosts_read`

---

## audit

### audit search

Search the Datadog audit trail.

```bash
datadog-cli audit search --query "action:created"
datadog-cli audit search -q "action:deleted @asset.type:dashboard" --from 1h
datadog-cli audit search -q "@usr.email:user@example.com" --from 24h --json
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--query` | `-q` | (required) | Audit search query |
| `--from` | | `15m` | Start time |
| `--to` | | `now` | End time |

Required scope: `audit_logs_read`

---

## usage

### usage summary

Get usage summary.

```bash
datadog-cli usage summary
datadog-cli usage summary --json
```

No command-specific flags.

Required scope: `usage_read`

### usage top-metrics

Get top metrics by usage.

```bash
datadog-cli usage top-metrics
datadog-cli usage top-metrics --json
```

No command-specific flags.

Required scope: `usage_read`

---

## users

### users list

List users in the organization.

```bash
datadog-cli users list
datadog-cli users list --limit 50
datadog-cli users list --json
```

No command-specific flags (uses global `--limit`).

Required scope: `user_access_read`

### users get

Get a user by ID.

```bash
datadog-cli users get abc-123-def
datadog-cli users get abc-123-def --json
```

Argument: `<user_id>` (required, positional)

Required scope: `user_access_read`

---

## pipelines

### pipelines list

List log pipelines.

```bash
datadog-cli pipelines list
datadog-cli pipelines list --json
```

No command-specific flags.

Required scope: `logs_read_config`

### pipelines get

Get a log pipeline by ID.

```bash
datadog-cli pipelines get abc123
datadog-cli pipelines get abc123 --json
```

Argument: `<pipeline_id>` (required, positional)

Required scope: `logs_read_config`

---

## Built-in Commands

### skill

```bash
datadog-cli skill print          # Print SKILL.md to stdout
datadog-cli skill add            # Install to ~/.claude/skills/datadog-cli/
```

### docs

```bash
datadog-cli docs                 # Show README.md
```

### completion

```bash
datadog-cli completion bash      # Bash shell completion
datadog-cli completion zsh       # Zsh shell completion
datadog-cli completion fish      # Fish shell completion
datadog-cli completion powershell
```

---

## Required Scopes Summary

| Scope | Used By |
|-------|---------|
| `api_keys_read` | api-keys |
| `apm_read` | apm, traces |
| `apm_service_catalog_read` | apm definitions |
| `audit_logs_read` | audit |
| `containers_read` | containers |
| `dashboards_read` | dashboards |
| `events_read` | events |
| `hosts_read` | hosts, tags |
| `incident_read` | incidents |
| `logs_read_config` | logs indexes, pipelines |
| `logs_read_data` | logs |
| `logs_read_index_data` | logs |
| `metrics_read` | metrics |
| `monitors_downtime` | downtimes |
| `monitors_read` | monitors |
| `notebooks_read` | notebooks |
| `processes_read` | processes |
| `rum_read` | rum |
| `slos_read` | slos |
| `timeseries_query` | metrics query |
| `usage_read` | usage |
| `user_access_read` | users |

Run `datadog-cli auth scopes` to see this list with per-command breakdown.
