package cmd

// Pagination regression tests for cursor-POST commands: rum search, audit search, traces search.
//
// rum aggregate and traces aggregate are NOT paginated (single-response APIs).
// These tests are documented as such; we include a smoke test for each to
// confirm they make exactly one HTTP call.
//
// Pattern: httptest.NewServer + DD_API_URL env var + direct run* calls.
// See pagination_test.go for the helper definitions (setMockServer, captureStdout,
// resetFlags, writeJSON).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ---- item factories ---------------------------------------------------------

// makeRumItem returns a minimal RUM event item with a unique ID.
// The outer attributes.timestamp field is used for display;
// inner attributes nested under attributes.attributes holds application/type/etc.
func makeRumItem(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":   "rum-" + id,
		"type": "rum_event",
		"attributes": map[string]interface{}{
			"timestamp": "2024-01-01T00:00:00Z",
			"service":   "test-app",
			"attributes": map[string]interface{}{
				"type": "view",
				"application": map[string]interface{}{
					"name": "app-" + id,
				},
				"action": map[string]interface{}{
					"type": "click",
				},
				"view": map[string]interface{}{
					"name": "view-" + id,
				},
			},
		},
	}
}

// makeAuditItem returns a minimal audit log item with a unique ID.
func makeAuditItem(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":   "audit-" + id,
		"type": "audit",
		"attributes": map[string]interface{}{
			"timestamp": "2024-01-01T00:00:00Z",
			"service":   "audit-svc",
			"attributes": map[string]interface{}{
				"evt": map[string]interface{}{
					"category": "audit",
					"name":     "action-" + id,
				},
				"usr": map[string]interface{}{
					"email": "user-" + id + "@example.com",
				},
			},
		},
	}
}

// makeSpanItem returns a minimal span item with a unique ID.
// Traces search displays attrs["service"] and attrs["resource_name"].
func makeSpanItem(id string) map[string]interface{} {
	return map[string]interface{}{
		"id":   "span-" + id,
		"type": "spans",
		"attributes": map[string]interface{}{
			"service":       "svc-" + id,
			"resource_name": "resource-" + id,
			"start_timestamp": "2024-01-01T00:00:00Z",
		},
	}
}

// cursorResponse builds a standard cursor-POST response envelope.
func cursorResponse(items []interface{}, nextCursor string) map[string]interface{} {
	page := map[string]interface{}{}
	if nextCursor != "" {
		page["after"] = nextCursor
	}
	return map[string]interface{}{
		"data": items,
		"meta": map[string]interface{}{
			"page": page,
		},
	}
}

// ============================================================================
// RUM SEARCH TESTS
// ============================================================================

// ---- 1. Two-page cursor fetch -----------------------------------------------

func TestPaginationRumSearch_TwoPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("r1"), makeRumItem("r2")},
				"rum-cursor-p2",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("r3"), makeRumItem("r4")},
				"", // no more pages
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4 // exceeds single page of 2 → must fetch page 2
	flagQuiet = true
	rumSearchAll = false
	rumSearchQuery = "test"
	rumSearchFrom = "1h"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runRumSearch(nil, nil); err != nil {
			t.Errorf("runRumSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// All 4 items should appear in output (view-r1 .. view-r4)
	for _, id := range []string{"r1", "r2", "r3", "r4"} {
		if !strings.Contains(out, "view-"+id) {
			t.Errorf("output missing rum item %s; output:\n%s", id, out)
		}
	}
}

// ---- 2. --all flag exhausts 3 pages -----------------------------------------

func TestPaginationRumSearch_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("a1"), makeRumItem("a2")},
				"rum-all-cur-2",
			))
		case 2:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("a3"), makeRumItem("a4")},
				"rum-all-cur-3",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("a5"), makeRumItem("a6")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2 // --all should override this
	flagQuiet = true
	rumSearchAll = true
	rumSearchQuery = "test"
	rumSearchFrom = "1h"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runRumSearch(nil, nil); err != nil {
			t.Errorf("runRumSearch --all returned error: %v", err)
		}
	})
	rumSearchAll = false

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []string{"a1", "a2", "a3", "a4", "a5", "a6"} {
		if !strings.Contains(out, "view-"+id) {
			t.Errorf("output missing rum item %s with --all; output:\n%s", id, out)
		}
	}
}

// ---- 3. --json mode merges all pages (DDOG-50 regression) -------------------

func TestPaginationRumSearch_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("j1"), makeRumItem("j2")},
				"rum-json-cursor-p2",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeRumItem("j3"), makeRumItem("j4")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	flagJSON = true
	flagQuiet = true
	rumSearchAll = false
	rumSearchQuery = "test"
	rumSearchFrom = "1h"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runRumSearch(nil, nil); err != nil {
			t.Errorf("runRumSearch --json returned error: %v", err)
		}
	})
	flagJSON = false

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d; early-exit regression?", callCount)
	}

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
}

// ---- 4. --limit at page boundary stops after exactly 1 HTTP call -----------

