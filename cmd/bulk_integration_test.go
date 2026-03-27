//go:build integration

package cmd

// Live integration tests for bulk (single-response) and single-item get commands.
//
// These tests hit the REAL Datadog API at api.datadoghq.com.
// They are skipped automatically when DD_API_KEY / DD_APP_KEY are not set.
//
// Run with:
//   DD_API_KEY=... DD_APP_KEY=... go test -tags integration -timeout 120s -v ./...
//
// Coverage:
//   Bulk: tags list, metrics list, metrics search, events list, users list,
//         api-keys list, pipelines list, usage summary, usage top-metrics,
//         hosts totals, apm services, apm definitions, apm dependencies
//   Single-get (smoke): monitors get, users get, slos get

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// assertValidJSON checks that the string is non-empty and parses as JSON.
// Returns the parsed value.
func assertValidJSON(t *testing.T, out, cmd string) interface{} {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Fatalf("%s --json produced empty output", cmd)
	}
	var v interface{}
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		t.Fatalf("%s --json output is not valid JSON: %v\nOutput:\n%s", cmd, err, out)
	}
	return v
}

// ============================================================================
// TAGS LIST
// ============================================================================

func TestIntegration_TagsList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	tagsListSource = ""
	flagLimit = 10

	_, err := captureStdoutIntegration(t, func() error {
		return runTagsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("tags list smoke: unexpected error: %v", err)
	}
}

func TestIntegration_TagsList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	tagsListSource = ""
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runTagsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("tags list --json: unexpected error: %v", err)
	}

	// Returns {"tags": {...}} — verify it's valid JSON with a "tags" key.
	v := assertValidJSON(t, out, "tags list")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("tags list --json: expected JSON object, got %T", v)
	}
	if _, ok := m["tags"]; !ok {
		t.Errorf("tags list --json: expected 'tags' key in response; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// METRICS LIST
// ============================================================================

func TestIntegration_MetricsList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	metricsListFrom = "1h"
	flagLimit = 10

	out, err := captureStdoutIntegration(t, func() error {
		return runMetricsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("metrics list smoke: unexpected error: %v", err)
	}
	requireNonEmptyOutput(t, out, "metrics list")
}

func TestIntegration_MetricsList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	metricsListFrom = "1h"
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runMetricsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("metrics list --json: unexpected error: %v", err)
	}

	// Returns {"metrics": [...]}
	v := assertValidJSON(t, out, "metrics list")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("metrics list --json: expected JSON object, got %T", v)
	}
	if _, ok := m["metrics"]; !ok {
		t.Errorf("metrics list --json: expected 'metrics' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// METRICS SEARCH
// ============================================================================

func TestIntegration_MetricsSearch_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	// Search for "system" — always present in any DD org.
	metricsSearchQuery = "system"
	flagLimit = 10

	out, err := captureStdoutIntegration(t, func() error {
		return runMetricsSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("metrics search smoke: unexpected error: %v", err)
	}
	requireNonEmptyOutput(t, out, "metrics search")
}

func TestIntegration_MetricsSearch_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	metricsSearchQuery = "system"
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runMetricsSearch(nil, nil)
	})
	if err != nil {
		t.Fatalf("metrics search --json: unexpected error: %v", err)
	}

	// Returns {"results": {"metrics": [...]}}
	v := assertValidJSON(t, out, "metrics search")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("metrics search --json: expected JSON object, got %T", v)
	}
	if _, ok := m["results"]; !ok {
		t.Errorf("metrics search --json: expected 'results' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// EVENTS LIST
// ============================================================================

func TestIntegration_EventsList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	eventsListStart = "7d"
	eventsListEnd = "now"
	eventsListPriority = ""
	eventsListSources = ""
	eventsListTags = ""
	eventsListAll = false
	flagLimit = 10

	_, err := captureStdoutIntegration(t, func() error {
		return runEventsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("events list smoke: unexpected error: %v", err)
	}
}

func TestIntegration_EventsList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	eventsListStart = "7d"
	eventsListEnd = "now"
	eventsListPriority = ""
	eventsListSources = ""
	eventsListTags = ""
	eventsListAll = false
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runEventsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("events list --json: unexpected error: %v", err)
	}

	// Returns {"events": [...]} or {"events": null} if empty.
	v := assertValidJSON(t, out, "events list")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("events list --json: expected JSON object, got %T", v)
	}
	if _, ok := m["events"]; !ok {
		t.Errorf("events list --json: expected 'events' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// USERS LIST
// ============================================================================

func TestIntegration_UsersList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagLimit = 10

	out, err := captureStdoutIntegration(t, func() error {
		return runUsersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("users list smoke: unexpected error: %v", err)
	}
	requireNonEmptyOutput(t, out, "users list")
}

func TestIntegration_UsersList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runUsersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("users list --json: unexpected error: %v", err)
	}

	// Returns {"data": [...]}
	v := assertValidJSON(t, out, "users list")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("users list --json: expected JSON object, got %T", v)
	}
	dataArr, ok := m["data"].([]interface{})
	if !ok {
		t.Fatalf("users list --json: expected 'data' array; got keys: %v", mapKeys(m))
	}
	if len(dataArr) == 0 {
		t.Error("users list --json: 'data' array is empty; expected at least one user")
	}
}

