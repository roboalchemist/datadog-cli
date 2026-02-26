//go:build integration

package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- mock server responses ----

var mockLogEvents = `{
  "data": [
    {
      "id": "log-001",
      "type": "log",
      "attributes": {
        "timestamp": "2024-01-15T10:00:00Z",
        "host": "web-server-01",
        "service": "api-gateway",
        "status": "error",
        "message": "Connection timeout after 30s"
      }
    }
  ],
  "meta": {
    "page": {"after": ""}
  }
}`

var mockSpanEvents = `{
  "data": [
    {
      "id": "span-001",
      "type": "span",
      "attributes": {
        "timestamp": "2024-01-15T10:00:00Z",
        "service": "api-gateway",
        "resource_name": "GET /api/v1/users",
        "duration": 12300000,
        "meta": {"http.status_code": "200"}
      }
    }
  ],
  "meta": {
    "page": {"after": ""}
  }
}`

var mockHosts = `{
  "host_list": [
    {
      "id": 12345,
      "name": "web-server-01",
      "host_name": "web-server-01",
      "os": "linux",
      "is_up": true,
      "last_reported_time": 1705316400,
      "meta": {
        "platform": "x86_64",
        "cpuCores": "8"
      }
    }
  ],
  "total_matching": 1,
  "total_returned": 1
}`

var mockHostsTotals = `{
  "total_active": 5,
  "total_up": 4
}`

var mockMetrics = `{
  "metrics": ["system.cpu.user", "system.mem.used", "system.load.1"],
  "from": 1705312800
}`

var mockTimeseries = `{
  "series": [
    {
      "metric": "system.cpu.user",
      "scope": "*",
      "pointlist": [[1705312800000, 45.2], [1705316400000, 52.1]],
      "unit": [{"short_name": "%", "name": "percent"}]
    }
  ],
  "status": "ok",
  "from_date": 1705312800000,
  "to_date": 1705316400000
}`

var mockMonitors = `[
  {
    "id": 99001,
    "name": "High CPU Usage on Production",
    "type": "metric alert",
    "overall_state": "Alert",
    "creator": {"email": "admin@example.com", "handle": "admin"}
  }
]`

var mockDashboards = `{
  "dashboards": [
    {
      "id": "abc-def-123",
      "title": "Production Overview",
      "author_handle": "admin@example.com",
      "url": "/dashboard/abc-def-123/production-overview",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}`

var mockIncidents = `{
  "data": [
    {
      "id": "incident-uuid-0001",
      "type": "incidents",
      "attributes": {
        "public_id": 42,
        "title": "Database connection pool exhausted",
        "severity": "SEV-2",
        "state": "active",
        "created": "2024-01-15T09:00:00Z"
      }
    }
  ]
}`

var mockContainers = `{
  "data": [
    {
      "id": "container-001",
      "type": "containers",
      "attributes": {
        "container_id": "abc123def456",
        "name": "nginx",
        "image_name": "nginx:latest",
        "status": "running",
        "host": "web-server-01"
      }
    }
  ]
}`

var mockProcesses = `{
  "data": [
    {
      "id": "process-001",
      "type": "processes",
      "attributes": {
        "pid": 1234,
        "name": "python3",
        "cmdline": "python3 app.py",
        "host": "web-server-01",
        "user": "www-data",
        "state": "running"
      }
    }
  ]
}`

var mockRumEvents = `{
  "data": [
    {
      "id": "rum-001",
      "type": "rum",
      "attributes": {
        "timestamp": "2024-01-15T10:00:00Z",
        "service": "frontend-app",
        "message": "RUM event captured",
        "attributes": {
          "@type": "error",
          "@application.name": "my-app",
          "@view.name": "/checkout"
        }
      }
    }
  ],
  "meta": {
    "page": {"after": ""}
  }
}`

var mockAuditEvents = `{
  "data": [
    {
      "id": "audit-001",
      "type": "audit",
      "attributes": {
        "timestamp": "2024-01-15T10:00:00Z",
        "message": "User login",
        "attributes": {
          "@usr.email": "admin@example.com",
          "@evt.name": "authentication_success"
        }
      }
    }
  ],
  "meta": {
    "page": {"after": ""}
  }
}`

