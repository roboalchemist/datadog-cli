# datadog-cli

Read-only CLI for the Datadog API. Query logs, metrics, monitors, dashboards, hosts, APM traces, and more.

## Quick Start

```bash
# Set credentials
export DD_API_KEY=your_api_key
export DD_APP_KEY=your_app_key

# Basic usage
datadog-cli logs search --query "service:my-service" --from 1h
datadog-cli monitors list
datadog-cli hosts list --limit 50
```

## Output Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | `-j` | JSON output |
| `--plaintext` | `-p` | Plain text, no color |
| `--limit N` | `-l` | Max results (default 100) |
| `--jq EXPR` | | Apply jq expression to JSON output |

## Auth

Credentials resolved in order:
1. `--api-key` / `--app-key` flags
2. `DD_API_KEY` / `DD_APP_KEY` environment variables
3. `~/.datadog-cli/config.yaml` (profile-based)

## Reference

See [reference/commands.md](reference/commands.md) for full command reference.