func TestPaginationRumSearch_LimitAtPageBoundary_StopsEarly(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		// Always return 2 items with a cursor (would continue if not for limit)
		writeJSON(w, cursorResponse(
			[]interface{}{makeRumItem("b1"), makeRumItem("b2")},
			"rum-would-continue",
		))
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2 // limit == page size → should stop after 1 call
	flagQuiet = true
	rumSearchAll = false
	rumSearchQuery = "test"
	rumSearchFrom = "1h"
	rumSearchTo = "now"
	rumSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runRumSearch(nil, nil); err != nil {
			t.Errorf("runRumSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call (limit == page size), got %d", callCount)
	}

	for _, id := range []string{"b1", "b2"} {
		if !strings.Contains(out, "view-"+id) {
			t.Errorf("output missing rum item %s; output:\n%s", id, out)
		}
	}
}

// ============================================================================
// AUDIT SEARCH TESTS
// ============================================================================

// ---- 5. Two-page cursor fetch -----------------------------------------------

func TestPaginationAuditSearch_TwoPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("e1"), makeAuditItem("e2")},
				"audit-cursor-p2",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("e3"), makeAuditItem("e4")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	flagQuiet = true
	auditSearchAll = false
	auditSearchQuery = "*"
	auditSearchFrom = "1h"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runAuditSearch(nil, nil); err != nil {
			t.Errorf("runAuditSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// action-eN appears in the output (truncated to 25 chars, all fit)
	for _, id := range []string{"e1", "e2", "e3", "e4"} {
		if !strings.Contains(out, "action-"+id) {
			t.Errorf("output missing audit item %s; output:\n%s", id, out)
		}
	}
}

// ---- 6. --all flag exhausts 3 pages -----------------------------------------

func TestPaginationAuditSearch_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("f1"), makeAuditItem("f2")},
				"audit-all-cur-2",
			))
		case 2:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("f3"), makeAuditItem("f4")},
				"audit-all-cur-3",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("f5"), makeAuditItem("f6")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2 // --all overrides
	flagQuiet = true
	auditSearchAll = true
	auditSearchQuery = "*"
	auditSearchFrom = "1h"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runAuditSearch(nil, nil); err != nil {
			t.Errorf("runAuditSearch --all returned error: %v", err)
		}
	})
	auditSearchAll = false

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []string{"f1", "f2", "f3", "f4", "f5", "f6"} {
		if !strings.Contains(out, "action-"+id) {
			t.Errorf("output missing audit item %s with --all; output:\n%s", id, out)
		}
	}
}

// ---- 7. --json mode merges all pages (DDOG-50 regression) ------------------

func TestPaginationAuditSearch_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("g1"), makeAuditItem("g2")},
				"audit-json-cursor-p2",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeAuditItem("g3"), makeAuditItem("g4")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	flagJSON = true
	flagQuiet = true
	auditSearchAll = false
	auditSearchQuery = "*"
	auditSearchFrom = "1h"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runAuditSearch(nil, nil); err != nil {
			t.Errorf("runAuditSearch --json returned error: %v", err)
		}
	})
	flagJSON = false

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d; early-exit regression?", callCount)
	}

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
}

// ---- 8. --limit at page boundary stops after exactly 1 HTTP call -----------

func TestPaginationAuditSearch_LimitAtPageBoundary_StopsEarly(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, cursorResponse(
			[]interface{}{makeAuditItem("c1"), makeAuditItem("c2")},
			"audit-would-continue",
		))
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2
	flagQuiet = true
	auditSearchAll = false
	auditSearchQuery = "*"
	auditSearchFrom = "1h"
	auditSearchTo = "now"
	auditSearchSort = "-timestamp"

	out := captureStdout(t, func() {
		if err := runAuditSearch(nil, nil); err != nil {
			t.Errorf("runAuditSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call (limit == page size), got %d", callCount)
	}

	for _, id := range []string{"c1", "c2"} {
		if !strings.Contains(out, "action-"+id) {
			t.Errorf("output missing audit item %s; output:\n%s", id, out)
		}
	}
}

// ============================================================================
// TRACES SEARCH TESTS
//
// Note: traces search uses a nested body: data.attributes.page.cursor
// (not top-level page.cursor). The implementation updates attributes["page"]
// in place, so the mock server just needs to return the standard envelope.
// ============================================================================

// ---- 9. Two-page cursor fetch -----------------------------------------------

func TestPaginationTracesSearch_TwoPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("s1"), makeSpanItem("s2")},
				"spans-cursor-p2",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("s3"), makeSpanItem("s4")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	flagQuiet = true
	tracesSearchAll = false
	tracesSearchQuery = "service:test"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""

	out := captureStdout(t, func() {
		if err := runTracesSearch(nil, nil); err != nil {
			t.Errorf("runTracesSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// resource_name field shows "resource-sN" in table output
	for _, id := range []string{"s1", "s2", "s3", "s4"} {
		if !strings.Contains(out, "resource-"+id) {
			t.Errorf("output missing span %s; output:\n%s", id, out)
		}
	}
}

// ---- 10. --all flag exhausts 3 pages ----------------------------------------

func TestPaginationTracesSearch_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("t1"), makeSpanItem("t2")},
				"spans-all-cur-2",
			))
		case 2:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("t3"), makeSpanItem("t4")},
				"spans-all-cur-3",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("t5"), makeSpanItem("t6")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2 // --all overrides
	flagQuiet = true
	tracesSearchAll = true
	tracesSearchQuery = "service:test"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""

	out := captureStdout(t, func() {
		if err := runTracesSearch(nil, nil); err != nil {
			t.Errorf("runTracesSearch --all returned error: %v", err)
		}
	})
	tracesSearchAll = false

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []string{"t1", "t2", "t3", "t4", "t5", "t6"} {
		if !strings.Contains(out, "resource-"+id) {
			t.Errorf("output missing span %s with --all; output:\n%s", id, out)
		}
	}
}