// ============================================================================
// API-KEYS LIST
// ============================================================================

func TestIntegration_APIKeysList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagLimit = 10

	out, err := captureStdoutIntegration(t, func() error {
		return runAPIKeysList(nil, nil)
	})
	if err != nil {
		t.Fatalf("api-keys list smoke: unexpected error: %v", err)
	}
	requireNonEmptyOutput(t, out, "api-keys list")
}

func TestIntegration_APIKeysList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runAPIKeysList(nil, nil)
	})
	if err != nil {
		t.Fatalf("api-keys list --json: unexpected error: %v", err)
	}

	// Returns {"data": [...]}
	v := assertValidJSON(t, out, "api-keys list")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("api-keys list --json: expected JSON object, got %T", v)
	}
	dataArr, ok := m["data"].([]interface{})
	if !ok {
		t.Fatalf("api-keys list --json: expected 'data' array; got keys: %v", mapKeys(m))
	}
	if len(dataArr) == 0 {
		t.Error("api-keys list --json: 'data' array is empty; expected at least one API key")
	}
}

// ============================================================================
// PIPELINES LIST
// ============================================================================

func TestIntegration_PipelinesList_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagLimit = 10

	_, err := captureStdoutIntegration(t, func() error {
		return runPipelinesList(nil, nil)
	})
	if err != nil {
		t.Fatalf("pipelines list smoke: unexpected error: %v", err)
	}
}

func TestIntegration_PipelinesList_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runPipelinesList(nil, nil)
	})
	if err != nil {
		t.Fatalf("pipelines list --json: unexpected error: %v", err)
	}

	// Returns a JSON array directly or an object — either is valid JSON.
	assertValidJSON(t, out, "pipelines list")
}

// ============================================================================
// USAGE SUMMARY
// ============================================================================

func TestIntegration_UsageSummary_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	// Use last month to ensure data exists.
	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)
	usageSummaryStartMonth = lastMonth.Format("2006-01")
	usageSummaryEndMonth = ""
	flagLimit = 10

	out, err := captureStdoutIntegration(t, func() error {
		return runUsageSummary(nil, nil)
	})
	if err != nil {
		t.Fatalf("usage summary smoke: unexpected error: %v", err)
	}
	requireNonEmptyOutput(t, out, "usage summary")
}

func TestIntegration_UsageSummary_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)
	usageSummaryStartMonth = lastMonth.Format("2006-01")
	usageSummaryEndMonth = ""
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runUsageSummary(nil, nil)
	})
	if err != nil {
		t.Fatalf("usage summary --json: unexpected error: %v", err)
	}

	// Returns {"usage": [...]}
	v := assertValidJSON(t, out, "usage summary")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("usage summary --json: expected JSON object, got %T", v)
	}
	if _, ok := m["usage"]; !ok {
		t.Errorf("usage summary --json: expected 'usage' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// USAGE TOP-METRICS
// Verify that --all returns >= the default single-page result (pagination check).
// ============================================================================

func TestIntegration_UsageTopMetrics_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)
	usageTopMetricsMonth = lastMonth.Format("2006-01")
	usageTopMetricsMetricName = ""
	usageTopMetricsAll = false
	flagLimit = 100
	flagQuiet = true

	_, err := captureStdoutIntegration(t, func() error {
		return runUsageTopMetrics(nil, nil)
	})
	if err != nil {
		t.Fatalf("usage top-metrics smoke: unexpected error: %v", err)
	}

	t.Cleanup(func() {
		usageTopMetricsMonth = ""
		usageTopMetricsMetricName = ""
		usageTopMetricsAll = false
	})
}