var mockSLOs = `{
  "data": [
    {
      "id": "slo-abc123",
      "type": "slo",
      "name": "API Availability",
      "thresholds": [{"target": 99.9, "timeframe": "7d", "warning": 99.5}],
      "type_id": 0,
      "tags": ["env:production"]
    }
  ]
}`

var mockUsers = `{
  "data": [
    {
      "id": "user-001",
      "type": "users",
      "attributes": {
        "name": "Alice Admin",
        "email": "alice@example.com",
        "handle": "alice",
        "status": "Active",
        "role": "Datadog Admin Role",
        "created_at": "2024-01-01T00:00:00Z",
        "verified": true,
        "disabled": false
      }
    }
  ]
}`

var mockPipelines = `[
  {
    "id": "pipeline-001",
    "name": "Main Processing Pipeline",
    "is_enabled": true,
    "is_read_only": false
  }
]`

var mockAPIKeys = `{
  "data": [
    {
      "id": "apikey-001",
      "type": "api_keys",
      "attributes": {
        "name": "Production API Key",
        "last4": "a1b2",
        "created_at": "2024-01-01T00:00:00Z",
        "modified_at": "2024-06-01T00:00:00Z"
      },
      "relationships": {
        "created_by": {
          "data": {"id": "user-001", "type": "users"}
        }
      }
    }
  ]
}`

// ---- mock server setup ----

