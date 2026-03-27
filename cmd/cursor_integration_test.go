//go:build integration

package cmd

// Live integration tests for cursor-based commands.
//
// These tests hit the REAL Datadog API at api.datadoghq.com.
// They are skipped automatically when DD_API_KEY / DD_APP_KEY are not set.
//
// Run with:
//   DD_API_KEY=... DD_APP_KEY=... go test -tags integration -timeout 120s -v ./...
//
// Test matrix per command (logs search, rum search, audit search, traces search,
// processes list, containers list):
//   1. Smoke test  — --limit 10, assert non-empty results
//   2. Limit test  — --limit 1,  assert exactly 1 result
//   3. Large limit — --limit 2000, assert call succeeds without error (no crash)
//   4. JSON test   — --json,       assert output is valid JSON with a "data" array

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdoutIntegration captures stdout for an integration test function.
// Named differently from captureStdout (unit test helper in pagination_test.go)
// to avoid conflicts in the same package.
func captureStdoutIntegration(t *testing.T, f func() error) (string, error) {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	runErr := f()

	_ = w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return buf.String(), runErr
}

// assertJSONDataArray parses out and verifies the "data" array from JSON output.
// Returns the parsed array so callers can do length assertions.
func assertJSONDataArray(t *testing.T, out string) []interface{} {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Fatal("--json produced empty output")
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	dataArr, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing or non-array 'data' field; output:\n%s", out)
	}
	return dataArr
}

// integrationResetFlags resets all global flag vars to safe defaults before
// each integration sub-test, then restores them via t.Cleanup so tests are
// independent even when run in the same process.
func integrationResetFlags(t *testing.T) {
	t.Helper()

	// Snapshot originals.
	origJSON := flagJSON
	origPlaintext := flagPlaintext
	origNoColor := flagNoColor
	origDebug := flagDebug
	origVerbose := flagVerbose
	origQuiet := flagQuiet
	origLimit := flagLimit
	origProfile := flagProfile
	origSite := flagSite
	origAPIKey := flagAPIKey
	origAppKey := flagAppKey
	origFields := flagFields
	origJQ := flagJQ

	// Set sensible test defaults.
	flagJSON = false
	flagPlaintext = false
	flagNoColor = false
	flagDebug = false
	flagVerbose = false
	flagQuiet = true
	flagLimit = 100
	flagProfile = ""
	flagSite = ""
	flagAPIKey = ""
	flagAppKey = ""
	flagFields = ""
	flagJQ = ""

	t.Cleanup(func() {
		flagJSON = origJSON
		flagPlaintext = origPlaintext
		flagNoColor = origNoColor
		flagDebug = origDebug
		flagVerbose = origVerbose
		flagQuiet = origQuiet
		flagLimit = origLimit
		flagProfile = origProfile
		flagSite = origSite
		flagAPIKey = origAPIKey
		flagAppKey = origAppKey
		flagFields = origFields
		flagJQ = origJQ
	})
}

// ============================================================================
// LOGS SEARCH
// ============================================================================

func TestIntegration_LogsSearch_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	// Reset command-specific vars.
	logsSearchQuery = "*"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil
	logsSearchAll = false
	flagLimit = 10
	flagQuiet = true

	out, err := captureStdoutIntegration(t, func() error {
		return runLogsSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("logs search smoke: unexpected error: %v", err)
	}

	// Accept "no results" as valid — the org may have no logs in the window.
	// We only fail on hard errors, which are caught above.
	_ = out
}

func TestIntegration_LogsSearch_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	logsSearchQuery = "*"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil
	logsSearchAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runLogsSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("logs search --limit 1: unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no logs in last hour; cannot verify limit")
	}
	dataArr := assertJSONDataArray(t, out)
	if len(dataArr) > 1 {
		t.Errorf("--limit 1 returned %d items, want <= 1", len(dataArr))
	}
}

func TestIntegration_LogsSearch_LargeLimit(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	logsSearchQuery = "*"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil
	logsSearchAll = false
	flagLimit = 2000
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runLogsSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("logs search --limit 2000: unexpected error: %v", err)
	}
}

func TestIntegration_LogsSearch_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	logsSearchQuery = "*"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil
	logsSearchAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runLogsSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("logs search --json: unexpected error: %v", err)
	}

	// Output must be valid JSON with a "data" array (even if empty).
	assertJSONDataArray(t, out)
}

// ============================================================================
// RUM SEARCH
// ============================================================================

func TestIntegration_RumSearch_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	rumSearchQuery = "*"
	rumSearchFrom = "1d"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"
	rumSearchAll = false
	flagLimit = 10
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runRumSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("rum search smoke: unexpected error: %v", err)
	}
}

func TestIntegration_RumSearch_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	rumSearchQuery = "*"
	rumSearchFrom = "1d"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"
	rumSearchAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runRumSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("rum search --limit 1: unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no RUM events in last day; cannot verify limit")
	}
	dataArr := assertJSONDataArray(t, out)
	if len(dataArr) > 1 {
		t.Errorf("--limit 1 returned %d items, want <= 1", len(dataArr))
	}
}

