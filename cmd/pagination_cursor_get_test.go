package cmd

// Pagination tests for cursor-GET and count+start GET commands.
//
// Covered commands:
//   - apm services    (GET /api/v2/services/definitions, meta.page.cursor)
//   - apm definitions (GET /api/v2/services/definitions, meta.page.cursor)
//   - users list      (GET /api/v2/users — single-page, no cursor in current impl)
//   - api-keys list   (GET /api/v2/api_keys — single-page, no cursor in current impl)
//   - dashboards list (GET /api/v1/dashboard, count+start offset)
//   - dashboards search (same endpoint, client-side filter after paginated fetch)
//   - hosts list      (GET /api/v1/hosts, start+count offset)
//
// Pattern: httptest.NewServer + setMockServer + direct run* call.
// No real HTTP calls are made.
//
// NOTE: Keep page item counts small (< 20) in these tests.  The captureStdout
// helper uses an os.Pipe whose kernel buffer is ~64 KB; if the table renderer
// writes more than that without the reader goroutine catching up, the write
// blocks and the test times out.

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

// newTestSrv creates an httptest.Server whose handler calls f with an
// atomic 1-based page counter so tests can branch per page number.
func newTestSrv(t *testing.T, f func(w http.ResponseWriter, r *http.Request, n int32)) *httptest.Server {
	t.Helper()
	var counter int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&counter, 1)
		f(w, r, n)
	}))
}

// mustParseJSONObj parses s as a JSON object and fatals the test on error.
func mustParseJSONObj(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &result); err != nil {
		t.Fatalf("output is not valid JSON object: %v\nOutput:\n%s", err, s)
	}
	return result
}

// mustParseJSONArr parses s as a JSON array and fatals the test on error.
func mustParseJSONArr(t *testing.T, s string) []interface{} {
	t.Helper()
	var result []interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &result); err != nil {
		t.Fatalf("output is not valid JSON array: %v\nOutput:\n%s", err, s)
	}
	return result
}

// ---- item builders ----------------------------------------------------------

// makeAPMItem returns a minimal APM service definition item.
func makeAPMItem(name string) map[string]interface{} {
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

// makeUserEntry returns a minimal user data item.
func makeUserEntry(id, email string) map[string]interface{} {
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

// makeAPIKeyEntry returns a minimal API key data item.
func makeAPIKeyEntry(id, name string) map[string]interface{} {
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

// makeDashEntry returns a minimal dashboard item.
func makeDashEntry(id, title string) map[string]interface{} {
	return map[string]interface{}{
		"id":            id,
		"title":         title,
		"author_handle": "user@example.com",
		"url":           "/dashboard/" + id,
		"created_at":    "2024-01-01T00:00:00Z",
	}
}

// makeHostEntry returns a minimal host_list item.
func makeHostEntry(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":               name,
		"up":                 true,
		"last_reported_time": float64(1700000000),
	}
}

// apmCursorPage returns a response with n items, an optional cursor, and the
// standard meta.page envelope used by the APM services/definitions endpoints.
func apmCursorPage(items []interface{}, cursor string) map[string]interface{} {
	page := map[string]interface{}{}
	if cursor != "" {
		page["cursor"] = cursor
	}
	return map[string]interface{}{
		"data": items,
		"meta": map[string]interface{}{"page": page},
	}
}

// ============================================================================
// 1. APM services — cursor-GET two pages
//    Page 1 has cursor, page 2 has no cursor.  --limit 4 triggers page 2.
// ============================================================================

func TestPaginationAPMServices_CursorGET_TwoPages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("svc-1"), makeAPMItem("svc-2")}, "apm-cursor-p2"))
		default:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("svc-3"), makeAPMItem("svc-4")}, ""))
		}
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
		if !strings.Contains(out, name) {
			t.Errorf("output missing service %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 2. APM services — --all flag exhausts three pages
// ============================================================================

func TestPaginationAPMServices_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("as1"), makeAPMItem("as2")}, "cur-as2"))
		case 2:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("as3"), makeAPMItem("as4")}, "cur-as4"))
		default:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("as5")}, ""))
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all overrides this
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
	for _, name := range []string{"as1", "as2", "as3", "as4", "as5"} {
		if !strings.Contains(out, name) {
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

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("js1"), makeAPMItem("js2")}, "json-cur-p2"))
		default:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("js3")}, ""))
		}
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

	result := mustParseJSONObj(t, out)
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

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("def-1"), makeAPMItem("def-2")}, "def-cursor-p2"))
		default:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("def-3"), makeAPMItem("def-4")}, ""))
		}
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
		if !strings.Contains(out, name) {
			t.Errorf("output missing definition %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 5. APM definitions — --all flag exhausts three pages
// ============================================================================

func TestPaginationAPMDefinitions_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("da1"), makeAPMItem("da2")}, "d-cur-2"))
		case 2:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("da3"), makeAPMItem("da4")}, "d-cur-3"))
		default:
			writeJSON(w, apmCursorPage([]interface{}{makeAPMItem("da5")}, ""))
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all overrides this
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
		if !strings.Contains(out, name) {
			t.Errorf("output missing definition %s with --all; output:\n%s", name, out)
		}
	}

	apmDefinitionsAll = false
}