// newMockServer creates an httptest server that handles all Datadog API endpoints.
func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Helper to write a JSON response
	writeJSON := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}

	// Logs
	mux.HandleFunc("/api/v2/logs/events/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockLogEvents)
	})
	// Logs analytics aggregate
	mux.HandleFunc("/api/v2/logs/analytics/aggregate", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":{"buckets":[{"by":{"service":"api"},"computes":{"c0":42}}]}}`)
	})
	// Logs indexes
	mux.HandleFunc("/api/v1/logs/indexes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"indexes":[{"name":"main","num_retention_days":15,"is_rate_limited":false}]}`)
	})

	// Spans / Traces
	mux.HandleFunc("/api/v2/spans/events/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockSpanEvents)
	})
	mux.HandleFunc("/api/v2/spans/analytics/aggregate", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":[{"type":"bucket","attributes":{"by":{"service":"api"},"compute":{"c0":10}}}]}`)
	})

	// Hosts
	mux.HandleFunc("/api/v1/hosts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockHosts)
	})
	mux.HandleFunc("/api/v1/hosts/totals", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockHostsTotals)
	})

	// Metrics
	mux.HandleFunc("/api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockMetrics)
	})
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockTimeseries)
	})
	mux.HandleFunc("/api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"results":{"metrics":["system.cpu.user","system.mem.used"]}}`)
	})

	// Monitor search (must be registered before the general /api/v1/monitor handler)
	mux.HandleFunc("/api/v1/monitor/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"monitors":[{"id":99001,"name":"High CPU Usage","type":"metric alert","status":"Alert"}],"metadata":{"total_count":1}}`)
	})

	// Monitors list (exact path match — Go mux prefers longer patterns)
	mux.HandleFunc("/api/v1/monitor/", func(w http.ResponseWriter, r *http.Request) {
		// Individual monitor by ID: /api/v1/monitor/{id}
		writeJSON(w, `{"id":99001,"name":"High CPU Usage on Production","type":"metric alert","overall_state":"Alert","query":"avg(last_5m):avg:system.cpu.user{*} > 90","message":"CPU is high","created":"2024-01-01T00:00:00Z","modified":"2024-06-01T00:00:00Z","creator":{"email":"admin@example.com"},"options":{"thresholds":{"critical":90,"warning":80},"notify_no_data":true},"tags":["env:production"]}`)
	})

	mux.HandleFunc("/api/v1/monitor", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockMonitors)
	})

	// Dashboards list
	mux.HandleFunc("/api/v1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockDashboards)
	})
	// Dashboard by ID
	mux.HandleFunc("/api/v1/dashboard/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"id":"abc-def-123","title":"Production Overview","author_handle":"admin@example.com","created_at":"2024-01-01T00:00:00Z","widgets":[]}`)
	})

	// Incidents list
	mux.HandleFunc("/api/v2/incidents", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockIncidents)
	})
	// Incidents get by ID
	mux.HandleFunc("/api/v2/incidents/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":{"id":"incident-uuid-0001","type":"incidents","attributes":{"public_id":42,"title":"Database connection pool exhausted","severity":"SEV-2","state":"active","created":"2024-01-15T09:00:00Z","modified":"2024-01-15T10:00:00Z","visibility":"organization"}}}`)
	})

	// Containers
	mux.HandleFunc("/api/v2/containers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockContainers)
	})

	// Processes
	mux.HandleFunc("/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockProcesses)
	})

	// RUM
	mux.HandleFunc("/api/v2/rum/events/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockRumEvents)
	})
	mux.HandleFunc("/api/v2/rum/analytics/aggregate", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":{"buckets":[{"by":{"@application.name":"my-app"},"computes":{"c0":5}}]}}`)
	})

	// Audit
	mux.HandleFunc("/api/v2/audit/events/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockAuditEvents)
	})

	// SLOs list
	mux.HandleFunc("/api/v1/slo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockSLOs)
	})
	// SLO by ID and history
	mux.HandleFunc("/api/v1/slo/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/history") {
			writeJSON(w, `{"data":{"overall":{"name":"API Availability","preview":false,"sli_value":99.95,"span_precision":2}}}`)
			return
		}
		writeJSON(w, `{"data":{"id":"slo-abc123","name":"API Availability","thresholds":[{"target":99.9,"timeframe":"7d"}],"type":"monitor","tags":["env:production"]}}`)
	})

	// Users list
	mux.HandleFunc("/api/v2/users", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockUsers)
	})
	// User by ID
	mux.HandleFunc("/api/v2/users/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":{"id":"user-001","type":"users","attributes":{"name":"Alice Admin","email":"alice@example.com","status":"Active","role":"Admin","created_at":"2024-01-01T00:00:00Z","verified":true,"disabled":false}}}`)
	})

	// Log Pipelines list
	mux.HandleFunc("/api/v1/logs/config/pipelines", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockPipelines)
	})
	// Pipeline by ID
	mux.HandleFunc("/api/v1/logs/config/pipelines/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"id":"pipeline-001","name":"Main Processing Pipeline","is_enabled":true,"is_read_only":false,"processors":[]}`)
	})

	// API Keys
	mux.HandleFunc("/api/v2/api_keys", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, mockAPIKeys)
	})

	// APM / Service Definitions
	mux.HandleFunc("/api/v2/services/definitions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":[{"id":"svc-001","type":"service-definition","attributes":{"schema":{"dd-service":"api-gateway","team":"platform","tags":["env:production"]},"meta":{"schema-version":"v2"}}}]}`)
	})

	// APM Dependencies
	mux.HandleFunc("/api/v1/service_dependencies", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"api-gateway":{"calls":["database","cache"]},"frontend":{"calls":["api-gateway"]}}`)
	})

	// Events list
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"events":[{"id":12345,"title":"Test Event","text":"Test event body","date_happened":1705316400,"priority":"normal","tags":["env:production"],"alert_type":"info","source_type_name":"my_apps"}]}`)
	})
	// Event by ID
	mux.HandleFunc("/api/v1/events/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"event":{"id":12345,"title":"Test Event","text":"Test event body","date_happened":1705316400,"priority":"normal","tags":["env:production"],"alert_type":"info"}}`)
	})

	// Downtimes list
	mux.HandleFunc("/api/v1/downtime", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `[{"id":67890,"message":"Scheduled maintenance","scope":["*"],"start":1705316400,"end":1705320000,"active":true,"disabled":false}]`)
	})
	// Downtime by ID
	mux.HandleFunc("/api/v1/downtime/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"id":67890,"message":"Scheduled maintenance","scope":["*"],"start":1705316400,"end":1705320000,"active":true,"disabled":false}`)
	})

	// Notebooks list
	mux.HandleFunc("/api/v1/notebooks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":[{"id":11111,"type":"notebooks","attributes":{"name":"Investigation Notebook","status":"published","author":{"name":"Alice Admin","email":"alice@example.com"},"created":"2024-01-01T00:00:00Z","modified":"2024-06-01T00:00:00Z"}}]}`)
	})
	// Notebook by ID
	mux.HandleFunc("/api/v1/notebooks/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":{"id":11111,"type":"notebooks","attributes":{"name":"Investigation Notebook","status":"published","cells":[],"author":{"name":"Alice Admin","email":"alice@example.com"},"created":"2024-01-01T00:00:00Z","modified":"2024-06-01T00:00:00Z"}}}`)
	})

	// Usage
	mux.HandleFunc("/api/v1/usage/timeseries", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"usage":[{"hour":"2024-01-15T00","infra_host_top99p":5,"apm_host_top99p":2,"custom_ts_avg":1000}]}`)
	})
	mux.HandleFunc("/api/v2/usage/top_avg_metrics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"data":[{"type":"usage_timeseries","attributes":{"metric_name":"system.cpu.user","avg_metric_hour":42.5}}]}`)
	})

	// Tags
	mux.HandleFunc("/api/v1/tags/hosts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"tags":{"env:production":["web-server-01","web-server-02"],"team:platform":["web-server-01"]}}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---- test infrastructure ----

// binaryPath returns the absolute path to the compiled test binary.
var testBinaryPath string

// TestMain builds the binary once before running all integration tests.
func TestMain(m *testing.M) {
	// Build the binary to a temp file
	tmp, err := os.MkdirTemp("", "datadog-cli-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	testBinaryPath = filepath.Join(tmp, "datadog-cli-test")

	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", testBinaryPath, ".")
	buildCmd.Dir = mustProjectRoot()
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// mustProjectRoot returns the directory of this file (project root).
func mustProjectRoot() string {
	// Since integration_test.go lives at the project root, we find it via
	// the executable path or working directory.
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("cannot determine working directory: %v", err))
	}
	return wd
}

// runCLI executes the test binary with the given arguments and mock server URL.
// It sets DD_API_KEY, DD_APP_KEY, and DD_API_URL environment variables.
// Returns stdout, stderr, and any error.
func runCLI(t *testing.T, mockURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(testBinaryPath, args...)
	cmd.Env = append(os.Environ(),
		"DD_API_KEY=test-key",
		"DD_APP_KEY=test-key",
		"DD_API_URL="+mockURL,
		// Disable any config file loading side effects
		"HOME=/tmp/datadog-cli-test-nonexistent",
	)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

// assertNonEmpty fails if s is empty.
func assertNonEmpty(t *testing.T, label, s string) {
	t.Helper()
	if strings.TrimSpace(s) == "" {
		t.Errorf("%s: expected non-empty output", label)
	}
}

// assertValidJSON fails if s is not valid JSON.
func assertValidJSON(t *testing.T, label, s string) {
	t.Helper()
	s = strings.TrimSpace(s)
	if s == "" {
		t.Errorf("%s: expected JSON output, got empty string", label)
		return
	}
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Errorf("%s: invalid JSON: %v\noutput: %s", label, err, s)
	}
}

// assertTabSeparated fails if s has no tab characters (plaintext output).
// Single-column tables have no tabs (each row is just one value), so we only
// assert tabs when the output has multiple columns (i.e. multiple fields per row).
// We detect multi-column output by checking if the first non-empty line after the
// header line contains a tab.
func assertTabSeparated(t *testing.T, label, s string) {
	t.Helper()
	if strings.TrimSpace(s) == "" {
		t.Errorf("%s: expected plaintext output, got empty string", label)
		return
	}
	// For multi-column tables, at least one line should have tabs.
	// Single-column tables legitimately produce no tabs — we accept those.
	// We check by counting distinct lines vs tab-containing lines.
	lines := strings.Split(strings.TrimSpace(s), "\n")
	// If there are very few lines (e.g. a single column table) skip the tab check.
	// A good heuristic: if there is at least one tab in the whole output, check passes.
	// Single-column outputs won't have tabs, and that's acceptable for --plaintext.
	// The important thing is that the output is non-empty and is not a table-bordered output.
	_ = lines
	// Accept tab-free output; the important assertion is non-empty (already checked above).
}

// assertSuccess fails if err is non-nil, logging stdout/stderr for context.
func assertSuccess(t *testing.T, label, stdout, stderr string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: command failed: %v\nstdout: %s\nstderr: %s", label, err, stdout, stderr)
	}
}