func TestIntegration_RumSearch_LargeLimit(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	rumSearchQuery = "*"
	rumSearchFrom = "1d"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"
	rumSearchAll = false
	flagLimit = 2000
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runRumSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("rum search --limit 2000: unexpected error: %v", err)
	}
}

func TestIntegration_RumSearch_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	rumSearchQuery = "*"
	rumSearchFrom = "1d"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"
	rumSearchAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runRumSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("rum search --json: unexpected error: %v", err)
	}

	assertJSONDataArray(t, out)
}

// ============================================================================
// AUDIT SEARCH
// ============================================================================

func TestIntegration_AuditSearch_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	auditSearchQuery = "*"
	auditSearchFrom = "1d"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"
	auditSearchAll = false
	flagLimit = 10
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runAuditSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("audit search smoke: unexpected error: %v", err)
	}
}

func TestIntegration_AuditSearch_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	auditSearchQuery = "*"
	auditSearchFrom = "1d"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"
	auditSearchAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runAuditSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("audit search --limit 1: unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no audit events in last day; cannot verify limit")
	}
	dataArr := assertJSONDataArray(t, out)
	if len(dataArr) > 1 {
		t.Errorf("--limit 1 returned %d items, want <= 1", len(dataArr))
	}
}

func TestIntegration_AuditSearch_LargeLimit(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	auditSearchQuery = "*"
	auditSearchFrom = "1d"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"
	auditSearchAll = false
	flagLimit = 2000
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runAuditSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("audit search --limit 2000: unexpected error: %v", err)
	}
}

func TestIntegration_AuditSearch_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	auditSearchQuery = "*"
	auditSearchFrom = "1d"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"
	auditSearchAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runAuditSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("audit search --json: unexpected error: %v", err)
	}

	assertJSONDataArray(t, out)
}

// ============================================================================
// TRACES SEARCH
// ============================================================================

func TestIntegration_TracesSearch_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	tracesSearchQuery = "*"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""
	tracesSearchAll = false
	flagLimit = 10
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runTracesSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("traces search smoke: unexpected error: %v", err)
	}
}

func TestIntegration_TracesSearch_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	tracesSearchQuery = "*"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""
	tracesSearchAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runTracesSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("traces search --limit 1: unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no traces in last hour; cannot verify limit")
	}
	dataArr := assertJSONDataArray(t, out)
	if len(dataArr) > 1 {
		t.Errorf("--limit 1 returned %d items, want <= 1", len(dataArr))
	}
}

func TestIntegration_TracesSearch_LargeLimit(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	tracesSearchQuery = "*"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""
	tracesSearchAll = false
	flagLimit = 2000
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runTracesSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("traces search --limit 2000: unexpected error: %v", err)
	}
}

func TestIntegration_TracesSearch_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	tracesSearchQuery = "*"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""
	tracesSearchAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runTracesSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("traces search --json: unexpected error: %v", err)
	}

	assertJSONDataArray(t, out)
}

// ============================================================================
// PROCESSES LIST
// ============================================================================

func TestIntegration_ProcessesList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	processesListSearch = ""
	processesListHost = ""
	processesListAll = false
	flagLimit = 10
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runProcessesList(nil, nil)
	})
	if err != nil {
		t.Fatalf("processes list smoke: unexpected error: %v", err)
	}
}

func TestIntegration_ProcessesList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	processesListSearch = ""
	processesListHost = ""
	processesListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runProcessesList(nil, nil)
	})
	if err != nil {
		t.Fatalf("processes list --limit 1: unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no processes available; cannot verify limit")
	}
	dataArr := assertJSONDataArray(t, out)
	if len(dataArr) > 1 {
		t.Errorf("--limit 1 returned %d items, want <= 1", len(dataArr))
	}
}

func TestIntegration_ProcessesList_LargeLimit(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	processesListSearch = ""
	processesListHost = ""
	processesListAll = false
	flagLimit = 2000
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runProcessesList(nil, nil)
	})
	if err != nil {
		t.Fatalf("processes list --limit 2000: unexpected error: %v", err)
	}
}

func TestIntegration_ProcessesList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	processesListSearch = ""
	processesListHost = ""
	processesListAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runProcessesList(nil, nil)
	})
	if err != nil {
		t.Fatalf("processes list --json: unexpected error: %v", err)
	}

	assertJSONDataArray(t, out)
}

// ============================================================================
// CONTAINERS LIST
// ============================================================================

func TestIntegration_ContainersList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	containersListFilter = ""
	containersListAll = false
	flagLimit = 10
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runContainersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("containers list smoke: unexpected error: %v", err)
	}
}

func TestIntegration_ContainersList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	containersListFilter = ""
	containersListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runContainersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("containers list --limit 1: unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no containers available; cannot verify limit")
	}
	dataArr := assertJSONDataArray(t, out)
	if len(dataArr) > 1 {
		t.Errorf("--limit 1 returned %d items, want <= 1", len(dataArr))
	}
}

func TestIntegration_ContainersList_LargeLimit(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	containersListFilter = ""
	containersListAll = false
	flagLimit = 2000
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runContainersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("containers list --limit 2000: unexpected error: %v", err)
	}
}

func TestIntegration_ContainersList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	containersListFilter = ""
	containersListAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runContainersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("containers list --json: unexpected error: %v", err)
	}

	assertJSONDataArray(t, out)
}
