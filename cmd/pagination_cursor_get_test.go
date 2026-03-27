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

// newTestSrv creates an httptest.Server whose handler calls f with an
// atomic 1-based page counter so tests can branch per page number.
// captureStdoutConcurrent is like captureStdout but drains the pipe in a goroutine.
// Use this when output may exceed the 64KB OS pipe buffer (e.g. 100+ item JSON arrays).
func captureStdoutConcurrent(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	f()
	_ = w.Close()
	os.Stdout = orig
	<-done
	return buf.String()
}


func newTestSrv(t *testing.T, f func(w http.ResponseWriter, r *http.Request, n int32)) *httptest.Server {
	t.Helper()
	var counter int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&counter, 1)
		validateRequestAgainstSpec(t, r)
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

// discardStdout redirects os.Stdout to /dev/null while running f().
// Unlike captureStdout, writing to /dev/null never blocks, so this is safe
// for commands that produce large output (e.g. > 64 KB, which would fill the
// os.Pipe kernel buffer used by captureStdout).
func discardStdout(t *testing.T, f func()) {
	t.Helper()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open /dev/null for writing: %v", err)
	}
	defer devNull.Close()
	orig := os.Stdout
	os.Stdout = devNull
	defer func() { os.Stdout = orig }()
	f()
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
// Pagination trigger: pageSize = min(effectiveLimit, maxDashboardsPageSize).
// For the loop to continue after page 1, the page must be "full"
// (len(page) == pageSize) AND len(rows) < effectiveLimit.
//
// Strategy: flagLimit=200 > maxDashboardsPageSize(100), so pageSize=100.
//   Page 1: exactly 100 items (full page, 100 < 200 → loop continues).
//   Page 2: 5 items (short page → loop stops).
// Total output ≈ 105 rows × ~110 chars = ~11 KB — safe for the pipe.
// ============================================================================

func TestPaginationDashboardsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (100 == pageSize=100) → loop must continue.
			items := make([]interface{}, maxDashboardsPageSize)
			for i := 0; i < maxDashboardsPageSize; i++ {
				id := fmt.Sprintf("dp1-%03d", i)
				items[i] = makeDashEntry(id, "Dashboard "+id)
			}
			writeJSON(w, map[string]interface{}{"dashboards": items})
		default:
			// Short page → loop stops.
			writeJSON(w, map[string]interface{}{
				"dashboards": []interface{}{
					makeDashEntry("dp2-00", "Dashboard dp2-00"),
					makeDashEntry("dp2-01", "Dashboard dp2-01"),
					makeDashEntry("dp2-02", "Dashboard dp2-02"),
				},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 200 // effectiveLimit=200 > maxDashboardsPageSize=100 → pageSize=100
	flagJSON = true  // avoid pipe deadlock with 100-item table render
	flagQuiet = true
	dashboardsListAll = false

	out := captureStdout(t, func() {
		if err := runDashboardsList(nil, nil); err != nil {
			t.Errorf("runDashboardsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}
	for _, id := range []string{"dp1-000", "dp1-099", "dp2-00", "dp2-02"} {
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
// For pagination to continue after page 1, two conditions must hold:
//   (a) len(rows) < effectiveLimit  (need more items)
//   (b) len(hostList) == pageSize   (page was full, not the last)
//
// Strategy: flagLimit=1500, pageSize = min(1500, maxHostsPageSize=1000) = 1000.
//   Page 1: exactly 1000 items (full: 1000 < 1500 rows needed → continue).
//   Page 2: 3 items (short: 3 < 1000 → stop).
//
// 1000 items in table output ≈ 80 KB which fills the pipe buffer.
// Use flagJSON=true: each item is ≈55 bytes → 1003 items ≈ 55 KB, safely under 64 KB.
// ============================================================================

func TestPaginationHostsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (1000 == pageSize, 1000 < effectiveLimit=1500) → continue.
			items := make([]interface{}, maxHostsPageSize)
			for i := 0; i < maxHostsPageSize; i++ {
				items[i] = makeHostEntry(fmt.Sprintf("h1-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			// Short page → stop.
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{
					makeHostEntry("h2-000"),
					makeHostEntry("h2-001"),
					makeHostEntry("h2-002"),
				},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1500  // pageSize = min(1500, 1000) = 1000
	flagJSON = true   // avoid table renderer filling pipe buffer
	flagQuiet = true
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	hostsListCount = 0
	hostsListStart = 0

	// Use discardStdout — 1003-item JSON output (~55KB) can fill the 64KB pipe buffer.
	// Verify correctness via HTTP call count only.
	discardStdout(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Errorf("runHostsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}
}

// ============================================================================
// 17. Hosts list — --all flag exhausts three pages
//
// With --all, effectiveLimit=0 and pageSize=maxHostsPageSize=1000.
// Pages 1 and 2 must return exactly 1000 items each (full page) to continue.
// Page 3 returns 1 item (short page) → stops.
//
// 2001 items as JSON (≈55 bytes each) = ≈110 KB which EXCEEDS the 64 KB
// os.Pipe kernel buffer used by captureStdout.  Use discardStdout instead;
// we verify pagination via HTTP call count and not output content.
// ============================================================================

func TestPaginationHostsList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1, 2:
			// Full pages → pagination continues.
			items := make([]interface{}, maxHostsPageSize)
			for i := 0; i < maxHostsPageSize; i++ {
				items[i] = makeHostEntry(fmt.Sprintf("ah%d-%04d", n, i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			// Short page → stops.
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{makeHostEntry("ah3-last")},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all overrides this
	flagJSON = true
	flagQuiet = true
	hostsListAll = true
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	hostsListCount = 0
	hostsListStart = 0

	// Output is 2001 items ≈ 110 KB; use discardStdout to avoid pipe deadlock.
	var runErr error
	discardStdout(t, func() {
		runErr = runHostsList(nil, nil)
	})
	if runErr != nil {
		t.Errorf("runHostsList --all returned error: %v", runErr)
	}

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	hostsListAll = false
	flagJSON = false
}

// ============================================================================
// 18. Hosts list — --json output merges all pages into a flat JSON array
//
// Uses flagLimit=1500 (> maxHostsPageSize=1000) so pageSize=1000.
// Page 1: 1000 items (full page, 1000 < 1500 → continue).
// Page 2: 3 items (short page → stop).
//
// Output is ~75KB of indented JSON, exceeding the captureStdout pipe buffer.
// Use captureStdoutConcurrent (goroutine-drained pipe) and verify item count.
// ============================================================================

func TestPaginationHostsList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := newTestSrv(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, maxHostsPageSize)
			for i := 0; i < maxHostsPageSize; i++ {
				items[i] = makeHostEntry(fmt.Sprintf("jh1-%04d", i))
			}
			writeJSON(w, map[string]interface{}{"host_list": items})
		default:
			writeJSON(w, map[string]interface{}{
				"host_list": []interface{}{
					makeHostEntry("jh2-00"),
					makeHostEntry("jh2-01"),
					makeHostEntry("jh2-02"),
				},
			})
		}
	})
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1500 // pageSize = min(1500, 1000) = 1000; full page → continue
	flagJSON = true
	flagQuiet = true
	hostsListAll = false
	hostsListFilter = ""
	hostsListSortField = ""
	hostsListSortDir = ""
	hostsListCount = 0
	hostsListStart = 0

	out := captureStdoutConcurrent(t, func() {
		if err := runHostsList(nil, nil); err != nil {
			t.Errorf("runHostsList --json returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d", callCount)
	}

	arr := mustParseJSONArr(t, out)
	wantLen := maxHostsPageSize + 3
	if len(arr) != wantLen {
		t.Errorf("--json mode returned %d items, want %d (all pages merged)", len(arr), wantLen)
	}

	flagJSON = false
}
