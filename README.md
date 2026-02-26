# datadog-cli

A read-only CLI for the Datadog API. Query logs, metrics, monitors, dashboards, hosts, APM traces, and more from the command line.

## Installation

```bash
brew install roboalchemist/private/datadog-cli
```

## Configuration

Set environment variables or use `~/.datadog-cli/config.yaml`:

```bash
export DD_API_KEY=your_api_key
export DD_APP_KEY=your_app_key
export DD_SITE=datadoghq.com  # default
```

## Usage

```bash
datadog-cli --help
datadog-cli logs search --query "service:my-service" --from 1h
datadog-cli monitors list
datadog-cli hosts list
datadog-cli metrics query "avg:system.cpu.user{*}" --from 1h
```

## Output Formats

- Default: formatted table output
- `--json` / `-j`: JSON output
- `--plaintext` / `-p`: plain text (no color, no borders)

## Command Groups

| Group | Subcommands |
|-------|-------------|
| auth | scopes |
| api-keys | list |
| logs | search, aggregate, indexes |
| hosts | list, totals |
| apm | services, definitions, dependencies |
| traces | search, aggregate, get |
| metrics | list, query, search |
| containers | list |
| processes | list |
| dashboards | list, get, search |
| monitors | list, get, search |
| events | list, get |
| downtimes | list, get |
| incidents | list, get |
| notebooks | list, get |
| rum | search, aggregate |
| slos | list, get, history |
| tags | list, get |
| audit | search |
| usage | summary, top-metrics |
| users | list, get |
| pipelines | list, get |