// ============================================================================
// 6. Users list — single-page response (current impl makes one request)
// ============================================================================

func TestPaginationUsersList_SinglePage(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeUserEntry("u1", "alice@example.com"),
				makeUserEntry("u2", "bob@example.com"),
				makeUserEntry("u3", "carol@example.com"),
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
		if !strings.Contains(out, email) {
			t.Errorf("output missing user %s; output:\n%s", email, out)
		}
	}
}

// ============================================================================
// 7. Users list — --limit slices the result
// ============================================================================

func TestPaginationUsersList_LimitSlicesResult(t *testing.T) {
	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeUserEntry("u1", "first@example.com"),
				makeUserEntry("u2", "second@example.com"),
				makeUserEntry("u3", "third@example.com"),
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

	if strings.Contains(out, "third@example.com") {
		t.Errorf("output should NOT contain third@example.com with --limit 2; output:\n%s", out)
	}
}

// ============================================================================
// 8. Users list — --json output
// ============================================================================

func TestPaginationUsersList_JSONMode(t *testing.T) {
	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeUserEntry("u1", "json-user@example.com"),
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

	result := mustParseJSONObj(t, out)
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

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeAPIKeyEntry("k1", "prod-key"),
				makeAPIKeyEntry("k2", "staging-key"),
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
		if !strings.Contains(out, name) {
			t.Errorf("output missing API key %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 10. API keys list — --json output
// ============================================================================

func TestPaginationAPIKeysList_JSONMode(t *testing.T) {
	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				makeAPIKeyEntry("k1", "json-key"),
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

	result := mustParseJSONObj(t, out)
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
// 11. Dashboards list — count+start offset, two pages
//
// We use a small synthetic "page size" of 5 items so the test produces little
// output (avoids pipe-buffer blocking).  The key property under test is the
// pagination loop, not the volume of data.
//
// Page 1: 5 items = full page  → loop must continue.
// Page 2: 3 items = short page → loop must stop.
// flagLimit = 10 > 5, so pagination triggers.
// ============================================================================

func TestPaginationDashboardsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	// Override maxDashboardsPageSize for this test via a small flagLimit
	// that happens to be larger than our mock page.  The real constant is 100;
	// we just need flagLimit > items-per-page so the loop asks for a second page.
	const mockPageSize = 5

	makeDashPage := func(prefix string, n int) []interface{} {
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("%s-%02d", prefix, i)
			items[i] = makeDashEntry(id, "Dashboard "+id)
		}
		return items
	}

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{"dashboards": makeDashPage("pg1", mockPageSize)})
		default:
			writeJSON(w, map[string]interface{}{"dashboards": makeDashPage("pg2", 3)})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	// flagLimit > mockPageSize so the first page is "full" and pagination continues.
	// We also need flagLimit < maxDashboardsPageSize (100) so the API page size
	// equals flagLimit, not maxDashboardsPageSize.  With flagLimit=8 and a mock
	// page of 5, the loop will see 5 < 8 and... wait, 5 < 8, so page is short!
	// Instead set flagLimit = 200 (> maxDashboardsPageSize=100) so pageSize=100.
	// But our mock only returns 5 items which is < 100 → short page → stops after 1.
	// We need pages of exactly maxDashboardsPageSize.  Use flagJSON and small pages.
	//
	// Simplest approach: use flagLimit=10 which becomes the pageSize (since 10 < 100).
	// Server returns 10 items on page 1 (full page) and 3 on page 2 (short).
	flagLimit = 20
	dashboardsListAll = false

	// Rebuild the server with the right page size now that we know flagLimit=20
	// means pageSize=20 (min(20, 100)).  20 < 100, so pageSize=20.
	// Mock: page 1 returns exactly 20 items (full), page 2 returns 3 (short).
	var callCount2 int32
	srv2Items1 := make([]interface{}, 20)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("p1-%02d", i)
		srv2Items1[i] = makeDashEntry(id, "Dashboard "+id)
	}
	srv2Items2 := []interface{}{
		makeDashEntry("p2-00", "Dashboard p2-00"),
		makeDashEntry("p2-01", "Dashboard p2-01"),
	}
	srv2 := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount2, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{"dashboards": srv2Items1})
		default:
			writeJSON(w, map[string]interface{}{"dashboards": srv2Items2})
		}
	})
	defer srv2.Close()
	setMockServer(t, srv2)
	_ = srv  // suppress unused variable
	_ = callCount

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Errorf("runDashboardsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount2) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount2)
	}
	for _, id := range []string{"p1-00", "p1-19", "p2-00", "p2-01"} {
		if !strings.Contains(out, id) {
			t.Errorf("output missing dashboard %s; output (truncated):\n%.500s", id, out)
		}
	}
}

