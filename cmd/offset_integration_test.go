//go:build integration

package cmd

// Live integration tests for offset/page-index-based paginated commands.
//
// Each test calls the real run* function against the live Datadog API.
// Credentials are read from DD_API_KEY and DD_APP_KEY env vars.
// Tests are skipped automatically when credentials are absent.
//
// Commands covered:
//   - monitors list   (page index)
//   - monitors search (page index)
//   - slos list       (limit + offset)
//   - incidents list  (page[size] + page[offset])
//   - dashboards list (count + start)
//   - hosts list      (start + count)
//   - notebooks list  (count + start)
//   - downtimes list  (page[limit] + page[offset])

import (
	"encoding/json"
	"strings"
	"testing"
)

// ============================================================================
// monitors list
// ============================================================================

func TestIntegration_MonitorsList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsListAll = false
	monitorsListGroupStates = ""
	monitorsListName = ""
	monitorsListTags = ""
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runMonitorsList(nil, nil); err != nil {
			t.Fatalf("runMonitorsList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatal("monitors list returned empty output; expected at least one monitor")
	}
}

func TestIntegration_MonitorsList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsListAll = false
	monitorsListGroupStates = ""
	monitorsListName = ""
	monitorsListTags = ""
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runMonitorsList(nil, nil); err != nil {
			t.Fatalf("runMonitorsList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	var result []interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if len(result) != 1 {
		t.Errorf("expected exactly 1 monitor with --limit 1, got %d", len(result))
	}
}

func TestIntegration_MonitorsList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsListAll = false
	monitorsListGroupStates = ""
	monitorsListName = ""
	monitorsListTags = ""
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runMonitorsList(nil, nil); err != nil {
			t.Fatalf("runMonitorsList --json returned error: %v", err)
		}
	})

	flagJSON = false

	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_MonitorsList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsListAll = false
	monitorsListGroupStates = ""
	monitorsListName = ""
	monitorsListTags = ""
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runMonitorsList(nil, nil); err != nil {
			t.Fatalf("runMonitorsList --limit 2000 returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatal("monitors list --limit 2000 returned empty output")
	}
}

// ============================================================================
// monitors search
// ============================================================================

func TestIntegration_MonitorsSearch_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsSearchAll = false
	monitorsSearchQuery = "type:metric"
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runMonitorsSearch(nil, nil); err != nil {
			t.Fatalf("runMonitorsSearch returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Log("monitors search returned no results (query may match nothing in this org, not a failure)")
	}

	monitorsSearchQuery = ""
}

func TestIntegration_MonitorsSearch_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsSearchAll = false
	monitorsSearchQuery = "type:metric"
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runMonitorsSearch(nil, nil); err != nil {
			t.Fatalf("runMonitorsSearch --limit 1 returned error: %v", err)
		}
	})

	monitorsSearchQuery = ""
	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" || trimmed == "No monitors found matching" {
		t.Skip("no monitors matched query; skipping limit assertion")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_MonitorsSearch_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	monitorsSearchAll = false
	monitorsSearchQuery = "type:metric"
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runMonitorsSearch(nil, nil); err != nil {
			t.Fatalf("runMonitorsSearch --json returned error: %v", err)
		}
	})

	monitorsSearchQuery = ""
	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no monitors matched query; skipping JSON validation")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

// ============================================================================
// slos list
// ============================================================================

func TestIntegration_SLOsList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	slosListAll = false
	slosListIDs = ""
	slosListTagQuery = ""
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runSLOsList(nil, nil); err != nil {
			t.Fatalf("runSLOsList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Log("slos list returned empty output (org may have no SLOs defined)")
	}
}

func TestIntegration_SLOsList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	slosListAll = false
	slosListIDs = ""
	slosListTagQuery = ""
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runSLOsList(nil, nil); err != nil {
			t.Fatalf("runSLOsList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no SLOs in org; skipping limit assertion")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if arr, ok := result.([]interface{}); ok && len(arr) > 1 {
		t.Errorf("expected at most 1 SLO with --limit 1, got %d", len(arr))
	}
}

func TestIntegration_SLOsList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	slosListAll = false
	slosListIDs = ""
	slosListTagQuery = ""
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runSLOsList(nil, nil); err != nil {
			t.Fatalf("runSLOsList --json returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no SLOs in org; skipping JSON validation")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_SLOsList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	slosListAll = false
	slosListIDs = ""
	slosListTagQuery = ""
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runSLOsList(nil, nil); err != nil {
			t.Fatalf("runSLOsList --limit 2000 returned error: %v", err)
		}
	})

	// Empty is acceptable if the org has no SLOs.
	_ = out
}

// ============================================================================
// incidents list
// ============================================================================

func TestIntegration_IncidentsList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	incidentsListAll = false
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runIncidentsList(nil, nil); err != nil {
			t.Fatalf("runIncidentsList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Log("incidents list returned empty output (org may have no incidents)")
	}
}

