# datadog-cli

Read-only CLI for querying Datadog APIs. Covers logs, traces, APM, metrics, hosts, monitors, dashboards, SLOs, incidents, RUM, audit, usage, and more.

## Quick Start

```bash
# Set credentials (required)
export DD_API_KEY=your_api_key
export DD_APP_KEY=your_app_key

# First commands
datadog-cli logs search --query "service:my-service" --from 1h
datadog-cli monitors list
datadog-cli hosts list --limit 20
datadog-cli auth scopes        # show required API key scopes
```

## Authentication

Credentials are resolved in priority order:

1. `--api-key` / `--app-key` flags (one-off override)
2. `DD_API_KEY` / `DD_APP_KEY` environment variables (recommended for scripts)
3. `~/.datadog-cli/config.yaml` (named profiles for multiple accounts)

Config file with profiles:

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

Switch profiles: `datadog-cli --profile eu monitors list`

Supported sites: `datadoghq.com` (default), `datadoghq.eu`, `us3.datadoghq.com`, `us5.datadoghq.com`, `ap1.datadoghq.com`

Override site: `export DD_SITE=datadoghq.eu`

## Output Formats

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | `-j` | Raw JSON output (first page) |
| `--plaintext` | `-p` | Plain text, no color, no borders |
| `--no-color` | | Disable ANSI colors only |
| `--limit N` | `-l` | Max results (default 100) |
| `--fields f1,f2` | | Display only named columns |
| `--jq EXPR` | | Apply jq expression to JSON output |

```bash
# JSON with jq filtering
datadog-cli monitors list --json --jq '.[] | select(.overall_state=="Alert") | .name'

# Limit results
datadog-cli hosts list --limit 10

# Specific fields only
datadog-cli monitors list --fields "ID,Name,Status"

# Plain text for piping
datadog-cli logs search -q "service:api" --from 1h --plaintext | grep ERROR
```

## Examples

**Search logs for errors in the last hour:**
```bash
datadog-cli logs search --query "service:api-gateway status:error" --from 1h
```

**Search APM traces by service with JSON output:**
```bash
datadog-cli traces search --query "service:checkout-service @duration:>500ms" --from 30m --json
```

**List all hosts and filter to production:**
```bash
datadog-cli hosts list --filter "env:production" --limit 50
```

**List monitors in alert state as JSON:**
```bash
datadog-cli monitors list --json | jq '.[] | select(.overall_state=="Alert")'
# or using built-in --jq:
datadog-cli monitors list --json --jq '.[] | select(.overall_state=="Alert")'
```

**Aggregate log errors by service:**
```bash
datadog-cli logs aggregate --query "status:error" --group-by service --compute count --from 1h
```

**Get SLO history for the last 30 days:**
```bash
datadog-cli slos list
datadog-cli slos history --id <slo-id> --from 30d
```

**Check required API key scopes:**
```bash
datadog-cli auth scopes
datadog-cli auth scopes --json
```

## Time Formats

`--from` and `--to` accept:
- Relative: `15m`, `1h`, `2d`, `1w`
- ISO-8601: `2024-01-15T00:00:00Z`
- Unix timestamp (seconds): `1705276800`
- `now` (for `--to`)

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | `-j` | false | JSON output |
| `--plaintext` | `-p` | false | Plain text (no color/borders) |
| `--no-color` | | false | Disable color output |
| `--limit` | `-l` | 100 | Max results |
| `--fields` | | | Comma-separated column filter |
| `--jq` | | | JQ expression for JSON output |
| `--profile` | | | Config profile name |
| `--site` | | datadoghq.com | Datadog site |
| `--api-key` | | | Datadog API key |
| `--app-key` | | | Datadog App key |
| `--debug` | | false | Debug logging |
| `--verbose` | `-v` | false | Verbose output |

## Command Groups

| Group | Subcommands | Description |
|-------|-------------|-------------|
| `auth` | `scopes` | API key scope requirements |
| `api-keys` | `list` | List API keys |
| `logs` | `search`, `aggregate`, `indexes` | Log Explorer |
| `traces` | `search`, `aggregate`, `get` | APM spans |
| `apm` | `services`, `definitions`, `dependencies` | APM service catalog |
| `metrics` | `list`, `query`, `search` | Metrics and timeseries |
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

## Skill Management

```bash
datadog-cli skill print          # Print this skill to stdout
datadog-cli skill add            # Install to ~/.claude/skills/datadog-cli/
datadog-cli docs                 # Show README
datadog-cli completion zsh       # Shell completion
datadog-cli completion bash
```

## Reference

See [reference/commands.md](reference/commands.md) for complete flag reference for every command.