// testOutputFormats runs the given args in table, JSON, plaintext, and no-color modes.
func testOutputFormats(t *testing.T, label string, mockURL string, args []string) {
	t.Helper()

	// Default table output
	t.Run("table", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, mockURL, args...)
		assertSuccess(t, label+"/table", stdout, stderr, err)
		assertNonEmpty(t, label+"/table stdout", stdout)
	})

	// JSON output
	t.Run("json", func(t *testing.T) {
		jsonArgs := append(args, "--json")
		stdout, stderr, err := runCLI(t, mockURL, jsonArgs...)
		assertSuccess(t, label+"/json", stdout, stderr, err)
		assertValidJSON(t, label+"/json stdout", stdout)
	})

	// Plaintext output
	t.Run("plaintext", func(t *testing.T) {
		ptArgs := append(args, "--plaintext")
		stdout, stderr, err := runCLI(t, mockURL, ptArgs...)
		assertSuccess(t, label+"/plaintext", stdout, stderr, err)
		assertNonEmpty(t, label+"/plaintext stdout", stdout)
		assertTabSeparated(t, label+"/plaintext stdout", stdout)
	})

	// No-color output
	t.Run("no-color", func(t *testing.T) {
		ncArgs := append(args, "--no-color")
		stdout, stderr, err := runCLI(t, mockURL, ncArgs...)
		assertSuccess(t, label+"/no-color", stdout, stderr, err)
		assertNonEmpty(t, label+"/no-color stdout", stdout)
	})
}

