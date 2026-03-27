package cmd

// Pagination regression tests for offset-based commands:
// downtimes list, incidents list, notebooks list, events list.
//
// Pattern: httptest.NewServer + DD_API_URL env var + direct run* calls.
// See pagination_test.go for helper definitions (setMockServer, captureStdout,
// resetFlags, writeJSON).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// ============================================================================
// Downtimes list — offset-based (page[limit] + page[offset])
// Termination: short page (len(data) < thisPageSize).
// ============================================================================

// makeDowntimeItem returns a minimal downtime item map with a unique numeric ID.
func makeDowntimeItem(id int) map[string]interface{} {
	return map[string]interface{}{
		"id":   fmt.Sprintf("downtime-%04d", id),
		"type": "downtime",
		"attributes": map[string]interface{}{
			"message": fmt.Sprintf("downtime-msg-%04d", id),
			"status":  "active",
			"monitor_identifier": map[string]interface{}{
				"monitor_tags": []interface{}{"env:test"},
			},
			"schedule": map[string]interface{}{
				"start": "2024-01-01T00:00:00Z",
				"end":   "2024-01-02T00:00:00Z",
			},
		},
	}
}

// TestPaginationDowntimesList_OffsetBased_TwoPages verifies that a second HTTP
// call is made when page 1 is full-sized and page 2 is a short page.
func TestPaginationDowntimesList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	makeDowntimePage := func(startID, n int) []interface{} {
		page := make([]interface{}, n)
		for i := 0; i < n; i++ {
			page[i] = makeDowntimeItem(startID + i)
		}
		return page
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (100 items = maxDowntimesPageSize) → pagination continues.
			writeJSON(w, map[string]interface{}{
				"data": makeDowntimePage(1, 100),
			})
		default:
			// Short page (3 items < 100) → signals last page.
			writeJSON(w, map[string]interface{}{
				"data": makeDowntimePage(101, 3),
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	// Use a limit larger than one page so the loop must fetch page 2.
	flagLimit = 200
	downtimesListAll = false

	out := captureStdout(t, func() {
		err := runDowntimesList(nil, nil)
		if err != nil {
			t.Errorf("runDowntimesList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// Spot-check items from both pages.
	for _, id := range []int{1, 50, 100, 101, 103} {
		want := fmt.Sprintf("downtime-%04d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s; output (truncated):\n%.500s", want, out)
		}
	}
}

// TestPaginationDowntimesList_AllFlag_ThreePages verifies that --all exhausts all
// pages (3 pages until the last short page) ignoring --limit.
func TestPaginationDowntimesList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeDowntimeItem(i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		case 2:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeDowntimeItem(100 + i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		default:
			// Short page → last page.
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeDowntimeItem(201), makeDowntimeItem(202)},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all should override this
	flagQuiet = true
	downtimesListAll = true

	out := captureStdout(t, func() {
		err := runDowntimesList(nil, nil)
		if err != nil {
			t.Errorf("runDowntimesList --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	// Items from all three pages must appear.
	for _, id := range []int{1, 100, 101, 200, 201, 202} {
		want := fmt.Sprintf("downtime-%04d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s with --all; output (truncated):\n%.500s", want, out)
		}
	}

	downtimesListAll = false
}

// TestPaginationDowntimesList_JSONMode_MergesAllPages verifies that --json mode
// fetches all pages and merges them into a single "data" array.
func TestPaginationDowntimesList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeDowntimeItem(i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeDowntimeItem(101), makeDowntimeItem(102)},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 200
	flagJSON = true
	flagQuiet = true
	downtimesListAll = false

	out := captureStdout(t, func() {
		err := runDowntimesList(nil, nil)
		if err != nil {
			t.Errorf("runDowntimesList --json returned error: %v", err)
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

	if len(dataArr) != 102 {
		t.Errorf("--json mode returned %d items, want 102 (all pages merged)", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// Incidents list — offset-based (page[size] + page[offset])
// Termination: short page (len(data) < pageSize).
// ============================================================================

// makeIncidentItem returns a minimal incident item map with a unique ID.
func makeIncidentItem(id int) map[string]interface{} {
	return map[string]interface{}{
		"id":   fmt.Sprintf("incident-%04d", id),
		"type": "incidents",
		"attributes": map[string]interface{}{
			"public_id": float64(id),
			"title":     fmt.Sprintf("incident-title-%04d", id),
			"severity":  "SEV-3",
			"state":     "resolved",
			"created":   "2024-01-01T00:00:00Z",
		},
	}
}

// TestPaginationIncidentsList_OffsetBased_TwoPages verifies that pagination
// continues when page 1 is full and stops on a short page 2.
func TestPaginationIncidentsList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	makeIncidentPage := func(startID, n int) []interface{} {
		page := make([]interface{}, n)
		for i := 0; i < n; i++ {
			page[i] = makeIncidentItem(startID + i)
		}
		return page
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (100 items = maxIncidentsPageSize) → pagination continues.
			writeJSON(w, map[string]interface{}{
				"data": makeIncidentPage(1, 100),
			})
		default:
			// Short page (4 items < 100) → signals last page.
			writeJSON(w, map[string]interface{}{
				"data": makeIncidentPage(101, 4),
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 200
	incidentsListAll = false

	out := captureStdout(t, func() {
		err := runIncidentsList(nil, nil)
		if err != nil {
			t.Errorf("runIncidentsList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// Spot-check items from both pages via public_id which appears in output.
	for _, id := range []int{1, 50, 100, 101, 104} {
		want := fmt.Sprintf("%d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing incident id %d; output (truncated):\n%.500s", id, out)
		}
	}
}

// TestPaginationIncidentsList_AllFlag_ThreePages verifies that --all exhausts
// all pages regardless of --limit.
func TestPaginationIncidentsList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeIncidentItem(i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		case 2:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeIncidentItem(100 + i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeIncidentItem(201), makeIncidentItem(202), makeIncidentItem(203)},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all must override this
	flagQuiet = true
	incidentsListAll = true

	out := captureStdout(t, func() {
		err := runIncidentsList(nil, nil)
		if err != nil {
			t.Errorf("runIncidentsList --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []int{1, 100, 101, 200, 201, 203} {
		want := fmt.Sprintf("%d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing incident id %d with --all; output (truncated):\n%.500s", id, out)
		}
	}

	incidentsListAll = false
}

// TestPaginationIncidentsList_JSONMode_MergesAllPages verifies that --json mode
// returns a merged array from all pages.
func TestPaginationIncidentsList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeIncidentItem(i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeIncidentItem(101), makeIncidentItem(102), makeIncidentItem(103)},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 200
	flagJSON = true
	flagQuiet = true
	incidentsListAll = false

	out := captureStdout(t, func() {
		err := runIncidentsList(nil, nil)
		if err != nil {
			t.Errorf("runIncidentsList --json returned error: %v", err)
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

	if len(dataArr) != 103 {
		t.Errorf("--json mode returned %d items, want 103 (all pages merged)", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// Notebooks list — offset-based (count + start)
// Termination: short page (len(data) < pageSize).
// ============================================================================

// makeNotebookItem returns a minimal notebook item map with a unique numeric ID.
func makeNotebookItem(id int) map[string]interface{} {
	return map[string]interface{}{
		"id":   float64(id),
		"type": "notebooks",
		"attributes": map[string]interface{}{
			"name":     fmt.Sprintf("notebook-name-%04d", id),
			"created":  "2024-01-01T00:00:00Z",
			"modified": "2024-01-01T00:00:00Z",
			"author": map[string]interface{}{
				"name":   "Test Author",
				"handle": "test@example.com",
			},
		},
	}
}

// TestPaginationNotebooksList_OffsetBased_TwoPages verifies that a second HTTP
// call is made when page 1 is full and page 2 is short.
func TestPaginationNotebooksList_OffsetBased_TwoPages(t *testing.T) {
	var callCount int32

	makeNotebookPage := func(startID, n int) []interface{} {
		page := make([]interface{}, n)
		for i := 0; i < n; i++ {
			page[i] = makeNotebookItem(startID + i)
		}
		return page
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			// Full page (100 items = maxNotebooksPageSize) → pagination continues.
			writeJSON(w, map[string]interface{}{
				"data": makeNotebookPage(1, 100),
			})
		default:
			// Short page (5 items < 100) → signals last page.
			writeJSON(w, map[string]interface{}{
				"data": makeNotebookPage(101, 5),
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 200
	notebooksListAll = false

	out := captureStdout(t, func() {
		err := runNotebooksList(nil, nil)
		if err != nil {
			t.Errorf("runNotebooksList returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 2 {
		t.Errorf("expected at least 2 HTTP calls (pagination), got %d", callCount)
	}

	// Spot-check notebook names from both pages.
	for _, id := range []int{1, 50, 100, 101, 105} {
		want := fmt.Sprintf("notebook-name-%04d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s; output (truncated):\n%.500s", want, out)
		}
	}
}

// TestPaginationNotebooksList_AllFlag_ThreePages verifies that --all exhausts
// all pages regardless of --limit.
func TestPaginationNotebooksList_AllFlag_ThreePages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeNotebookItem(i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		case 2:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeNotebookItem(100 + i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeNotebookItem(201), makeNotebookItem(202)},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 1 // --all must override this
	flagQuiet = true
	notebooksListAll = true

	out := captureStdout(t, func() {
		err := runNotebooksList(nil, nil)
		if err != nil {
			t.Errorf("runNotebooksList --all returned error: %v", err)
		}
	})

	if atomic.LoadInt32(&callCount) < 3 {
		t.Errorf("expected at least 3 HTTP calls (--all), got %d", callCount)
	}

	for _, id := range []int{1, 100, 101, 200, 201, 202} {
		want := fmt.Sprintf("notebook-name-%04d", id)
		if !strings.Contains(out, want) {
			t.Errorf("output missing %s with --all; output (truncated):\n%.500s", want, out)
		}
	}

	notebooksListAll = false
}

// TestPaginationNotebooksList_JSONMode_MergesAllPages verifies that --json mode
// returns a merged "data" array from all pages.
func TestPaginationNotebooksList_JSONMode_MergesAllPages(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		switch n {
		case 1:
			items := make([]interface{}, 100)
			for i := 0; i < 100; i++ {
				items[i] = makeNotebookItem(i + 1)
			}
			writeJSON(w, map[string]interface{}{"data": items})
		default:
			writeJSON(w, map[string]interface{}{
				"data": []interface{}{makeNotebookItem(101), makeNotebookItem(102)},
			})
		}
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 200
	flagJSON = true
	flagQuiet = true
	notebooksListAll = false

	out := captureStdout(t, func() {
		err := runNotebooksList(nil, nil)
		if err != nil {
			t.Errorf("runNotebooksList --json returned error: %v", err)
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

	if len(dataArr) != 102 {
		t.Errorf("--json mode returned %d items, want 102 (all pages merged)", len(dataArr))
	}

	flagJSON = false
}

// ============================================================================
// Events list — single-page API (GET /api/v1/events)
// The v1 events API has no cursor or offset: all results come in one response
// up to count (max 1000). Tests verify:
//   1. The count query param equals min(--limit, 1000) (not client-side slice).
//   2. --all sends count=1000 to the API.
//   3. --json returns the raw JSON response unchanged.
// ============================================================================

// makeEventItem returns a minimal event item map.
func makeEventItem(id int) map[string]interface{} {
	return map[string]interface{}{
		"id":               float64(id),
		"title":            fmt.Sprintf("event-title-%04d", id),
		"text":             fmt.Sprintf("event-text-%04d", id),
		"source_type_name": "test-source",
		"priority":         "normal",
		"date_happened":    float64(1704067200), // 2024-01-01 00:00 UTC
	}
}

// TestPaginationEventsList_CountSentToAPI verifies that the count query param
// sent to the API equals flagLimit (not a client-side truncation).
func TestPaginationEventsList_CountSentToAPI(t *testing.T) {
	var receivedCount string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount = r.URL.Query().Get("count")
		// Return exactly flagLimit items so we can confirm no extra requests.
		items := make([]interface{}, 50)
		for i := 0; i < 50; i++ {
			items[i] = makeEventItem(i + 1)
		}
		writeJSON(w, map[string]interface{}{"events": items})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 50
	flagQuiet = true
	eventsListAll = false
	eventsListStart = "1d"
	eventsListEnd = "now"
	eventsListPriority = ""
	eventsListSources = ""
	eventsListTags = ""

	captureStdout(t, func() {
		err := runEventsList(nil, nil)
		if err != nil {
			t.Errorf("runEventsList returned error: %v", err)
		}
	})

	if receivedCount != "50" {
		t.Errorf("API received count=%q, want %q; count must be sent to API, not applied client-side", receivedCount, "50")
	}
}

// TestPaginationEventsList_AllFlag_SendsMaxCount verifies that --all causes
// count=1000 (maxEventsPageSize) to be sent to the API.
func TestPaginationEventsList_AllFlag_SendsMaxCount(t *testing.T) {
	var receivedCount string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount = r.URL.Query().Get("count")
		writeJSON(w, map[string]interface{}{
			"events": []interface{}{makeEventItem(1), makeEventItem(2)},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 10 // --all must override this
	flagQuiet = true
	eventsListAll = true
	eventsListStart = "1d"
	eventsListEnd = "now"
	eventsListPriority = ""
	eventsListSources = ""
	eventsListTags = ""

	captureStdout(t, func() {
		err := runEventsList(nil, nil)
		if err != nil {
			t.Errorf("runEventsList --all returned error: %v", err)
		}
	})

	if receivedCount != "1000" {
		t.Errorf("--all: API received count=%q, want \"1000\"", receivedCount)
	}

	eventsListAll = false
}

// TestPaginationEventsList_JSONMode_ReturnsRaw verifies that --json returns
// the raw server JSON (including the "events" key) with correct structure.
func TestPaginationEventsList_JSONMode_ReturnsRaw(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		items := make([]interface{}, 5)
		for i := 0; i < 5; i++ {
			items[i] = makeEventItem(i + 1)
		}
		writeJSON(w, map[string]interface{}{"events": items})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagLimit = 100
	flagJSON = true
	flagQuiet = true
	eventsListAll = false
	eventsListStart = "1d"
	eventsListEnd = "now"
	eventsListPriority = ""
	eventsListSources = ""
	eventsListTags = ""

	out := captureStdout(t, func() {
		err := runEventsList(nil, nil)
		if err != nil {
			t.Errorf("runEventsList --json returned error: %v", err)
		}
	})

	// Events is a single-page API; exactly 1 HTTP call expected.
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected exactly 1 HTTP call (single-page API), got %d", callCount)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}

	eventsArr, ok := result["events"].([]interface{})
	if !ok {
		t.Fatalf("--json output missing 'events' array; output:\n%s", out)
	}

	if len(eventsArr) != 5 {
		t.Errorf("--json mode returned %d events, want 5", len(eventsArr))
	}

	flagJSON = false
}
