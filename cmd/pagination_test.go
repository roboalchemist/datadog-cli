package cmd

// Pagination regression tests.
//
// Each test mocks an HTTP server via httptest.NewServer and drives the
// real run* functions directly (same package).  No real HTTP calls are made.
//
// Mechanism for injecting the mock server:
//   - Set DD_API_URL to the test server URL  (api/client.go honours this)
//   - Set DD_API_KEY / DD_APP_KEY to dummy values so auth.ResolveCredentials succeeds
//
// After every test the env vars are cleaned up via t.Cleanup.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

// setMockServer points the CLI at a test HTTP server and installs credentials.
// The returned cleanup function restores the original env.
func setMockServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("DD_API_URL", srv.URL)
	t.Setenv("DD_API_KEY", "test-api-key")
	t.Setenv("DD_APP_KEY", "test-app-key")
}

// captureStdout runs f() and returns what it wrote to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return buf.String()
}

// writeJSON marshals v to the ResponseWriter as application/json.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// makeLogItem returns a minimal log item map with a unique ID.
func makeLogItem(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":   id,
		"type": "log",
		"attributes": map[string]interface{}{
			"timestamp": "2024-01-01T00:00:00Z",
			"host":      "host-1",
			"service":   "svc",
			"status":    "info",
			"message":   "msg-" + id,
		},
	}
}

// makeProcessItem returns a minimal process item map.
func makeProcessItem(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":   id,
		"type": "process",
		"attributes": map[string]interface{}{
			"pid":     1,
			"name":    "proc-" + id,
			"cmdline": "proc-" + id,
			"user":    "root",
			"host":    "host-1",
		},
	}
}

// makeMonitorItem returns a minimal monitor item map.
func makeMonitorItem(id int) map[string]interface{} {
	return map[string]interface{}{
		"id":            float64(id),
		"name":          fmt.Sprintf("monitor-%d", id),
		"type":          "metric alert",
		"overall_state": "OK",
		"creator": map[string]interface{}{
			"email": "user@example.com",
		},
	}
}

// makeSLOItem returns a minimal SLO item map.
func makeSLOItem(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":   id,
		"name": "slo-" + id,
		"type": "metric",
		"thresholds": []interface{}{
			map[string]interface{}{"target": 99.9, "timeframe": "7d"},
		},
	}
}

// resetFlags resets all relevant global flag vars to their defaults.
// Must be called before each run* invocation to avoid cross-test contamination.
func resetFlags() {
	flagJSON = false
	flagPlaintext = false
	flagNoColor = false
	flagDebug = false
	flagVerbose = false
	flagQuiet = true // suppress progress output in tests
	flagLimit = 100
	flagProfile = ""
	flagSite = ""
	flagAPIKey = ""
	flagAppKey = ""
	flagFields = ""
	flagJQ = ""
}

// ============================================================================
// 1. Cursor-based POST pagination — logs search
//    Page 1 has cursor, page 2 has no cursor.
//    --limit 3 (> page 1 size of 2) must trigger a second fetch.
// ============================================================================