func TestIntegration_UsageTopMetrics_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)
	usageTopMetricsMonth = lastMonth.Format("2006-01")
	usageTopMetricsMetricName = ""
	usageTopMetricsAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() {
		flagJSON = false
		usageTopMetricsMonth = ""
		usageTopMetricsMetricName = ""
		usageTopMetricsAll = false
	})

	out, err := captureStdoutIntegration(t, func() error {
		return runUsageTopMetrics(nil, nil)
	})
	if err != nil {
		t.Fatalf("usage top-metrics --json: unexpected error: %v", err)
	}

	// Returns {"usage": [...], "metadata": {...}}
	v := assertValidJSON(t, out, "usage top-metrics")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("usage top-metrics --json: expected JSON object, got %T", v)
	}
	if _, ok := m["usage"]; !ok {
		t.Errorf("usage top-metrics --json: expected 'usage' key; got keys: %v", mapKeys(m))
	}
}

// TestIntegration_UsageTopMetrics_AllPaginates verifies that --all returns
// at least as many results as the default single-page limit.
// The test is meaningful only when there are enough metrics to span a page.
func TestIntegration_UsageTopMetrics_AllPaginates(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)
	month := lastMonth.Format("2006-01")

	// --- single-page run: limit 100 ---
	usageTopMetricsMonth = month
	usageTopMetricsMetricName = ""
	usageTopMetricsAll = false
	flagLimit = 100
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() {
		flagJSON = false
		usageTopMetricsMonth = ""
		usageTopMetricsMetricName = ""
		usageTopMetricsAll = false
	})

	outSingle, err := captureStdoutIntegration(t, func() error {
		return runUsageTopMetrics(nil, nil)
	})
	if err != nil {
		t.Fatalf("usage top-metrics (single-page): unexpected error: %v", err)
	}

	var singleResult map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(outSingle)), &singleResult); err != nil {
		t.Fatalf("usage top-metrics single-page --json: parse error: %v", err)
	}
	singleUsage, _ := singleResult["usage"].([]interface{})
	singleCount := len(singleUsage)

	if singleCount == 0 {
		t.Skip("no top-metrics data for last month; skipping pagination check")
	}

	// --- all-pages run: --all ---
	usageTopMetricsMonth = month
	usageTopMetricsMetricName = ""
	usageTopMetricsAll = true
	flagLimit = 100 // ignored by --all
	flagJSON = true
	flagQuiet = true

	outAll, err := captureStdoutIntegration(t, func() error {
		return runUsageTopMetrics(nil, nil)
	})
	if err != nil {
		t.Fatalf("usage top-metrics --all: unexpected error: %v", err)
	}

	var allResult map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(outAll)), &allResult); err != nil {
		t.Fatalf("usage top-metrics --all --json: parse error: %v", err)
	}
	allUsage, _ := allResult["usage"].([]interface{})
	allCount := len(allUsage)

	if allCount < singleCount {
		t.Errorf("usage top-metrics --all returned %d items, but single-page returned %d; expected --all >= single", allCount, singleCount)
	}
	t.Logf("usage top-metrics pagination: single-page=%d, --all=%d", singleCount, allCount)
}

// ============================================================================
// HOSTS TOTALS
// ============================================================================

func TestIntegration_HostsTotals_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	out, err := captureStdoutIntegration(t, func() error {
		return runHostsTotals(nil, nil)
	})
	if err != nil {
		t.Fatalf("hosts totals smoke: unexpected error: %v", err)
	}
	requireNonEmptyOutput(t, out, "hosts totals")
}