func TestIntegration_IncidentsList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	incidentsListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runIncidentsList(nil, nil); err != nil {
			t.Fatalf("runIncidentsList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no incidents in org; skipping limit assertion")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if m, ok := result.(map[string]interface{}); ok {
		if data, ok := m["data"].([]interface{}); ok && len(data) > 1 {
			t.Errorf("expected at most 1 incident with --limit 1, got %d", len(data))
		}
	}
}

func TestIntegration_IncidentsList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	incidentsListAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runIncidentsList(nil, nil); err != nil {
			t.Fatalf("runIncidentsList --json returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no incidents in org; skipping JSON validation")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_IncidentsList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	incidentsListAll = false
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runIncidentsList(nil, nil); err != nil {
			t.Fatalf("runIncidentsList --limit 2000 returned error: %v", err)
		}
	})

	_ = out
}

// ============================================================================
// dashboards list
// ============================================================================

func TestIntegration_DashboardsList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	dashboardsListAll = false
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Fatalf("runDashboardsList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatal("dashboards list returned empty output; expected at least one dashboard")
	}
}

func TestIntegration_DashboardsList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	dashboardsListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Fatalf("runDashboardsList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if arr, ok := result.([]interface{}); ok && len(arr) != 1 {
		t.Errorf("expected exactly 1 dashboard with --limit 1, got %d", len(arr))
	}
}

func TestIntegration_DashboardsList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	dashboardsListAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Fatalf("runDashboardsList --json returned error: %v", err)
		}
	})

	flagJSON = false

	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_DashboardsList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	dashboardsListAll = false
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Fatalf("runDashboardsList --limit 2000 returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatal("dashboards list --limit 2000 returned empty output")
	}
}

// ============================================================================
// hosts list
// ============================================================================

func TestIntegration_HostsList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Fatalf("runHostsList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatal("hosts list returned empty output; expected at least one host")
	}
}

func TestIntegration_HostsList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Fatalf("runHostsList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if arr, ok := result.([]interface{}); ok && len(arr) != 1 {
		t.Errorf("expected exactly 1 host with --limit 1, got %d", len(arr))
	}
}

func TestIntegration_HostsList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Fatalf("runHostsList --json returned error: %v", err)
		}
	})

	flagJSON = false

	var result interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_HostsList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Fatalf("runHostsList --limit 2000 returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatal("hosts list --limit 2000 returned empty output")
	}
}

// ============================================================================
// notebooks list
// ============================================================================

func TestIntegration_NotebooksList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	notebooksListAll = false
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runNotebooksList(nil, nil); err != nil {
			t.Fatalf("runNotebooksList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Log("notebooks list returned empty output (org may have no notebooks)")
	}
}

func TestIntegration_NotebooksList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	notebooksListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runNotebooksList(nil, nil); err != nil {
			t.Fatalf("runNotebooksList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no notebooks in org; skipping limit assertion")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if m, ok := result.(map[string]interface{}); ok {
		if data, ok := m["data"].([]interface{}); ok && len(data) > 1 {
			t.Errorf("expected at most 1 notebook with --limit 1, got %d", len(data))
		}
	}
}

func TestIntegration_NotebooksList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	notebooksListAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runNotebooksList(nil, nil); err != nil {
			t.Fatalf("runNotebooksList --json returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no notebooks in org; skipping JSON validation")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_NotebooksList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	notebooksListAll = false
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runNotebooksList(nil, nil); err != nil {
			t.Fatalf("runNotebooksList --limit 2000 returned error: %v", err)
		}
	})

	_ = out
}

// ============================================================================
// downtimes list
// ============================================================================

func TestIntegration_DowntimesList_BasicSmoke(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	downtimesListAll = false
	flagLimit = 10

	out := captureStdout(t, func() {
		if err := runDowntimesList(nil, nil); err != nil {
			t.Fatalf("runDowntimesList returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Log("downtimes list returned empty output (org may have no active downtimes)")
	}
}

func TestIntegration_DowntimesList_LimitOne(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	downtimesListAll = false
	flagLimit = 1
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runDowntimesList(nil, nil); err != nil {
			t.Fatalf("runDowntimesList --limit 1 returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no downtimes in org; skipping limit assertion")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if m, ok := result.(map[string]interface{}); ok {
		if data, ok := m["data"].([]interface{}); ok && len(data) > 1 {
			t.Errorf("expected at most 1 downtime with --limit 1, got %d", len(data))
		}
	}
}

func TestIntegration_DowntimesList_JSONOutput(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	downtimesListAll = false
	flagLimit = 10
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runDowntimesList(nil, nil); err != nil {
			t.Fatalf("runDowntimesList --json returned error: %v", err)
		}
	})

	flagJSON = false

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Skip("no downtimes in org; skipping JSON validation")
	}

	var result interface{}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
}

func TestIntegration_DowntimesList_HighLimit(t *testing.T) {
	skipIfNoCredentials(t)

	resetFlags()
	downtimesListAll = false
	flagLimit = 2000
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runDowntimesList(nil, nil); err != nil {
			t.Fatalf("runDowntimesList --limit 2000 returned error: %v", err)
		}
	})

	_ = out
}
