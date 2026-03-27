package cmd

// Pagination tests for cursor-GET and count+start GET commands.
//
// Covered commands:
//   - apm services   (GET /api/v2/services/definitions, meta.page.cursor)
//   - apm definitions (GET /api/v2/services/definitions, meta.page.cursor)
//   - users list      (GET /api/v2/users — single-page, no cursor in current impl)
//   - api-keys list   (GET /api/v2/api_keys — single-page, no cursor in current impl)
//   - dashboards list (GET /api/v1/dashboard, count+start offset)
//   - dashboards search (same endpoint, client-side filter after paginated fetch)
//   - hosts list      (GET /api/v1/hosts, start+count offset)
//
// Pattern: httptest.NewServer + setMockServer + direct run* call.
// No real HTTP calls are made.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

// makeAPMServiceItem returns a minimal APM service definition item.
func makeAPMServiceItem(name string) map[string]interface{} {
	return map[string]interface{}{
		"type": "service-definition",
		"id":   name,
		"attributes": map[string]interface{}{
			"schema": map[string]interface{}{
				"dd-service": name,
				"team":       "team-a",
			},
			"meta": map[string]interface{}{
				"schema-version": "v2",
			},
		},
	}
}

// makeUserItem returns a minimal user data item.
func makeUserItem(id, email string) map[string]interface{} {
	return map[string]interface{}{
		"type": "users",
		"id":   id,
		"attributes": map[string]interface{}{
			"name":   "User " + id,
			"email":  email,
			"status": "active",
		},
	}
}

// makeAPIKeyItem returns a minimal API key data item.
func makeAPIKeyItem(id, name string) map[string]interface{} {
	return map[string]interface{}{
		"type": "api_keys",
		"id":   id,
		"attributes": map[string]interface{}{
			"name":       name,
			"last4":      "abcd",
			"created_at": "2024-01-01T00:00:00Z",
		},
	}
}

// makeDashboardItem returns a minimal dashboard item.
func makeDashboardItem(id, title string) map[string]interface{} {
	return map[string]interface{}{
		"id":            id,
		"title":         title,
		"author_handle": "user@example.com",
		"url":           "/dashboard/" + id,
		"created_at":    "2024-01-01T00:00:00Z",
	}
}

// makeHostItem returns a minimal host_list item.
func makeHostItem(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":               name,
		"up":                 true,
		"last_reported_time": float64(1700000000),
	}
}

// ============================================================================
// 1. APM services — cursor-GET two pages
//    Page 1 has cursor, page 2 has no cursor.
//    --limit 4 must trigger a second fetch.
// ============================================================================