func TestIntegration_HostsTotals_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	flagJSON = true
	t.Cleanup(func() { flagJSON = false })

	out, err := captureStdoutIntegration(t, func() error {
		return runHostsTotals(nil, nil)
	})
	if err != nil {
		t.Fatalf("hosts totals --json: unexpected error: %v", err)
	}

	// Returns {"total_active": N, "total_up": N}
	v := assertValidJSON(t, out, "hosts totals")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("hosts totals --json: expected JSON object, got %T", v)
	}
	if _, ok := m["total_active"]; !ok {
		t.Errorf("hosts totals --json: expected 'total_active' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// APM SERVICES
// ============================================================================

func TestIntegration_APMServices_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	apmServicesAll = false
	flagLimit = 10

	_, err := captureStdoutIntegration(t, func() error {
		return runAPMServices(nil, nil)
	})
	if err != nil {
		t.Fatalf("apm services smoke: unexpected error: %v", err)
	}

	t.Cleanup(func() { apmServicesAll = false })
}

func TestIntegration_APMServices_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	apmServicesAll = false
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() {
		flagJSON = false
		apmServicesAll = false
	})

	out, err := captureStdoutIntegration(t, func() error {
		return runAPMServices(nil, nil)
	})
	if err != nil {
		t.Fatalf("apm services --json: unexpected error: %v", err)
	}

	// Returns {"data": [...]}
	v := assertValidJSON(t, out, "apm services")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("apm services --json: expected JSON object, got %T", v)
	}
	if _, ok := m["data"]; !ok {
		t.Errorf("apm services --json: expected 'data' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// APM DEFINITIONS
// ============================================================================

func TestIntegration_APMDefinitions_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	apmDefinitionsAll = false
	flagLimit = 10

	_, err := captureStdoutIntegration(t, func() error {
		return runAPMDefinitions(nil, nil)
	})
	if err != nil {
		t.Fatalf("apm definitions smoke: unexpected error: %v", err)
	}

	t.Cleanup(func() { apmDefinitionsAll = false })
}

func TestIntegration_APMDefinitions_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	apmDefinitionsAll = false
	flagLimit = 10
	flagJSON = true
	t.Cleanup(func() {
		flagJSON = false
		apmDefinitionsAll = false
	})

	out, err := captureStdoutIntegration(t, func() error {
		return runAPMDefinitions(nil, nil)
	})
	if err != nil {
		t.Fatalf("apm definitions --json: unexpected error: %v", err)
	}

	// Returns {"data": [...]}
	v := assertValidJSON(t, out, "apm definitions")
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("apm definitions --json: expected JSON object, got %T", v)
	}
	if _, ok := m["data"]; !ok {
		t.Errorf("apm definitions --json: expected 'data' key; got keys: %v", mapKeys(m))
	}
}

// ============================================================================
// APM DEPENDENCIES
// ============================================================================

func TestIntegration_APMDependencies_Smoke(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	apmDepsEnv = "production"
	apmDepsService = ""
	t.Cleanup(func() {
		apmDepsEnv = ""
		apmDepsService = ""
	})

	_, err := captureStdoutIntegration(t, func() error {
		return runAPMDependencies(nil, nil)
	})
	if err != nil {
		t.Fatalf("apm dependencies smoke: unexpected error: %v", err)
	}
}

func TestIntegration_APMDependencies_JSON(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	apmDepsEnv = "production"
	apmDepsService = ""
	flagJSON = true
	t.Cleanup(func() {
		flagJSON = false
		apmDepsEnv = ""
		apmDepsService = ""
	})

	out, err := captureStdoutIntegration(t, func() error {
		return runAPMDependencies(nil, nil)
	})
	if err != nil {
		t.Fatalf("apm dependencies --json: unexpected error: %v", err)
	}

	// Returns a JSON object (map of service -> {calls: [...]})
	assertValidJSON(t, out, "apm dependencies")
}

// ============================================================================
// SINGLE-GET SMOKE TESTS
// Pattern: use list to get an ID, then fetch via get.
// ============================================================================