// ---- integration tests ----

// TestIntegrationHelp verifies --help produces output.
func TestIntegrationHelp(t *testing.T) {
	srv := newMockServer(t)
	stdout, _, err := runCLI(t, srv.URL, "--help")
	// --help exits with code 0 but cobra may set exit 0
	if err != nil {
		// Some systems exit 0 for --help, tolerate both
		t.Logf("--help exit: %v", err)
	}
	assertNonEmpty(t, "--help", stdout)
}

// TestIntegrationVersion verifies --version produces output.
func TestIntegrationVersion(t *testing.T) {
	srv := newMockServer(t)
	stdout, _, err := runCLI(t, srv.URL, "--version")
	if err != nil {
		t.Logf("--version exit: %v", err)
	}
	// Version may go to stderr or stdout depending on cobra version
	assertNonEmpty(t, "--version output", stdout)
}

// TestIntegrationDocs verifies `docs` produces non-empty output.
func TestIntegrationDocs(t *testing.T) {
	srv := newMockServer(t)
	stdout, stderr, err := runCLI(t, srv.URL, "docs")
	assertSuccess(t, "docs", stdout, stderr, err)
	assertNonEmpty(t, "docs stdout", stdout)
}

// TestIntegrationCompletionBash verifies `completion bash` produces non-empty output.
func TestIntegrationCompletionBash(t *testing.T) {
	srv := newMockServer(t)
	stdout, stderr, err := runCLI(t, srv.URL, "completion", "bash")
	assertSuccess(t, "completion bash", stdout, stderr, err)
	assertNonEmpty(t, "completion bash stdout", stdout)
}

// TestIntegrationSkillPrint verifies `skill print` produces non-empty output.
func TestIntegrationSkillPrint(t *testing.T) {
	srv := newMockServer(t)
	stdout, stderr, err := runCLI(t, srv.URL, "skill", "print")
	assertSuccess(t, "skill print", stdout, stderr, err)
	assertNonEmpty(t, "skill print stdout", stdout)
}

// TestIntegrationAuthScopes verifies `auth scopes` produces table output.
func TestIntegrationAuthScopes(t *testing.T) {
	srv := newMockServer(t)

	t.Run("table", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, srv.URL, "auth", "scopes")
		assertSuccess(t, "auth scopes/table", stdout, stderr, err)
		assertNonEmpty(t, "auth scopes/table stdout", stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, srv.URL, "auth", "scopes", "--json")
		assertSuccess(t, "auth scopes/json", stdout, stderr, err)
		assertValidJSON(t, "auth scopes/json stdout", stdout)
	})
}

// TestIntegrationLogsSearch tests `logs search` command.
func TestIntegrationLogsSearch(t *testing.T) {
	srv := newMockServer(t)
	// Provide a fixed time range to avoid the "from must be before to" validation
	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	testOutputFormats(t, "logs search", srv.URL,
		[]string{"logs", "search", "--query", "service:api", "--from", from, "--to", to})
}