func TestPaginationAPMServices_CursorGET_TwoPages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeAPMServiceItem("svc-1"),
					makeAPMServiceItem("svc-2"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{
						"cursor": "apm-cursor-p2",
					},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeAPMServiceItem("svc-3"),
					makeAPMServiceItem("svc-4"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	apmServicesAll = false

	out := captureStdout(t, func() {
		if err := runAPMServices(nil, nil); err != nil {
			t.Errorf("runAPMServices returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	for _, name := range []string{"svc-1", "svc-2", "svc-3", "svc-4"} {
		if !containsString(out, name) {
			t.Errorf("output missing service %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 2. APM services — --all flag exhausts all three pages
// ============================================================================

func TestPaginationAPMServices_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("all-s1"), makeAPMServiceItem("all-s2")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"cursor": "cur-s2"},
				},
			})
		case 2:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("all-s3"), makeAPMServiceItem("all-s4")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"cursor": "cur-s4"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("all-s5")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all should ignore this
	flagQuiet = true
	apmServicesAll = true

	out := captureStdout(t, func() {
		if err := runAPMServices(nil, nil); err != nil {
			t.Errorf("runAPMServices --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, name := range []string{"all-s1", "all-s2", "all-s3", "all-s4", "all-s5"} {
		if !containsString(out, name) {
			t.Errorf("output missing service %s with --all; output:\n%s", name, out)
		}
	}

	apmServicesAll = false
}

// ============================================================================
// 3. APM services — --json merges all pages
// ============================================================================

func TestPaginationAPMServices_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("js1"), makeAPMServiceItem("js2")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"cursor": "json-cur-p2"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("js3")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 3
	flagJSON = true
	flagQuiet = true
	apmServicesAll = false

	out := captureStdout(t, func() {
		if err := runAPMServices(nil, nil); err != nil {
			t.Errorf("runAPMServices --json returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d", callCount)
	}

	result := mustParseJSONMap(t, out)
	dataArr, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'data' array; output:\n%s", out)
	}
	if len(dataArr) != 3 {
		t.Errorf("--json mode returned %d items, want 3 (all pages merged)", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// 4. APM definitions — cursor-GET two pages
// ============================================================================

func TestPaginationAPMDefinitions_CursorGET_TwoPages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeAPMServiceItem("def-1"),
					makeAPMServiceItem("def-2"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"cursor": "def-cursor-p2"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{
					makeAPMServiceItem("def-3"),
					makeAPMServiceItem("def-4"),
				},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	apmDefinitionsAll = false

	out := captureStdout(t, func() {
		if err := runAPMDefinitions(nil, nil); err != nil {
			t.Errorf("runAPMDefinitions returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	for _, name := range []string{"def-1", "def-2", "def-3", "def-4"} {
		if !containsString(out, name) {
			t.Errorf("output missing definition %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 5. APM definitions — --all flag exhausts all pages
// ============================================================================

func TestPaginationAPMDefinitions_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("da1"), makeAPMServiceItem("da2")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"cursor": "d-cur-2"},
				},
			})
		case 2:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("da3"), makeAPMServiceItem("da4")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{"cursor": "d-cur-3"},
				},
			})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeAPMServiceItem("da5")},
				"meta": map[string]interface{}{
					"page": map[string]interface{}{},
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all should ignore this
	flagQuiet = true
	apmDefinitionsAll = true

	out := captureStdout(t, func() {
		if err := runAPMDefinitions(nil, nil); err != nil {
			t.Errorf("runAPMDefinitions --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, name := range []string{"da1", "da2", "da3", "da4", "da5"} {
		if !containsString(out, name) {
			t.Errorf("output missing definition %s with --all; output:\n%s", name, out)
		}
	}

	apmDefinitionsAll = false
}

// ============================================================================
// 6. Users list — single-page response (current impl fetches one page)
//    Verifies basic user display and --limit slicing.
// ============================================================================

func TestPaginationUsersList_SinglePage(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeUserItem("u1", "alice@example.com"),
				makeUserItem("u2", "bob@example.com"),
				makeUserItem("u3", "carol@example.com"),
			},
		})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 100

	out := captureStdout(t, func() {
		if err := runUsersList(nil, nil); err != nil {
			t.Errorf("runUsersList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call, got %d", callCount)
	}

	for _, email := range []string{"alice@example.com", "bob@example.com", "carol@example.com"} {
		if !containsString(out, email) {
			t.Errorf("output missing user %s; output:\n%s", email, out)
		}
	}
}

// ============================================================================
// 7. Users list — --limit slices the result
// ============================================================================

func TestPaginationUsersList_LimitSlicesResult(t *testing.T) {
	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeUserItem("u1", "first@example.com"),
				makeUserItem("u2", "second@example.com"),
				makeUserItem("u3", "third@example.com"),
			},
		})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2

	out := captureStdout(t, func() {
		if err := runUsersList(nil, nil); err != nil {
			t.Errorf("runUsersList returned error: %v", err)
		}
	})

	if containsString(out, "third@example.com") {
		t.Errorf("output should NOT contain third@example.com with --limit 2; output:\n%s", out)
	}
}

// ============================================================================
// 8. Users list — --json output
// ============================================================================

func TestPaginationUsersList_JSONMode(t *testing.T) {
	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeUserItem("u1", "json-user@example.com"),
			},
		})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 100
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runUsersList(nil, nil); err != nil {
			t.Errorf("runUsersList --json returned error: %v", err)
		}
	})

	result := mustParseJSONMap(t, out)
	dataArr, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'data' array; output:\n%s", out)
	}
	if len(dataArr) != 1 {
		t.Errorf("--json output has %d items, want 1", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// 9. API keys list — single-page response
// ============================================================================

func TestPaginationAPIKeysList_SinglePage(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeAPIKeyItem("k1", "prod-key"),
				makeAPIKeyItem("k2", "staging-key"),
			},
		})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 100

	out := captureStdout(t, func() {
		if err := runAPIKeysList(nil, nil); err != nil {
			t.Errorf("runAPIKeysList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call, got %d", callCount)
	}

	for _, name := range []string{"prod-key", "staging-key"} {
		if !containsString(out, name) {
			t.Errorf("output missing API key %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 10. API keys list — --json output
// ============================================================================

func TestPaginationAPIKeysList_JSONMode(t *testing.T) {
	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeAPIKeyItem("k1", "json-key"),
			},
		})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 100
	flagJSON = true
	flagQuiet = true

	out := captureStdout(t, func() {
		if err := runAPIKeysList(nil, nil); err != nil {
			t.Errorf("runAPIKeysList --json returned error: %v", err)
		}
	})

	result := mustParseJSONMap(t, out)
	dataArr, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'data' array; output:\n%s", out)
	}
	if len(dataArr) != 1 {
		t.Errorf("--json output has %d items, want 1", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// 11. Dashboards list — count+start offset two pages
//     Page 1: full page (pageSize items) → more expected.
//     Page 2: short page → signals last page.
// ============================================================================

func TestPaginationDashboardsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	makeDashboardPage := func(prefix string, n int) []interface{} {
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("%s-%03d", prefix, i)
			items[i] = makeDashboardItem(id, "Dashboard "+id)
		}
		return items
	}

	// maxDashboardsPageSize = 100 in dashboards.go
	// Use flagLimit = 150 so first page (100 items) is a full page → pagination.
	// Page 2 returns 5 items (short) → end.
	const pageSize = 100

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"dashboards": makeDashboardPage("pg1", pageSize),
			})
		default:
			writeJSON(w, map[string]interface{}{
				"dashboards": makeDashboardPage("pg2", 5),
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 150
	dashboardsListAll = false

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Errorf("runDashboardsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// Spot-check items from both pages
	for _, id := range []string{"pg1-000", "pg1-099", "pg2-000", "pg2-004"} {
		if !containsString(out, id) {
			t.Errorf("output missing dashboard %s; output (truncated):\n%.500s", id, out)
		}
	}
}

// ============================================================================
// 12. Dashboards list — --all flag exhausts all pages
// ============================================================================

func TestPaginationDashboardsList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			items := make([]interface{}, maxDashboardsPageSize)
			for i := 0; i < maxDashboardsPageSize; i++ {
				id := fmt.Sprintf("all-d1-%03d", i)
				items[i] = makeDashboardItem(id, "Dashboard "+id)
			}
			writeJSON(w, map[string]interface{}{"dashboards": items})
		case 2:
			items := make([]interface{}, maxDashboardsPageSize)
			for i := 0; i < maxDashboardsPageSize; i++ {
				id := fmt.Sprintf("all-d2-%03d", i)
				items[i] = makeDashboardItem(id, "Dashboard "+id)
			}
			writeJSON(w, map[string]interface{}{"dashboards": items})
		default:
			writeJSON(w, map[string]interface{}{
				"dashboards": []interface{}{
					makeDashboardItem("all-d3-last", "Dashboard all-d3-last"),
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all should override
	flagQuiet = true
	dashboardsListAll = true

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Errorf("runDashboardsList --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []string{"all-d1-000", "all-d2-000", "all-d3-last"} {
		if !containsString(out, id) {
			t.Errorf("output missing dashboard %s with --all; output (truncated):\n%.500s", id, out)
		}
	}

	dashboardsListAll = false
}

// ============================================================================
// 13. Dashboards list — --limit stops early
// ============================================================================

func TestPaginationDashboardsList_LimitStopsEarly(t *testing.T) {
	var callCount int32

	// Server always returns a full page of 100 with a next-page worth of items.
	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		atomic.AddInt32(&callCount, 1)
		items := make([]interface{}, maxDashboardsPageSize)
		for i := 0; i < maxDashboardsPageSize; i++ {
			id := fmt.Sprintf("lim-d%d-%03d", n, i)
			items[i] = makeDashboardItem(id, "Dashboard "+id)
		}
		writeJSON(w, map[string]interface{}{"dashboards": items})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 50
	dashboardsListAll = false

	captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Errorf("runDashboardsList returned error: %v", err)
		}
	})

	// With flagLimit=50 < pageSize=100, one page satisfies the limit; no second fetch.
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call with --limit 50 < pageSize 100, got %d", callCount)
	}
}

// ============================================================================
// 14. Dashboards search — fetches all pages, then client-side filter
//     Page 1: full page (100 items, one matching).
//     Page 2: short page (3 items, one matching).
// ============================================================================

func TestPaginationDashboardsSearch_AllPagesThenFilter(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			items := make([]interface{}, maxDashboardsPageSize)
			for i := 0; i < maxDashboardsPageSize; i++ {
				title := fmt.Sprintf("Generic Dashboard %03d", i)
				if i == 42 {
					title = "Special System Overview"
				}
				items[i] = makeDashboardItem(fmt.Sprintf("d1-%03d", i), title)
			}
			writeJSON(w, map[string]interface{}{"dashboards": items})
		default:
			writeJSON(w, map[string]interface{}{
				"dashboards": []interface{}{
					makeDashboardItem("d2-000", "Another System Monitor"),
					makeDashboardItem("d2-001", "Unrelated Dashboard"),
					makeDashboardItem("d2-002", "Yet Another Board"),
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1000
	dashboardsSearchQuery = "system"

	out := captureStdout(t, func() {
		if err := runDashboardsSearch(nil, nil); err != nil {
			t.Errorf("runDashboardsSearch returned error: %v", err)
		}
	})

	// Both pages must have been fetched to cover all dashboards.
	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls for full scan, got %d", callCount)
	}

	// Results matching "system" should appear.
	if !containsString(out, "Special System Overview") {
		t.Errorf("output missing 'Special System Overview'; output:\n%s", out)
	}
	if !containsString(out, "Another System Monitor") {
		t.Errorf("output missing 'Another System Monitor'; output:\n%s", out)
	}

	// Non-matching items should not appear.
	if containsString(out, "Yet Another Board") {
		t.Errorf("output should NOT contain 'Yet Another Board'; output:\n%s", out)
	}

	dashboardsSearchQuery = ""
}

// ============================================================================
// 15. Dashboards search — --json wraps filtered results
// ============================================================================

func TestPaginationDashboardsSearch_JSONMode(t *testing.T) {
	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		writeJSON(w, map[string]interface{}{
			"dashboards": []interface{}{
				makeDashboardItem("j1", "JSON System Board"),
				makeDashboardItem("j2", "Unrelated JSON Board"),
			},
		})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 100
	flagJSON = true
	flagQuiet = true
	dashboardsSearchQuery = "system"

	out := captureStdout(t, func() {
		if err := runDashboardsSearch(nil, nil); err != nil {
			t.Errorf("runDashboardsSearch --json returned error: %v", err)
		}
	})

	result := mustParseJSONMap(t, out)
	arr, ok := result["dashboards"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'dashboards' array; output:\n%s", out)
	}
	if len(arr) != 1 {
		t.Errorf("--json output has %d dashboards, want 1 (filtered by 'system')", len(arr))
	}

	flagJSON = false
	dashboardsSearchQuery = ""
}

// ============================================================================
// 16. Hosts list — count+start offset two pages
//     Page 1: full page (maxHostsPageSize = 1000) → more expected.
//     Page 2: short page (3 items) → signals last page.
//     We use --limit 1500 > 1000 to force pagination.
// ============================================================================

func TestPaginationHostsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	// maxHostsPageSize = 1000 in hosts.go
	const hostPageSize = 1000

	makeHostPage := func(prefix string, n int) []interface{} {
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			items[i] = makeHostItem(fmt.Sprintf("%s-host-%04d", prefix, i))
		}
		return items
	}

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{
				"host_list": makeHostPage("pg1", hostPageSize),
			})
		default:
			writeJSON(w, map[string]interface{}{
				"host_list": makeHostPage("pg2", 3),
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1500
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	hostsListCount = 0
	hostsListStart = 0

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Errorf("runHostsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	for _, name := range []string{"pg1-host-0000", "pg1-host-0999", "pg2-host-0000", "pg2-host-0002"} {
		if !containsString(out, name) {
			t.Errorf("output missing host %s; output (truncated):\n%.500s", name, out)
		}
	}
}

// ============================================================================
// 17. Hosts list — --all flag exhausts all pages
// ============================================================================

func TestPaginationHostsList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	const hostPageSize = 1000

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			items := make([]interface{}, hostPageSize)
			for i := 0; i < hostPageSize; i++ {
				items[i] = makeHostItem(fmt.Sprintf("all-h1-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		case 2:
			items := make([]interface{}, hostPageSize)
			for i := 0; i < hostPageSize; i++ {
				items[i] = makeHostItem(fmt.Sprintf("all-h2-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{
					makeHostItem("all-h3-last"),
				},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all should override
	flagQuiet = true
	hostsListAll = true
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	hostsListCount = 0
	hostsListStart = 0

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Errorf("runHostsList --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, name := range []string{"all-h1-0000", "all-h2-0000", "all-h3-last"} {
		if !containsString(out, name) {
			t.Errorf("output missing host %s with --all; output (truncated):\n%.500s", name, out)
		}
	}

	hostsListAll = false
}

// ============================================================================
// 18. Hosts list — --json merges all pages
// ============================================================================

func TestPaginationHostsList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := newTestServer(t, func(w testResponseWriter, r testRequest, n int32) {
		switch n {
		case 1:
			items := make([]interface{}, maxHostsPageSize)
			for i := 0; i < maxHostsPageSize; i++ {
				items[i] = makeHostItem(fmt.Sprintf("jh1-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{makeHostItem("jh2-0000"), makeHostItem("jh2-0001")},
			})
		}
		atomic.AddInt32(&callCount, 1)
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1100
	flagJSON = true
	flagQuiet = true
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	hostsListCount = 0
	hostsListStart = 0

	out := captureStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Errorf("runHostsList --json returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d", callCount)
	}

	// The JSON output is a plain array (not wrapped in an object).
	var arr []interface{}
	if err := jsonUnmarshalArray(out, &arr); err != nil {
		t.Fatalf("--json output is not a valid JSON array: %v\nOutput:\n%s", err, out)
	}
	wantLen := maxHostsPageSize + 2
	if len(arr) != wantLen {
		t.Errorf("--json mode returned %d items, want %d (all pages merged)", len(arr), wantLen)
	}

	flagJSON = false
}

// ============================================================================
// local helpers (not duplicated from pagination_test.go)
// ============================================================================

type testResponseWriter = http.ResponseWriter
type testRequest = *http.Request

// newTestServer creates an httptest.Server whose handler calls f with an
// atomic call counter (1-based) so tests can branch per page.
func newTestServer(t *testing.T, f func(w testResponseWriter, r testRequest, n int32)) *httptest.Server {
	t.Helper()
	var counter int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&counter, 1)
		f(w, r, n)
	}))
}

// containsString is a convenience wrapper around strings.Contains.
func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// mustParseJSONMap parses s as a JSON object and fatals the test on error.
func mustParseJSONMap(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &result); err != nil {
		t.Fatalf("output is not valid JSON object: %v\nOutput:\n%s", err, s)
	}
	return result
}

// jsonUnmarshalArray parses s as a JSON array.
func jsonUnmarshalArray(s string, out *[]interface{}) error {
	return json.Unmarshal([]byte(strings.TrimSpace(s)), out)
}