func TestPaginationLogsSearch_CursorPOST_TwoPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Page 1: 2 items + cursor
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeLogItem("log-1"),
					makeLogItem("log-2"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{
						"after": "cursor-page-2",
					},
				},
			})
		default:
			// Page 2: 2 items, no cursor
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeLogItem("log-3"),
					makeLogItem("log-4"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4 // exceeds single page of 2
	flagQuiet = true

	// Reset command-local state
	logsSearchAll = false
	logsSearchQuery = "test"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil

	out := captureStdout(t, func() {
		err := runLogsSearch(nil, nil)
		if err != nil {
			t.Errorf("runLogsSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// All 4 items should appear in the output
	for _, id := range []string{"log-1", "log-2", "log-3", "log-4"} {
		if !strings.Contains(out, "msg-"+id) {
			t.Errorf("output missing item %s; output:\n%s", id, out)
		}
	}
}

// ============================================================================
// 2. Cursor-based GET pagination — processes list
//    Page 1 has cursor, page 2 has no cursor.
//    --limit 4 must fetch both pages.
// ============================================================================

func TestPaginationProcessesList_CursorGET_TwoPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeProcessItem("p1"),
					makeProcessItem("p2"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{
						"after": "cursor-page-2",
					},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeProcessItem("p3"),
					makeProcessItem("p4"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	processesListAll = false
	processesListSearch = ""
	processesListHost = ""

	out := captureStdout(t, func() {
		err := runProcessesList(nil, nil)
		if err != nil {
			t.Errorf("runProcessesList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	for _, id := range []string{"p1", "p2", "p3", "p4"} {
		if !strings.Contains(out, "proc-"+id) {
			t.Errorf("output missing process %s; output:\n%s", id, out)
		}
	}
}

// ============================================================================
// 3. Offset-based pagination — monitors list
//    The Datadog monitors list API uses page_size + page (offset-based).
//    Pagination stops when the page is smaller than page_size or empty.
//    We set --limit 200 (> maxMonitorsPageSize=100) so page_size=100.
//    Page 1 returns exactly 100 items (full page → more expected).
//    Page 2 returns 3 items (short page → signals end).
// ============================================================================

func TestPaginationMonitorsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	// Build a page of N monitor items starting from a given ID.
	makeMonitorPage := func(startID, n int) []interface{} {
		page := make([]interface{}, n)
		for i := 0; i < n; i++ {
			page[i] = makeMonitorItem(startID + i)
		}
		return page
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (100 items = maxMonitorsPageSize) → pagination continues
			writeJSON(w, makeMonitorPage(1, 100))
		default:
			// Short page (3 items < 100) → signals last page
			writeJSON(w, makeMonitorPage(101, 3))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	// Use a limit larger than one page so the pagination loop must fetch page 2.
	flagLimit = 200
	monitorsListAll = false
	monitorsListGroupStates = ""
	monitorsListName = ""
	monitorsListTags = ""

	out := captureStdout(t, func() {
		err := runMonitorsList(nil, nil)
		if err != nil {
			t.Errorf("runMonitorsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// Spot-check items from both pages
	for _, id := range []int{1, 50, 100, 101, 103} {
		want := fmt.Sprintf("monitor-%d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s; output (truncated):\n%s", want, out[:min(len(out), 500)])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// 4. --all flag — logs search with 3 pages
//    --all must keep fetching until the cursor is gone.
// ============================================================================

func TestPaginationLogsSearch_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeLogItem("a1"), makeLogItem("a2")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"after": "cur-2"},
				},
			})
		case 2:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeLogItem("a3"), makeLogItem("a4")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"after": "cur-3"},
				},
			})
		default:
			// Last page: no cursor
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeLogItem("a5"), makeLogItem("a6")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2 // --all should override this
	flagQuiet = true
	logsSearchAll = true
	logsSearchQuery = "test"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil

	out := captureStdout(t, func() {
		err := runLogsSearch(nil, nil)
		if err != nil {
			t.Errorf("runLogsSearch --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	// All 6 items across 3 pages must appear
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5", "a6"} {
		if !strings.Contains(out, "msg-"+id) {
			t.Errorf("output missing item %s with --all; output:\n%s", id, out)
		}
	}

	// Cleanup: reset --all flag
	logsSearchAll = false
}

// ============================================================================
// 5. JSON mode regression — DDOG-50 bug
//    --json --limit N must return a merged array from ALL pages, not just page 1.
// ============================================================================

func TestPaginationLogsSearch_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeLogItem("j1"),
					makeLogItem("j2"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"after": "cursor-json-p2"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeLogItem("j3"),
					makeLogItem("j4"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	flagJSON = true
	flagQuiet = true
	logsSearchAll = false
	logsSearchQuery = "test"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil

	out := captureStdout(t, func() {
		err := runLogsSearch(nil, nil)
		if err != nil {
			t.Errorf("runLogsSearch --json returned error: %v", err)
		}
	})

	// Verify both pages were fetched
	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d; early-exit regression?", callCount)
	}

	// Parse the JSON output and count data items
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}

	dataArr, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'data' array; output:\n%s", out)
	}

	if len(dataArr) != 4 {
		t.Errorf("--json mode returned %d items, want 4 (all pages merged); early-exit regression?", len(dataArr))
	}

	// Restore flag
	flagJSON = false
}

// ============================================================================
// 6. Offset-based pagination — SLOs list
//    The SLO list API uses limit+offset.  Pagination stops when:
//      (a) offset >= total_count (from metadata.pagination.total_count), OR
//      (b) the page is shorter than fetchSize (no more data).
//    We use --limit 2000 (> maxSLOsPageSize=1000) so fetchSize=1000.
//    Page 1: returns 1000 items (full page) + total_count=1003.
//    Page 2: returns 3 items (short page) → end.
// ============================================================================

func TestPaginationSLOsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	makeSLOPage := func(prefix string, n int) []interface{} {
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			items[i] = makeSLOItem(fmt.Sprintf("%s-%04d", prefix, i))
		}
		return items
	}

	const totalCount = 1003

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (1000 items = maxSLOsPageSize)
			writeJSON(w, map[string]interface{}{
				"data": makeSLOPage("pg1", 1000),
				"metadata": map[string]interface{}{
					"pagination": map[string]interface{}{
						"total_count": float64(totalCount),
					},
				},
			})
		default:
			// Last page (3 items, total = 1003)
			writeJSON(w, map[string]interface{}{
				"data": makeSLOPage("pg2", 3),
				"metadata": map[string]interface{}{
					"pagination": map[string]interface{}{
						"total_count": float64(totalCount),
					},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2000 // exceeds one page → must paginate
	slosListAll = false
	slosListIDs = ""
	slosListTagQuery = ""

	out := captureStdout(t, func() {
		err := runSLOsList(nil, nil)
		if err != nil {
			t.Errorf("runSLOsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// Spot-check items from both pages
	for _, id := range []string{"pg1-0000", "pg1-0999", "pg2-0000", "pg2-0002"} {
		if !strings.Contains(out, id) {
			t.Errorf("output missing SLO %s; output (truncated):\n%.500s", id, out)
		}
	}
}

// ============================================================================
// 7. Cursor-based GET pagination — containers list
//    Uses meta.pagination.next_cursor.
// ============================================================================

func TestPaginationContainersList_CursorGET_TwoPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{
						"id":   "ctr-1",
						"type": "container",
						"attributes": map[string]interface{}{
							"name":       "container-1",
							"image_name": "nginx",
							"state":      "running",
							"host":       "host-1",
						},
					},
					map[string]interface{}{
						"id":   "ctr-2",
						"type": "container",
						"attributes": map[string]interface{}{
							"name":       "container-2",
							"image_name": "redis",
							"state":      "running",
							"host":       "host-2",
						},
					},
				},
				"meta": map[string]interface{}{
					"pagination": map[string]interface{}{
						"next_cursor": "containers-cursor-p2",
					},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					map[string]interface{}{
						"id":   "ctr-3",
						"type": "container",
						"attributes": map[string]interface{}{
							"name":       "container-3",
							"image_name": "postgres",
							"state":      "running",
							"host":       "host-3",
						},
					},
				},
				"meta": map[string]interface{}{
					"pagination": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 3
	containersListAll = false
	containersListFilter = ""

	out := captureStdout(t, func() {
		err := runContainersList(nil, nil)
		if err != nil {
			t.Errorf("runContainersList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (containers pagination), got %d", callCount)
	}

	for _, name := range []string{"container-1", "container-2", "container-3"} {
		if !strings.Contains(out, name) {
			t.Errorf("output missing %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 8. --all flag with processes list — 3 pages exhausted
// ============================================================================

func TestPaginationProcessesList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeProcessItem("q1"), makeProcessItem("q2")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"after": "cur-q2"},
				},
			})
		case 2:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeProcessItem("q3"), makeProcessItem("q4")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"after": "cur-q4"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeProcessItem("q5")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all should ignore this
	flagQuiet = true
	processesListAll = true
	processesListSearch = ""
	processesListHost = ""

	out := captureStdout(t, func() {
		err := runProcessesList(nil, nil)
		if err != nil {
			t.Errorf("runProcessesList --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []string{"q1", "q2", "q3", "q4", "q5"} {
		if !strings.Contains(out, "proc-"+id) {
			t.Errorf("output missing process %s with --all; output:\n%s", id, out)
		}
	}

	processesListAll = false
}

// ============================================================================
// 9. JSON mode regression — processes list (DDOG-50 analog)
//    --json --limit N must merge all pages.
// ============================================================================

func TestPaginationProcessesList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeProcessItem("r1"), makeProcessItem("r2")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"after": "cursor-r2"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeProcessItem("r3")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 3
	flagJSON = true
	flagQuiet = true
	processesListAll = false
	processesListSearch = ""
	processesListHost = ""

	out := captureStdout(t, func() {
		err := runProcessesList(nil, nil)
		if err != nil {
			t.Errorf("runProcessesList --json returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d", callCount)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}

	dataArr, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'data' array; output:\n%s", out)
	}

	if len(dataArr) != 3 {
		t.Errorf("--json mode returned %d items, want 3; early-exit regression?", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// 10. --limit exactly on a page boundary — logs search
//     --limit 2 when page size is 2: should stop after 1 page and NOT fetch more.
// ============================================================================

func TestPaginationLogsSearch_LimitAtPageBoundary_StopsEarly(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeLogItem("b1"),
				makeLogItem("b2"),
			},
			"meta": map[string]interface{}{
				"page": map[string]interface{}{"after": "cursor-would-page-2"},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2
	flagQuiet = true
	logsSearchAll = false
	logsSearchQuery = "test"
	logsSearchFrom = "1h"
	logsSearchTo = "now"
	logsSearchSort = "-timestamp"
	logsSearchIndexes = nil

	out := captureStdout(t, func() {
		err := runLogsSearch(nil, nil)
		if err != nil {
			t.Errorf("runLogsSearch returned error: %v", err)
		}
	})

	// Should have fetched exactly 1 page since limit == page size
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call (limit == page size), got %d", callCount)
	}

	for _, id := range []string{"b1", "b2"} {
		if !strings.Contains(out, "msg-"+id) {
			t.Errorf("output missing item %s; output:\n%s", id, out)
		}
	}
}