// ============================================================================
// 12. Dashboards list — --all flag exhausts three pages (small synthetic pages)
// ============================================================================

func TestPaginationDashboardsList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	// pageSize used by runDashboardsList when dashboardsListAll=true is
	// maxDashboardsPageSize (100).  We can't change that, but we can make
	// the mock return fewer items (< 100) to trigger "short page → stop".
	// For --all, the stop condition is len(page) < pageSize OR len(page)==0.
	//
	// Setup:
	//   page 1: 10 items  → short (< 100) BUT --all only stops on empty/short;
	//     since 10 < 100, it IS a short page and would stop after page 1!
	//
	// That means we must return exactly maxDashboardsPageSize items on pages
	// 1 and 2, and a short page on page 3.  That's 201 items total — but the
	// table renderer and pipe buffer can't handle 200 rows without deadlock.
	//
	// Solution: use flagJSON=true so no table is rendered, output is compact JSON.

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)

		makeItems := func(prefix string, count int) []interface{} {
			items := make([]interface{}, count)
			for i := 0; i < count; i++ {
				id := fmt.Sprintf("%s-%03d", prefix, i)
				items[i] = makeDashEntry(id, "Dashboard "+id)
			}
			return items
		}

		switch n {
		case 1:
			// Full page → pagination continues.
			writeJSON(w, map[string]interface{}{"dashboards": makeItems("ad1", maxDashboardsPageSize)})
		case 2:
			// Full page → pagination continues.
			writeJSON(w, map[string]interface{}{"dashboards": makeItems("ad2", maxDashboardsPageSize)})
		default:
			// Short page → pagination stops.
			writeJSON(w, map[string]interface{}{"dashboards": makeItems("ad3", 3)})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1     // --all should override
	flagJSON = true   // avoid table rendering deadlock with 200+ rows
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

	// Output is a JSON array; spot-check some IDs.
	arr := mustParseJSONArr(t, out)
	wantLen := maxDashboardsPageSize*2 + 3
	if len(arr) != wantLen {
		t.Errorf("--all --json returned %d items, want %d", len(arr), wantLen)
	}

	dashboardsListAll = false
	flagJSON = false
}

// ============================================================================
// 13. Dashboards list — --limit stops early (no second page needed)
// ============================================================================