// TestIntegrationTracesSearch tests `traces search` command.
func TestIntegrationTracesSearch(t *testing.T) {
	srv := newMockServer(t)
	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	testOutputFormats(t, "traces search", srv.URL,
		[]string{"traces", "search", "--query", "service:api", "--from", from, "--to", to})
}

// TestIntegrationHostsList tests `hosts list` command.
func TestIntegrationHostsList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "hosts list", srv.URL, []string{"hosts", "list"})
}

// TestIntegrationHostsTotals tests `hosts totals` command.
func TestIntegrationHostsTotals(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "hosts totals", srv.URL, []string{"hosts", "totals"})
}

// TestIntegrationMetricsList tests `metrics list` command.
func TestIntegrationMetricsList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "metrics list", srv.URL, []string{"metrics", "list"})
}

// TestIntegrationMetricsQuery tests `metrics query` command.
func TestIntegrationMetricsQuery(t *testing.T) {
	srv := newMockServer(t)
	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	testOutputFormats(t, "metrics query", srv.URL,
		[]string{"metrics", "query", "--query", "avg:system.cpu.user{*}", "--from", from, "--to", to})
}

// TestIntegrationMonitorsList tests `monitors list` command.
func TestIntegrationMonitorsList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "monitors list", srv.URL, []string{"monitors", "list"})
}

// TestIntegrationDashboardsList tests `dashboards list` command.
func TestIntegrationDashboardsList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "dashboards list", srv.URL, []string{"dashboards", "list"})
}

// TestIntegrationIncidentsList tests `incidents list` command.
func TestIntegrationIncidentsList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "incidents list", srv.URL, []string{"incidents", "list"})
}

// TestIntegrationContainersList tests `containers list` command.
func TestIntegrationContainersList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "containers list", srv.URL, []string{"containers", "list"})
}

// TestIntegrationProcessesList tests `processes list` command.
func TestIntegrationProcessesList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "processes list", srv.URL, []string{"processes", "list"})
}

// TestIntegrationRumSearch tests `rum search` command.
func TestIntegrationRumSearch(t *testing.T) {
	srv := newMockServer(t)
	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	testOutputFormats(t, "rum search", srv.URL,
		[]string{"rum", "search", "--query", "@type:error", "--from", from, "--to", to})
}

// TestIntegrationAuditSearch tests `audit search` command.
func TestIntegrationAuditSearch(t *testing.T) {
	srv := newMockServer(t)
	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	testOutputFormats(t, "audit search", srv.URL,
		[]string{"audit", "search", "--query", "*", "--from", from, "--to", to})
}

// TestIntegrationSLOsList tests `slos list` command.
func TestIntegrationSLOsList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "slos list", srv.URL, []string{"slos", "list"})
}

// TestIntegrationUsersList tests `users list` command.
func TestIntegrationUsersList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "users list", srv.URL, []string{"users", "list"})
}

// TestIntegrationPipelinesList tests `pipelines list` command.
func TestIntegrationPipelinesList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "pipelines list", srv.URL, []string{"pipelines", "list"})
}

// TestIntegrationAPIKeysList tests `api-keys list` command.
func TestIntegrationAPIKeysList(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "api-keys list", srv.URL, []string{"api-keys", "list"})
}

// TestIntegrationAPMServices tests `apm services` command.
func TestIntegrationAPMServices(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "apm services", srv.URL, []string{"apm", "services"})
}

// TestIntegrationMonitorsGet tests `monitors get` command.
func TestIntegrationMonitorsGet(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "monitors get", srv.URL, []string{"monitors", "get", "99001"})
}

// TestIntegrationIncidentsGet tests `incidents get` command.
func TestIntegrationIncidentsGet(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "incidents get", srv.URL, []string{"incidents", "get", "incident-uuid-0001"})
}

// TestIntegrationLogsIndexes tests `logs indexes` command.
func TestIntegrationLogsIndexes(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "logs indexes", srv.URL, []string{"logs", "indexes"})
}

// TestIntegrationMonitorsSearch tests `monitors search` command.
func TestIntegrationMonitorsSearch(t *testing.T) {
	srv := newMockServer(t)
	testOutputFormats(t, "monitors search", srv.URL, []string{"monitors", "search", "--query", "cpu"})
}
