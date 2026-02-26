# datadog-cli

A read-only CLI for the Datadog API. Query logs, metrics, monitors, dashboards, hosts, APM traces, SLOs, incidents, RUM, audit events, and more from the command line.

## Installation

```bash
brew tap roboalchemist/tap ssh://git@github.com:2222/roboalchemist/homebrew-private.git
brew install datadog-cli
```

## Quick Start

```bash
# Set credentials
export DD_API_KEY=your_api_key
export DD_APP_KEY=your_app_key

# Run your first queries
datadog-cli logs search --query "service:my-service" --from 1h
datadog-cli monitors list
datadog-cli hosts list --filter "env:production"
datadog-cli auth scopes    # see required API key permissions
```

## Authentication

Credentials are resolved in priority order:

1. `--api-key` / `--app-key` flags
2. `DD_API_KEY` / `DD_APP_KEY` environment variables
3. `~/.datadog-cli/config.yaml` (profile-based config)

### Environment Variables

```bash
export DD_API_KEY=your_api_key
export DD_APP_KEY=your_app_key
export DD_SITE=datadoghq.com   # optional, default is datadoghq.com
```

### Config File

```yaml
# ~/.datadog-cli/config.yaml
default_profile: prod

profiles:
  prod:
    api_key: "your-prod-api-key"
    app_key: "your-prod-app-key"
    site: datadoghq.com
  eu:
    api_key: "your-eu-api-key"
    app_key: "your-eu-app-key"
    site: datadoghq.eu
```

Switch profiles with `--profile eu`.

### Supported Datadog Sites

| Site | `DD_SITE` value |
|------|----------------|
| US1 (default) | `datadoghq.com` |
| EU | `datadoghq.eu` |
| US3 | `us3.datadoghq.com` |
| US5 | `us5.datadoghq.com` |
| AP1 | `ap1.datadoghq.com` |

## Output Formats

```bash
# Default: formatted table with colors
datadog-cli monitors list

# JSON output
datadog-cli monitors list --json

# JSON with jq filtering
datadog-cli monitors list --json --jq '.[] | select(.overall_state=="Alert") | .name'

# Plain text (no color, no borders) — good for piping
datadog-cli logs search -q "service:api" --from 1h --plaintext | grep ERROR

# Limit results
datadog-cli hosts list --limit 20

# Show only specific columns
datadog-cli monitors list --fields "ID,Name,Status"

# Disable color only
datadog-cli monitors list --no-color
```

## Command Groups

| Group | Subcommands | Description |
|-------|-------------|-------------|
| `auth` | `scopes` | List required API key scopes |
| `api-keys` | `list` | List API keys (metadata only) |
| `logs` | `search`, `aggregate`, `indexes` | Log Explorer queries and analytics |
| `traces` | `search`, `aggregate`, `get` | APM spans and trace data |
| `apm` | `services`, `definitions`, `dependencies` | APM service catalog |
| `metrics` | `list`, `query`, `search` | Metrics and timeseries data |
| `monitors` | `list`, `get`, `search` | Alert monitors |
| `hosts` | `list`, `totals` | Infrastructure hosts |
| `containers` | `list` | Container data |
| `processes` | `list` | Process data |
| `dashboards` | `list`, `get`, `search` | Dashboards |
| `events` | `list`, `get` | Event stream |
| `downtimes` | `list`, `get` | Downtime schedules |
| `incidents` | `list`, `get` | Incidents |
| `notebooks` | `list`, `get` | Notebooks |
| `rum` | `search`, `aggregate` | Real User Monitoring |
| `slos` | `list`, `get`, `history` | Service Level Objectives |
| `tags` | `list`, `get` | Infrastructure tags |
| `audit` | `search` | Audit trail |
| `usage` | `summary`, `top-metrics` | Usage metering |
| `users` | `list`, `get` | User management |
| `pipelines` | `list`, `get` | Log pipelines |

## Examples

**Search logs for errors:**
```bash
datadog-cli logs search --query "service:api-gateway status:error" --from 1h
```

**Aggregate log errors by service:**
```bash
datadog-cli logs aggregate -q "status:error" --group-by service --compute count --from 1h
```

**Search APM traces:**
```bash
datadog-cli traces search -q "service:checkout @duration:>500ms" --from 30m
```

**List monitors in alert state:**
```bash
datadog-cli monitors list --group-states "alert" --json
```

**Get host counts:**
```bash
datadog-cli hosts totals
datadog-cli hosts list --filter "env:production" --limit 50
```

**Get SLO history:**
```bash
datadog-cli slos list
datadog-cli slos history --id <slo-id> --from 30d
```

**Query a metric:**
```bash
datadog-cli metrics query "avg:system.cpu.user{env:production}" --from 1h
```

**Search audit trail:**
```bash
datadog-cli audit search -q "action:deleted @asset.type:dashboard" --from 24h
```

## Time Formats

`--from` and `--to` accept:
- Relative durations: `15m`, `1h`, `2d`, `1w`
- ISO-8601: `2024-01-15T00:00:00Z`
- Unix timestamps (seconds): `1705276800`
- `now` (for `--to`)

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | `-j` | false | JSON output |
| `--plaintext` | `-p` | false | Plain text (no color, no borders) |
| `--no-color` | | false | Disable color only |
| `--quiet` | `-q` | false | Suppress progress output |
| `--silent` | | false | Suppress progress output (synonym for --quiet) |
| `--limit` | `-l` | 100 | Max results |
| `--fields` | | | Comma-separated column filter |
| `--jq` | | | JQ expression for JSON output |
| `--profile` | | | Config profile name |
| `--site` | | datadoghq.com | Datadog site |
| `--api-key` | | | API key (overrides env/config) |
| `--app-key` | | | App key (overrides env/config) |
| `--debug` | | false | Debug logging |
| `--verbose` | `-v` | false | Verbose output |
| `--version` | `-V` | | Print version and exit |

## Exit Status

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | User/authentication error |
| `2` | Usage error (invalid flags or arguments) |
| `3` | System/network/server error |

When `--json` is active, errors are emitted to stderr as structured JSON:
```json
{"error": "Unauthorized (401)", "code": "auth_error", "recoverable": false}
```

## Shell Completion

```bash
# Zsh
datadog-cli completion zsh > "${fpath[1]}/_datadog-cli"

# Bash
datadog-cli completion bash > /etc/bash_completion.d/datadog-cli

# Fish
datadog-cli completion fish > ~/.config/fish/completions/datadog-cli.fish
```

## Claude Code Integration

```bash
# Install the Claude Code skill
datadog-cli skill add

# Print skill to stdout
datadog-cli skill print
```

The skill is installed to `~/.claude/skills/datadog-cli/` and enables Claude Code to understand datadog-cli commands and flags.