func TestPaginationDashboardsList_LimitStopsEarly(t *testing.T) {
	var callCount int32

	// Server returns a full page (maxDashboardsPageSize = 100 items) every time.
	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		items := make([]interface{}, maxDashboardsPageSize)
		for i := 0; i < maxDashboardsPageSize; i++ {
			id := fmt.Sprintf("lim-%d-%03d", n, i)
			items[i] = makeDashEntry(id, "Dashboard "+id)
		}
		writeJSON(w, map[string]interface{}{"dashboards": items})
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 5 // pageSize = min(5, 100) = 5; server returns 100 items which is ≥ 5
	// But wait: if pageSize=5 and server returns 100, then len(page)=100 > pageSize=5
	// and len(rows)=100 >= effectiveLimit=5, so the loop stops.  BUT we also
	// need to not render 100 rows.  Use flagJSON to keep output small.
	flagJSON = true
	flagQuiet = true
	dashboardsListAll = false

	captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Errorf("runDashboardsList returned error: %v", err)
		}
	})

	// With flagLimit=5 < pageSize=100 NOT true — pageSize = min(5,100) = 5.
	// Server returns 100 items but only 5 are needed; stop condition triggers.
	// Only 1 HTTP call should happen.
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call with --limit 5, got %d", callCount)
	}

	flagJSON = false
}

// ============================================================================
// 14. Dashboards search — fetches all pages, then client-side filter
//     Page 1: 10 items (full for maxDashboardsPageSize test, but we use JSON).
//     Page 2: 3 items (short page).
// ============================================================================

