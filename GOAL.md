# Goal: Build datadog-cli — a complete Go CLI for the Datadog API

Drop-in replacement for the Python datadog-cli. Read-only CLI for querying Datadog APIs (logs, traces, APM, metrics, hosts, containers, processes, monitors, dashboards, SLOs, incidents, RUM, audit, and more). Success = all 6 Phase 7 reviewers pass + brew formula installs + skill works in Claude Code.

## Phase 1 — Foundation {⬜ NOT STARTED}
Scaffold + pkg/ infrastructure. Binary builds, --help works, auth works against live API.
### 1A — Scaffold: directory structure, go.mod, Makefile
### 1B — pkg/api: HTTP client with rate limiting, pagination, retries, error handling
### 1C — pkg/auth: DD_API_KEY + DD_APP_KEY env vars → ~/.datadog-cli/config.yaml file chain, profiles
### 1D — pkg/output: table/JSON/plaintext + --fields + --jq

## Phase 2 — Commands {⬜ NOT STARTED}
All API resources implemented as cobra commands. Read-only (list/get/search) per resource.
### 2A — cmd/root.go: global flags (--json, --plaintext, --limit, --verbose, --debug, --profile, --site)
### 2B — cmd/<resource>.go: one file per API resource group (17+ groups, 28+ subcommands)
### 2C — Built-ins: docs, completion, skill print/add

## Phase 3 — Tests {⬜ NOT STARTED}
All three tiers pass. 90%+ unit coverage. Integration covers every command × every output flag.
### 3A — Unit tests (pkg/api, pkg/auth, pkg/output via httptest mocks)
### 3B — Integration tests (every command × every flag against httptest mock server)
### 3C — Smoke tests (make test passes, no API key needed)

## Phase 4 — Docs & Skill {⬜ NOT STARTED}
### 4A — skill/SKILL.md + skill/reference/commands.md
### 4B — README.md + llms.txt

## Phase 5 — Release Automation {⬜ NOT STARTED}
### 5A — .gitea/workflows/bump-tap.yml
### 5B — Initial brew formula in homebrew-private tap

## Phase 6 — Definition of Done {⬜ NOT STARTED}
Every check verified with actual output. No assumptions.

## Phase 7 — Review (6 Reviewers) {⬜ NOT STARTED}
All 6 reviewers pass. Loop on failures.

## Phase 8 — Claude Integration {⬜ NOT STARTED}
Skill installed, enforcer updated, jset sync-all complete.

## Available Resources
- **Skill**: creating-go-cli (all templates + patterns)
- **Python reference**: ~/gitea/datadog-cli-python (full source with 538 tests)
- **API docs**: Datadog API v1/v2 (17+ command groups, 28+ subcommands)
- **Credentials**: ~/.datadog-cli/config.yaml (api_key, app_key, site)
- **Hosting**: Gitea (private) — homebrew-private tap
- **Env vars**: DD_API_KEY, DD_APP_KEY, DD_SITE
- **Module path**: gitea.roboalch.com/roboalchemist/datadog-cli

## Command Groups (from Python reference)
| Group | Subcommands |
|-------|-------------|
| auth | scopes |
| api-keys | list |
| logs | search, aggregate, indexes |
| hosts | list, totals |
| apm | services, definitions, dependencies, deps |
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

## Success Criteria
- Phase 1: `make build` succeeds, `datadog-cli --help` runs, auth connects to live API
- Phase 3: `make test-unit` ≥ 90%, `make test-integration` passes with mock server
- Phase 7: All 6 reviewers PASS
- Phase 8: `brew install --build-from-source` + `datadog-cli skill add` both work