// ---- 11. --json mode merges all pages (DDOG-50 regression) -----------------

func TestPaginationTracesSearch_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("u1"), makeSpanItem("u2")},
				"spans-json-cursor-p2",
			))
		default:
			writeJSON(w, cursorResponse(
				[]interface{}{makeSpanItem("u3"), makeSpanItem("u4")},
				"",
			))
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 4
	flagJSON = true
	flagQuiet = true
	tracesSearchAll = false
	tracesSearchQuery = "service:test"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""

	out := captureStdout(t, func() {
		if err := runTracesSearch(nil, nil); err != nil {
			t.Errorf("runTracesSearch --json returned error: %v", err)
		}
	})
	flagJSON = false

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("--json mode: expected at least 2 HTTP calls, got %d; early-exit regression?", callCount)
	}

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
}

// ---- 12. --limit at page boundary stops after exactly 1 HTTP call ----------

func TestPaginationTracesSearch_LimitAtPageBoundary_StopsEarly(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		writeJSON(w, cursorResponse(
			[]interface{}{makeSpanItem("v1"), makeSpanItem("v2")},
			"spans-would-continue",
		))
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 2 // limit == page size → exactly 1 HTTP call
	flagQuiet = true
	tracesSearchAll = false
	tracesSearchQuery = "service:test"
	tracesSearchFrom = "1h"
	tracesSearchTo = "now"
	tracesSearchSort = ""
	tracesSearchFilterQuery = ""

	out := captureStdout(t, func() {
		if err := runTracesSearch(nil, nil); err != nil {
			t.Errorf("runTracesSearch returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call (limit == page size), got %d", callCount)
	}

	for _, id := range []string{"v1", "v2"} {
		if !strings.Contains(out, "resource-"+id) {
			t.Errorf("output missing span %s; output:\n%s", id, out)
		}
	}
}

// ============================================================================
// NON-PAGINATED SMOKE TESTS
//
// traces aggregate and rum aggregate use single-request APIs (no cursor loop).
// These tests confirm exactly 1 HTTP call is made.
// ============================================================================

// ---- 13. traces aggregate — single HTTP call --------------------------------

func TestTracesAggregate_SingleCall(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		// Response matches traces aggregate schema: {"data": [...buckets...]}
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{
					"type": "bucket",
					"attributes": map[string]interface{}{
						"by": map[string]interface{}{
							"service": "my-svc",
						},
						"compute": map[string]interface{}{
							"c0": float64(42),
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagQuiet = true
	tracesAggQuery = "env:production"
	tracesAggFrom = "1h"
	tracesAggTo = "now"
	tracesAggCompute = "count"
	tracesAggGroupBy = "service"

	_ = captureStdout(t, func() {
		if err := runTracesAggregate(nil, nil); err != nil {
			t.Errorf("runTracesAggregate returned error: %v", err)
		}
	})

	if n := atomic.LoadInt32(&callCount); n != 1 {
		t.Errorf("traces aggregate: expected exactly 1 HTTP call (not paginated), got %d", n)
	}
}

// ---- 14. rum aggregate — single HTTP call -----------------------------------

func TestRumAggregate_SingleCall(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		// Response matches rum aggregate schema: {"data": {"buckets": [...]}}
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"buckets": []interface{}{
					map[string]interface{}{
						"by": map[string]interface{}{
							"@type": "view",
						},
						"computes": map[string]interface{}{
							"c0": float64(100),
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagQuiet = true
	rumAggQuery = "*"
	rumAggFrom = "1h"
	rumAggTo = "now"
	rumAggCompute = "count"
	rumAggGroupBy = "@type"

	_ = captureStdout(t, func() {
		if err := runRumAggregate(nil, nil); err != nil {
			t.Errorf("runRumAggregate returned error: %v", err)
		}
	})

	if n := atomic.LoadInt32(&callCount); n != 1 {
		t.Errorf("rum aggregate: expected exactly 1 HTTP call (not paginated), got %d", n)
	}
}
