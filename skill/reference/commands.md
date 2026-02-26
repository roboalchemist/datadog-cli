# datadog-cli Command Reference

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | `-j` | false | Output as JSON |
| `--plaintext` | `-p` | false | Plain text output (no color/borders) |
| `--no-color` | | false | Disable color output |
| `--debug` | | false | Enable debug logging |
| `--verbose` | `-v` | false | Verbose output |
| `--limit` | `-l` | 100 | Maximum number of results |
| `--profile` | | | Config profile name |
| `--site` | | datadoghq.com | Datadog site |
| `--api-key` | | | Datadog API key |
| `--app-key` | | | Datadog Application key |

## Commands

### auth
```bash
datadog-cli auth scopes          # List API key scopes
```

### api-keys
```bash
datadog-cli api-keys list        # List API keys
```

### logs
```bash
datadog-cli logs search --query "service:foo" --from 1h --to now
datadog-cli logs aggregate --query "service:foo" --group-by service
datadog-cli logs indexes         # List log indexes
```

### hosts
```bash
datadog-cli hosts list           # List hosts
datadog-cli hosts totals         # Host count totals
```

### apm
```bash
datadog-cli apm services         # List APM services
datadog-cli apm definitions      # List APM service definitions
datadog-cli apm dependencies     # List APM service dependencies
```

### traces
```bash
datadog-cli traces search --query "service:foo" --from 1h
datadog-cli traces aggregate --query "service:foo"
datadog-cli traces get --id <trace-id>
```

### metrics
```bash
datadog-cli metrics list                              # List metric names
datadog-cli metrics query "avg:system.cpu.user{*}" --from 1h
datadog-cli metrics search --query cpu
```

### containers
```bash
datadog-cli containers list      # List containers
```

### processes
```bash
datadog-cli processes list       # List processes
```

### dashboards
```bash
datadog-cli dashboards list      # List dashboards
datadog-cli dashboards get --id <dashboard-id>
datadog-cli dashboards search --query "my dash"
```

### monitors
```bash
datadog-cli monitors list        # List monitors
datadog-cli monitors get --id <monitor-id>
datadog-cli monitors search --query "service:foo"
```

### events
```bash
datadog-cli events list --from 1h
datadog-cli events get --id <event-id>
```

### downtimes
```bash
datadog-cli downtimes list       # List downtimes
datadog-cli downtimes get --id <downtime-id>
```

### incidents
```bash
datadog-cli incidents list       # List incidents
datadog-cli incidents get --id <incident-id>
```

### notebooks
```bash
datadog-cli notebooks list       # List notebooks
datadog-cli notebooks get --id <notebook-id>
```

### rum
```bash
datadog-cli rum search --query "service:foo" --from 1h
datadog-cli rum aggregate --query "service:foo"
```

### slos
```bash
datadog-cli slos list            # List SLOs
datadog-cli slos get --id <slo-id>
datadog-cli slos history --id <slo-id> --from 1d
```

### tags
```bash
datadog-cli tags list            # List all tags
datadog-cli tags get --host <hostname>
```

### audit
```bash
datadog-cli audit search --query "action:created" --from 1h
```

### usage
```bash
datadog-cli usage summary        # Usage summary
datadog-cli usage top-metrics    # Top metrics by usage
```

### users
```bash
datadog-cli users list           # List users
datadog-cli users get --id <user-id>
```

### pipelines
```bash
datadog-cli pipelines list       # List log pipelines
datadog-cli pipelines get --id <pipeline-id>
```

### Built-ins
```bash
datadog-cli docs                 # Show documentation
datadog-cli completion bash      # Shell completion
datadog-cli completion zsh
datadog-cli skill print          # Print skill markdown
datadog-cli skill add            # Install skill to ~/.claude/skills/
```