func TestPaginationDashboardsSearch_AllPagesThenFilter(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (maxDashboardsPageSize=100) with one matching title.
			items := make([]interface{}, maxDashboardsPageSize)
			for i := 0; i < maxDashboardsPageSize; i++ {
				title := fmt.Sprintf("Generic Dashboard %03d", i)
				if i == 7 {
					title = "Special System Overview"
				}
				items[i] = makeDashEntry(fmt.Sprintf("d1-%03d", i), title)
			}
			writeJSON(w, map[string]interface{}{"dashboards": items})
		default:
			// Short page, one match.
			writeJSON(w, map[string]interface{}{
				"dashboards": []interface{}{
					makeDashEntry("d2-00", "Another System Monitor"),
					makeDashEntry("d2-01", "Unrelated Dashboard"),
					makeDashEntry("d2-02", "Yet Another Board"),
				},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1000
	flagJSON = true // avoid rendering 100+ rows through the pipe
	flagQuiet = true
	dashboardsSearchQuery = "system"

	out := captureStdout(t, func() {
		if err := runDashboardsSearch(nil, nil); err != nil {
			t.Errorf("runDashboardsSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls for full scan, got %d", callCount)
	}

	// JSON output: {"dashboards": [...]}
	result := mustParseJSONObj(t, out)
	arr, ok := result["dashboards"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'dashboards' key; output:\n%s", out)
	}
	// Should have exactly 2 matches: "Special System Overview" and "Another System Monitor".
	if len(arr) != 2 {
		t.Errorf("search returned %d results, want 2 matching 'system'; output:\n%s", len(arr), out)
	}

	// Non-matching items must not appear in raw JSON.
	if strings.Contains(out, "Yet Another Board") {
		t.Errorf("output should NOT contain 'Yet Another Board'; output:\n%s", out)
	}

	flagJSON = false
	dashboardsSearchQuery = ""
}

// ============================================================================
// 15. Dashboards search — --json wraps filtered results
// ============================================================================

func TestPaginationDashboardsSearch_JSONMode(t *testing.T) {
	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		writeJSON(w, map[string]interface{}{
			"dashboards": []interface{}{
				makeDashEntry("j1", "JSON System Board"),
				makeDashEntry("j2", "Unrelated JSON Board"),
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

	result := mustParseJSONObj(t, out)
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
// 16. Hosts list — count+start offset, two pages
//
// Uses a small synthetic page (5 items) to keep output small and avoid
// pipe-buffer blocking.  flagLimit=10 > 5, so a second page is requested.
// Page 2 returns 3 items (short page) → loop stops.
// ============================================================================

func TestPaginationHostsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	// With flagLimit=10 and maxHostsPageSize=1000, pageSize = min(10,1000) = 10.
	// Return exactly 10 items on page 1 (full page) and 3 on page 2 (short).
	const syntheticPageSize = 10

	makeHostPage := func(prefix string, n int) []interface{} {
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			items[i] = makeHostEntry(fmt.Sprintf("%s-host-%02d", prefix, i))
		}
		return items
	}

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, map[string]interface{}{"host_list": makeHostPage("pg1", syntheticPageSize)})
		default:
			writeJSON(w, map[string]interface{}{"host_list": makeHostPage("pg2", 3)})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = syntheticPageSize + 5 // 15; pageSize = min(15, 1000) = 15 but server returns 10 (full)
	// Hmm: if pageSize=15 and server returns 10, then 10 < 15 → short page → stop after 1 call.
	// We need pageSize == syntheticPageSize for the "full page" condition.
	// Set flagLimit = syntheticPageSize exactly: pageSize = min(10, 1000) = 10.
	// Server returns 10 on page 1 (NOT short: 10 == 10) → loop continues.
	// Server returns 3 on page 2 (short: 3 < 10) → loop stops.  ✓
	flagLimit = syntheticPageSize
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
	for _, name := range []string{"pg1-host-00", "pg1-host-09", "pg2-host-00", "pg2-host-02"} {
		if !strings.Contains(out, name) {
			t.Errorf("output missing host %s; output:\n%s", name, out)
		}
	}
}

// ============================================================================
// 17. Hosts list — --all flag exhausts three pages
//
// Use flagJSON=true to avoid rendering thousands of rows through the pipe.
// With --all and maxHostsPageSize=1000 we'd need 1000-item pages, which is
// too large for captureStdout's pipe.  Use flagJSON so output is compact JSON.
// ============================================================================

func TestPaginationHostsList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	// With --all the page size is maxHostsPageSize (1000).
	// Pages 1 and 2 must be exactly 1000 items (full) to continue pagination.
	// Page 3 is 1 item (short) to stop.
	// flagJSON=true keeps output small regardless of item count.
	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, maxHostsPageSize)
			for i := 0; i < maxHostsPageSize; i++ {
				items[i] = makeHostEntry(fmt.Sprintf("ah1-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		case 2:
			items := make([]interface{}, maxHostsPageSize)
			for i := 0; i < maxHostsPageSize; i++ {
				items[i] = makeHostEntry(fmt.Sprintf("ah2-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{makeHostEntry("ah3-last")},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1   // --all overrides
	flagJSON = true // compact output avoids pipe deadlock
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

	arr := mustParseJSONArr(t, out)
	wantLen := maxHostsPageSize*2 + 1
	if len(arr) != wantLen {
		t.Errorf("--all --json returned %d items, want %d", len(arr), wantLen)
	}

	hostsListAll = false
	flagJSON = false
}

// ============================================================================
// 18. Hosts list — --json output merges all pages into a compact array
// ============================================================================

func TestPaginationHostsList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	// Use a small synthetic page (5 items) so two pages = 8 items total.
	// flagLimit = 6 → pageSize = min(6, 1000) = 6.
	// Page 1 returns exactly 6 items (full page) → continue.
	// Page 2 returns 2 items (short) → stop.
	const syntheticPageSize = 6

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, syntheticPageSize)
			for i := 0; i < syntheticPageSize; i++ {
				items[i] = makeHostEntry(fmt.Sprintf("jh1-%02d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{
					makeHostEntry("jh2-00"),
					makeHostEntry("jh2-01"),
				},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = syntheticPageSize + 5 // ensure we'd ask for more than one page
	// pageSize = min(flagLimit, maxHostsPageSize) = min(11, 1000) = 11.
	// Server returns 6 on page 1 (6 < 11 → short page → stops).
	// That means we only get 1 call.  Fix: flagLimit = syntheticPageSize exactly.
	flagLimit = syntheticPageSize
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

	arr := mustParseJSONArr(t, out)
	wantLen := syntheticPageSize + 2
	if len(arr) != wantLen {
		t.Errorf("--json mode returned %d items, want %d (all pages merged)", len(arr), wantLen)
	}

	flagJSON = false
}