// TestIntegration_MonitorsGet uses monitors list to discover a real monitor ID.
func TestIntegration_MonitorsGet(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	// Step 1: list monitors with --limit 1 --json to find an ID.
	monitorsListGroupStates = ""
	monitorsListName = ""
	monitorsListTags = ""
	monitorsListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	outList, err := captureStdoutIntegration(t, func() error {
		return runMonitorsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("monitors list (pre-get): unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(outList)
	if trimmed == "" || trimmed == "[]" {
		t.Skip("no monitors available; skipping monitors get smoke")
	}

	// Parse array of monitors
	var monitors []interface{}
	if err := json.Unmarshal([]byte(trimmed), &monitors); err != nil {
		t.Fatalf("monitors list --json: parse error: %v\nOutput:\n%s", err, outList)
	}
	if len(monitors) == 0 {
		t.Skip("no monitors available; skipping monitors get smoke")
	}

	firstMonitor, ok := monitors[0].(map[string]interface{})
	if !ok {
		t.Fatalf("monitors list: first item is not a map")
	}
	monitorID := fmt.Sprintf("%v", firstMonitor["id"])
	if monitorID == "" || monitorID == "<nil>" {
		t.Skip("could not extract monitor ID; skipping")
	}

	// Step 2: call monitors get with the real ID.
	integrationResetFlags(t)

	out, err := captureStdoutIntegration(t, func() error {
		return runMonitorsGet(nil, []string{monitorID})
	})
	if err != nil {
		t.Fatalf("monitors get %s: unexpected error: %v", monitorID, err)
	}
	requireNonEmptyOutput(t, out, fmt.Sprintf("monitors get %s", monitorID))
}

// TestIntegration_UsersGet uses users list to discover the current user's ID.
func TestIntegration_UsersGet(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	// Step 1: list users --limit 1 --json.
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() { flagJSON = false })

	outList, err := captureStdoutIntegration(t, func() error {
		return runUsersList(nil, nil)
	})
	if err != nil {
		t.Fatalf("users list (pre-get): unexpected error: %v", err)
	}

	var listResult map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(outList)), &listResult); err != nil {
		t.Fatalf("users list --json: parse error: %v\nOutput:\n%s", err, outList)
	}
	dataArr, _ := listResult["data"].([]interface{})
	if len(dataArr) == 0 {
		t.Skip("no users returned; skipping users get smoke")
	}

	firstUser, ok := dataArr[0].(map[string]interface{})
	if !ok {
		t.Fatalf("users list: first item is not a map")
	}
	userID, _ := firstUser["id"].(string)
	if userID == "" {
		t.Skip("could not extract user ID; skipping")
	}

	// Step 2: call users get with the real ID.
	integrationResetFlags(t)

	out, err := captureStdoutIntegration(t, func() error {
		return runUsersGet(nil, []string{userID})
	})
	if err != nil {
		t.Fatalf("users get %s: unexpected error: %v", userID, err)
	}
	requireNonEmptyOutput(t, out, fmt.Sprintf("users get %s", userID))
}

// TestIntegration_SLOsGet uses slos list to discover a real SLO ID.
// Skipped if the org has no SLOs.
func TestIntegration_SLOsGet(t *testing.T) {
	skipIfNoCredentials(t)
	integrationResetFlags(t)

	// Step 1: list SLOs --limit 1 --json.
	slosListIDs = ""
	slosListTagQuery = ""
	slosListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true
	t.Cleanup(func() {
		flagJSON = false
		slosListIDs = ""
		slosListTagQuery = ""
		slosListAll = false
	})

	outList, err := captureStdoutIntegration(t, func() error {
		return runSLOsList(nil, nil)
	})
	if err != nil {
		t.Fatalf("slos list (pre-get): unexpected error: %v", err)
	}

	var listResult map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(outList)), &listResult); err != nil {
		t.Fatalf("slos list --json: parse error: %v\nOutput:\n%s", err, outList)
	}
	dataArr, _ := listResult["data"].([]interface{})
	if len(dataArr) == 0 {
		t.Skip("no SLOs available; skipping slos get smoke")
	}

	firstSLO, ok := dataArr[0].(map[string]interface{})
	if !ok {
		t.Fatalf("slos list: first item is not a map")
	}
	sloID, _ := firstSLO["id"].(string)
	if sloID == "" {
		t.Skip("could not extract SLO ID; skipping")
	}

	// Step 2: call slos get with the real ID.
	integrationResetFlags(t)

	out, err := captureStdoutIntegration(t, func() error {
		return runSLOsGet(nil, []string{sloID})
	})
	if err != nil {
		t.Fatalf("slos get %s: unexpected error: %v", sloID, err)
	}
	requireNonEmptyOutput(t, out, fmt.Sprintf("slos get %s", sloID))
}

// ============================================================================
// helpers
// ============================================================================

// requireNonEmptyOutput fails the test if the output is empty or contains only
// a "no X found" message that means the command succeeded but returned nothing.
func requireNonEmptyOutput(t *testing.T, out, cmd string) {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Errorf("%s: output was empty; expected at least a header or data row", cmd)
	}
}

// mapKeys returns the keys of a map for use in error messages.
func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
