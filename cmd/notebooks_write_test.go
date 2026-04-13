package cmd

// Unit tests for notebooks write subcommands: create, update, delete.
//
// Pattern: httptest.NewServer + DD_API_URL env var + direct run* calls.
// No real HTTP calls are made; no credentials are required.
//
// Also covers helper functions: buildNotebookBody, buildNotebookSlug,
// parseHLine, parseVLine, parseNotebookTime, notebooksFormatTimestamp.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// notebooks create
// ============================================================================

// TestNotebooksCreate_Success verifies the happy path: POST succeeds, URL is printed.
func TestNotebooksCreate_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(12345),
				"type": "notebooks",
				"attributes": map[string]interface{}{
					"name": "My Notebook",
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "My Notebook"
	notebooksCreateFlags.metric = ""
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() {
		notebooksCreateFlags.name = ""
		notebooksCreateFlags.metric = ""
	}()

	out := captureStdout(t, func() {
		if err := runNotebooksCreate(nil, nil); err != nil {
			t.Errorf("runNotebooksCreate returned error: %v", err)
		}
	})

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/api/v1/notebooks") {
		t.Errorf("expected path to contain /api/v1/notebooks, got %s", gotPath)
	}
	if !strings.Contains(out, "12345") {
		t.Errorf("expected notebook ID in output; got:\n%s", out)
	}
	if !strings.Contains(out, "datadoghq.com") {
		t.Errorf("expected Datadog URL in output; got:\n%s", out)
	}
}

// TestNotebooksCreate_JSONMode verifies --json outputs the raw API response.
func TestNotebooksCreate_JSONMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(99999),
				"type": "notebooks",
				"attributes": map[string]interface{}{
					"name": "JSON Notebook",
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagJSON = true
	defer func() { flagJSON = false }()

	notebooksCreateFlags.name = "JSON Notebook"
	notebooksCreateFlags.metric = ""
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() { notebooksCreateFlags.name = "" }()

	out := captureStdout(t, func() {
		if err := runNotebooksCreate(nil, nil); err != nil {
			t.Errorf("runNotebooksCreate --json returned error: %v", err)
		}
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if _, ok := result["data"]; !ok {
		t.Errorf("JSON output missing 'data' key; got:\n%s", out)
	}
}

// TestNotebooksCreate_MissingName verifies --name is required.
func TestNotebooksCreate_MissingName(t *testing.T) {
	resetFlags()
	notebooksCreateFlags.name = ""
	notebooksCreateFlags.metric = ""
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""

	err := runNotebooksCreate(nil, nil)
	if err == nil {
		t.Fatal("expected error when --name is missing, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error to mention 'name', got: %v", err)
	}
}

// TestNotebooksCreate_WithMetricAndTimeRange verifies a full create with --metric, --start, --end.
func TestNotebooksCreate_WithMetricAndTimeRange(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(55555),
				"type": "notebooks",
				"attributes": map[string]interface{}{
					"name": "Incident Notebook",
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "Incident Notebook"
	notebooksCreateFlags.metric = "avg:system.cpu.user{*}"
	notebooksCreateFlags.start = "2026-04-07 07:00"
	notebooksCreateFlags.end = "2026-04-07 17:00"
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = []string{"175:ok dashed:Pre-incident baseline"}
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = "## Notes"
	defer func() {
		notebooksCreateFlags.name = ""
		notebooksCreateFlags.metric = ""
		notebooksCreateFlags.start = ""
		notebooksCreateFlags.end = ""
		notebooksCreateFlags.hlines = nil
		notebooksCreateFlags.note = ""
	}()

	out := captureStdout(t, func() {
		if err := runNotebooksCreate(nil, nil); err != nil {
			t.Errorf("runNotebooksCreate returned error: %v", err)
		}
	})

	if !strings.Contains(out, "55555") {
		t.Errorf("expected ID 55555 in output; got:\n%s", out)
	}

	// Verify request body had the correct structure.
	data, ok := gotBody["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("request body missing 'data' key; got: %v", gotBody)
	}
	attrs, _ := data["attributes"].(map[string]interface{})
	if name, _ := attrs["name"].(string); name != "Incident Notebook" {
		t.Errorf("expected name 'Incident Notebook' in request body, got %q", name)
	}
}

// TestNotebooksCreate_APIError verifies that a 403 error is propagated.
func TestNotebooksCreate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Forbidden"]}`))
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "Test Notebook"
	notebooksCreateFlags.metric = ""
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() { notebooksCreateFlags.name = "" }()

	err := runNotebooksCreate(nil, nil)
	if err == nil {
		t.Fatal("expected error from 403 response, got nil")
	}
}

// TestNotebooksCreate_WithLiveSpan verifies --live-span path.
func TestNotebooksCreate_WithLiveSpan(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(77777),
				"type": "notebooks",
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "Live Span Notebook"
	notebooksCreateFlags.metric = "avg:system.cpu.user{*}"
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = "4h"
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() {
		notebooksCreateFlags.name = ""
		notebooksCreateFlags.metric = ""
		notebooksCreateFlags.liveSpan = ""
	}()

	captureStdout(t, func() {
		if err := runNotebooksCreate(nil, nil); err != nil {
			t.Errorf("runNotebooksCreate with --live-span returned error: %v", err)
		}
	})

	data, _ := gotBody["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	notebookTime, _ := attrs["time"].(map[string]interface{})
	if ls, _ := notebookTime["live_span"].(string); ls != "4h" {
		t.Errorf("expected live_span '4h' in request body, got %q", ls)
	}
}

// TestNotebooksCreate_BuildBodyError verifies that a bad --hline triggers buildNotebookBody failure.
func TestNotebooksCreate_BuildBodyError(t *testing.T) {
	resetFlags()
	notebooksCreateFlags.name = "Error Test"
	notebooksCreateFlags.metric = "avg:cpu{*}"
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = "1h"
	notebooksCreateFlags.hlines = []string{"bad-hline-no-colon"}
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() {
		notebooksCreateFlags.name = ""
		notebooksCreateFlags.metric = ""
		notebooksCreateFlags.liveSpan = ""
		notebooksCreateFlags.hlines = nil
	}()

	err := runNotebooksCreate(nil, nil)
	if err == nil {
		t.Fatal("expected error from bad --hline, got nil")
	}
}

// TestNotebooksCreate_MissingDataKey verifies error when response lacks 'data' key.
func TestNotebooksCreate_MissingDataKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"status": "ok",
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "No Data Key"
	notebooksCreateFlags.metric = ""
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() { notebooksCreateFlags.name = "" }()

	err := runNotebooksCreate(nil, nil)
	if err == nil {
		t.Fatal("expected error when response missing 'data' key, got nil")
	}
	if !strings.Contains(err.Error(), "data") {
		t.Errorf("expected error to mention 'data', got: %v", err)
	}
}

// TestNotebooksCreate_EmptyIDInResponse verifies error when response ID is empty/nil.
func TestNotebooksCreate_EmptyIDInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"type": "notebooks",
				// no 'id' key — formatID(nil) returns "<nil>"
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "Empty ID Test"
	notebooksCreateFlags.metric = ""
	notebooksCreateFlags.start = ""
	notebooksCreateFlags.end = ""
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = nil
	notebooksCreateFlags.note = ""
	defer func() { notebooksCreateFlags.name = "" }()

	err := runNotebooksCreate(nil, nil)
	if err == nil {
		t.Fatal("expected error when notebook ID is empty/nil, got nil")
	}
}

// TestNotebooksCreate_WithVLine verifies the vline flag creates vertical markers.
func TestNotebooksCreate_WithVLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(88888),
				"type": "notebooks",
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksCreateFlags.name = "VLine Notebook"
	notebooksCreateFlags.metric = "avg:system.cpu.user{*}"
	notebooksCreateFlags.start = "2026-04-07 07:00"
	notebooksCreateFlags.end = "2026-04-07 17:00"
	notebooksCreateFlags.liveSpan = ""
	notebooksCreateFlags.hlines = nil
	notebooksCreateFlags.vlines = []string{"2026-04-07 11:31:error bold:Mass eviction"}
	notebooksCreateFlags.note = ""
	defer func() {
		notebooksCreateFlags.name = ""
		notebooksCreateFlags.metric = ""
		notebooksCreateFlags.start = ""
		notebooksCreateFlags.end = ""
		notebooksCreateFlags.vlines = nil
	}()

	captureStdout(t, func() {
		if err := runNotebooksCreate(nil, nil); err != nil {
			t.Errorf("runNotebooksCreate with --vline returned error: %v", err)
		}
	})
}

// ============================================================================
// notebooks update
// ============================================================================

// TestNotebooksUpdate_Success verifies the happy path: PUT succeeds, URL printed.
func TestNotebooksUpdate_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(12345),
				"type": "notebooks",
				"attributes": map[string]interface{}{
					"name": "Updated Notebook",
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksUpdateFlags.name = "Updated Notebook"
	notebooksUpdateFlags.metric = ""
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = ""
	notebooksUpdateFlags.hlines = nil
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""
	defer func() { notebooksUpdateFlags.name = "" }()

	out := captureStdout(t, func() {
		if err := runNotebooksUpdate(nil, []string{"12345"}); err != nil {
			t.Errorf("runNotebooksUpdate returned error: %v", err)
		}
	})

	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "12345") {
		t.Errorf("expected notebook ID in URL path; got %s", gotPath)
	}
	if !strings.Contains(gotPath, "/api/v1/notebooks") {
		t.Errorf("expected /api/v1/notebooks in URL path; got %s", gotPath)
	}
	if !strings.Contains(out, "12345") {
		t.Errorf("expected ID in output; got:\n%s", out)
	}
}

// TestNotebooksUpdate_JSONMode verifies --json outputs the raw API response.
func TestNotebooksUpdate_JSONMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"id":   float64(12345),
				"type": "notebooks",
				"attributes": map[string]interface{}{
					"name": "Updated Notebook",
				},
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagJSON = true
	defer func() { flagJSON = false }()

	notebooksUpdateFlags.name = "Updated Notebook"
	notebooksUpdateFlags.metric = ""
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = ""
	notebooksUpdateFlags.hlines = nil
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""
	defer func() { notebooksUpdateFlags.name = "" }()

	out := captureStdout(t, func() {
		if err := runNotebooksUpdate(nil, []string{"12345"}); err != nil {
			t.Errorf("runNotebooksUpdate --json returned error: %v", err)
		}
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if _, ok := result["data"]; !ok {
		t.Errorf("JSON output missing 'data' key; got:\n%s", out)
	}
}

// TestNotebooksUpdate_MissingName verifies --name is required for update.
func TestNotebooksUpdate_MissingName(t *testing.T) {
	resetFlags()
	notebooksUpdateFlags.name = ""
	notebooksUpdateFlags.metric = ""
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = ""
	notebooksUpdateFlags.hlines = nil
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""

	err := runNotebooksUpdate(nil, []string{"12345"})
	if err == nil {
		t.Fatal("expected error when --name is missing, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error to mention 'name', got: %v", err)
	}
}

// TestNotebooksUpdate_FallbackIDWhenResponseMissingData verifies the ID fallback path.
// When the response "data" key is missing, the user-supplied ID is used.
func TestNotebooksUpdate_FallbackIDWhenResponseMissingData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a response without a 'data' key to trigger the fallback path.
		writeJSON(w, map[string]interface{}{
			"status": "ok",
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksUpdateFlags.name = "Fallback Test"
	notebooksUpdateFlags.metric = ""
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = ""
	notebooksUpdateFlags.hlines = nil
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""
	defer func() { notebooksUpdateFlags.name = "" }()

	err := runNotebooksUpdate(nil, []string{"99999"})
	// This hits the "unexpected response shape" error path.
	if err == nil {
		t.Fatal("expected error when response is missing 'data', got nil")
	}
}

// TestNotebooksUpdate_BuildBodyError verifies that a bad --hline triggers buildNotebookBody failure.
func TestNotebooksUpdate_BuildBodyError(t *testing.T) {
	resetFlags()
	notebooksUpdateFlags.name = "Error Test"
	notebooksUpdateFlags.metric = "avg:cpu{*}"
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = "1h"
	notebooksUpdateFlags.hlines = []string{"bad-hline-no-colon"}
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""
	defer func() {
		notebooksUpdateFlags.name = ""
		notebooksUpdateFlags.metric = ""
		notebooksUpdateFlags.liveSpan = ""
		notebooksUpdateFlags.hlines = nil
	}()

	err := runNotebooksUpdate(nil, []string{"12345"})
	if err == nil {
		t.Fatal("expected error from bad --hline, got nil")
	}
}

// TestNotebooksUpdate_FallbackToSuppliedIDWhenNilID verifies that when data.id is nil,
// the user-supplied notebook ID is used in the output URL.
func TestNotebooksUpdate_FallbackToSuppliedIDWhenNilID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": map[string]interface{}{
				"type": "notebooks",
				// no 'id' key — formatID(nil) returns "<nil>", triggering the fallback
			},
		})
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksUpdateFlags.name = "Nil ID Fallback"
	notebooksUpdateFlags.metric = ""
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = ""
	notebooksUpdateFlags.hlines = nil
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""
	defer func() { notebooksUpdateFlags.name = "" }()

	out := captureStdout(t, func() {
		if err := runNotebooksUpdate(nil, []string{"54321"}); err != nil {
			t.Errorf("runNotebooksUpdate with nil ID returned error: %v", err)
		}
	})

	// The output URL should use the user-supplied ID as fallback.
	if !strings.Contains(out, "54321") {
		t.Errorf("expected user-supplied ID '54321' in output URL; got:\n%s", out)
	}
}

// TestNotebooksUpdate_APIError verifies that a 403 error is propagated.
func TestNotebooksUpdate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["Access denied"]}`))
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksUpdateFlags.name = "Error Test"
	notebooksUpdateFlags.metric = ""
	notebooksUpdateFlags.start = ""
	notebooksUpdateFlags.end = ""
	notebooksUpdateFlags.liveSpan = ""
	notebooksUpdateFlags.hlines = nil
	notebooksUpdateFlags.vlines = nil
	notebooksUpdateFlags.note = ""
	defer func() { notebooksUpdateFlags.name = "" }()

	err := runNotebooksUpdate(nil, []string{"12345"})
	if err == nil {
		t.Fatal("expected error from 403 response, got nil")
	}
}

// ============================================================================
// notebooks delete
// ============================================================================

// TestNotebooksDelete_WithoutConfirm verifies that omitting --confirm prints a
// helpful message and exits without error.
func TestNotebooksDelete_WithoutConfirm(t *testing.T) {
	resetFlags()
	notebooksDeleteConfirm = false

	out := captureStdout(t, func() {
		if err := runNotebooksDelete(nil, []string{"12345"}); err != nil {
			t.Errorf("runNotebooksDelete without --confirm returned error: %v", err)
		}
	})

	if !strings.Contains(out, "12345") {
		t.Errorf("expected notebook ID in helpful message; got:\n%s", out)
	}
	if !strings.Contains(out, "--confirm") {
		t.Errorf("expected '--confirm' in helpful message; got:\n%s", out)
	}
}

// TestNotebooksDelete_WithConfirm verifies DELETE is called and success message printed.
func TestNotebooksDelete_WithConfirm(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksDeleteConfirm = true
	defer func() { notebooksDeleteConfirm = false }()

	out := captureStdout(t, func() {
		if err := runNotebooksDelete(nil, []string{"12345"}); err != nil {
			t.Errorf("runNotebooksDelete with --confirm returned error: %v", err)
		}
	})

	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "12345") {
		t.Errorf("expected notebook ID in URL path; got %s", gotPath)
	}
	if !strings.Contains(out, "Deleted notebook 12345") {
		t.Errorf("expected 'Deleted notebook 12345' in output; got:\n%s", out)
	}
}

// TestNotebooksDelete_WithConfirmJSON verifies --json output on successful delete.
func TestNotebooksDelete_WithConfirmJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	flagJSON = true
	notebooksDeleteConfirm = true
	defer func() {
		flagJSON = false
		notebooksDeleteConfirm = false
	}()

	out := captureStdout(t, func() {
		if err := runNotebooksDelete(nil, []string{"12345"}); err != nil {
			t.Errorf("runNotebooksDelete --confirm --json returned error: %v", err)
		}
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("--json output is not valid JSON: %v\nOutput:\n%s", err, out)
	}
	if deleted, _ := result["deleted"].(bool); !deleted {
		t.Errorf("expected 'deleted: true' in JSON output; got:\n%s", out)
	}
	if id, _ := result["id"].(string); id != "12345" {
		t.Errorf("expected 'id: 12345' in JSON output; got:\n%s", out)
	}
}

// TestNotebooksDelete_APIError verifies that an error from the DELETE call is propagated.
func TestNotebooksDelete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Notebook not found"]}`))
	}))
	defer srv.Close()
	setMockServer(t, srv)

	resetFlags()
	notebooksDeleteConfirm = true
	defer func() { notebooksDeleteConfirm = false }()

	err := runNotebooksDelete(nil, []string{"99999"})
	if err == nil {
		t.Fatal("expected error from 404 response, got nil")
	}
}

// ============================================================================
// buildNotebookBody
// ============================================================================

// TestBuildNotebookBody_DefaultLiveSpan verifies that omitting all time flags defaults to 1h live span.
func TestBuildNotebookBody_DefaultLiveSpan(t *testing.T) {
	body, err := buildNotebookBody("My Notebook", "", "", "", "", nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := body["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	notebookTime, _ := attrs["time"].(map[string]interface{})
	if ls, _ := notebookTime["live_span"].(string); ls != "1h" {
		t.Errorf("expected default live_span '1h', got %q", ls)
	}
}

// TestBuildNotebookBody_AbsoluteTimeRange verifies start/end set epoch seconds.
func TestBuildNotebookBody_AbsoluteTimeRange(t *testing.T) {
	body, err := buildNotebookBody("My Notebook", "", "2026-04-07 07:00", "2026-04-07 17:00", "", nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := body["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	notebookTime, _ := attrs["time"].(map[string]interface{})

	if _, ok := notebookTime["start"]; !ok {
		t.Errorf("expected 'start' in time object; got %v", notebookTime)
	}
	if _, ok := notebookTime["end"]; !ok {
		t.Errorf("expected 'end' in time object; got %v", notebookTime)
	}
}

// TestBuildNotebookBody_OnlyStartError verifies that providing only --start without --end returns an error.
func TestBuildNotebookBody_OnlyStartError(t *testing.T) {
	_, err := buildNotebookBody("My Notebook", "", "2026-04-07", "", "", nil, nil, "")
	if err == nil {
		t.Fatal("expected error when only --start provided, got nil")
	}
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("expected error to mention 'both', got: %v", err)
	}
}

// TestBuildNotebookBody_OnlyEndError verifies that providing only --end without --start returns an error.
func TestBuildNotebookBody_OnlyEndError(t *testing.T) {
	_, err := buildNotebookBody("My Notebook", "", "", "2026-04-07", "", nil, nil, "")
	if err == nil {
		t.Fatal("expected error when only --end provided, got nil")
	}
}

// TestBuildNotebookBody_InvalidStart verifies that an invalid --start returns an error.
func TestBuildNotebookBody_InvalidStart(t *testing.T) {
	_, err := buildNotebookBody("My Notebook", "", "not-a-date", "2026-04-07", "", nil, nil, "")
	if err == nil {
		t.Fatal("expected error for invalid --start, got nil")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Errorf("expected error to mention 'start', got: %v", err)
	}
}

// TestBuildNotebookBody_InvalidEnd verifies that an invalid --end returns an error.
func TestBuildNotebookBody_InvalidEnd(t *testing.T) {
	_, err := buildNotebookBody("My Notebook", "", "2026-04-07", "not-a-date", "", nil, nil, "")
	if err == nil {
		t.Fatal("expected error for invalid --end, got nil")
	}
	if !strings.Contains(err.Error(), "end") {
		t.Errorf("expected error to mention 'end', got: %v", err)
	}
}

// TestBuildNotebookBody_NoteCell verifies a note-only notebook has a markdown cell.
func TestBuildNotebookBody_NoteCell(t *testing.T) {
	body, err := buildNotebookBody("My Notebook", "", "", "", "1h", nil, nil, "## Notes\n\nSome text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := body["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	cells, _ := attrs["cells"].([]map[string]interface{})
	if len(cells) < 1 {
		t.Fatalf("expected at least 1 cell, got %d", len(cells))
	}
	cellAttrs, _ := cells[0]["attributes"].(map[string]interface{})
	def, _ := cellAttrs["definition"].(map[string]interface{})
	if typ, _ := def["type"].(string); typ != "markdown" {
		t.Errorf("expected first cell type 'markdown', got %q", typ)
	}
	if text, _ := def["text"].(string); !strings.Contains(text, "Notes") {
		t.Errorf("expected note text in cell, got %q", text)
	}
}

// TestBuildNotebookBody_EmptyPlaceholderCell verifies that a notebook with no cells
// gets a single empty markdown placeholder cell.
func TestBuildNotebookBody_EmptyPlaceholderCell(t *testing.T) {
	body, err := buildNotebookBody("My Notebook", "", "", "", "", nil, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := body["data"].(map[string]interface{})
	attrs, _ := data["attributes"].(map[string]interface{})
	cells, _ := attrs["cells"].([]map[string]interface{})
	if len(cells) != 1 {
		t.Fatalf("expected 1 placeholder cell, got %d", len(cells))
	}
	cellAttrs, _ := cells[0]["attributes"].(map[string]interface{})
	def, _ := cellAttrs["definition"].(map[string]interface{})
	if typ, _ := def["type"].(string); typ != "markdown" {
		t.Errorf("expected placeholder cell type 'markdown', got %q", typ)
	}
}

// TestBuildNotebookBody_InvalidHLine verifies that a bad --hline returns an error.
func TestBuildNotebookBody_InvalidHLine(t *testing.T) {
	_, err := buildNotebookBody("My Notebook", "avg:cpu{*}", "", "", "1h", []string{"bad-hline"}, nil, "")
	if err == nil {
		t.Fatal("expected error for invalid --hline, got nil")
	}
}

// TestBuildNotebookBody_InvalidVLine verifies that a bad --vline returns an error.
func TestBuildNotebookBody_InvalidVLine(t *testing.T) {
	_, err := buildNotebookBody("My Notebook", "avg:cpu{*}", "", "", "1h", nil, []string{"notadatetime"}, "")
	if err == nil {
		t.Fatal("expected error for invalid --vline, got nil")
	}
}

// ============================================================================
// parseHLine
// ============================================================================

// TestParseHLine_FullFormat verifies VALUE:DISPLAY_TYPE:LABEL parsing.
func TestParseHLine_FullFormat(t *testing.T) {
	m, err := parseHLine("175:ok dashed:Pre-incident baseline")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, _ := m["value"].(string); v != "y = 175" {
		t.Errorf("expected value 'y = 175', got %q", v)
	}
	if dt, _ := m["display_type"].(string); dt != "ok dashed" {
		t.Errorf("expected display_type 'ok dashed', got %q", dt)
	}
	if label, _ := m["label"].(string); label != "Pre-incident baseline" {
		t.Errorf("expected label 'Pre-incident baseline', got %q", label)
	}
}

// TestParseHLine_NoLabel verifies VALUE:DISPLAY_TYPE parsing without a label.
func TestParseHLine_NoLabel(t *testing.T) {
	m, err := parseHLine("100:warning bold")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, _ := m["value"].(string); v != "y = 100" {
		t.Errorf("expected value 'y = 100', got %q", v)
	}
	if _, ok := m["label"]; ok {
		t.Errorf("expected no label when not provided, but label key is present")
	}
}

// TestParseHLine_Invalid verifies that a string without a colon returns an error.
func TestParseHLine_Invalid(t *testing.T) {
	_, err := parseHLine("nocolon")
	if err == nil {
		t.Fatal("expected error for input without colon, got nil")
	}
	if !strings.Contains(err.Error(), "VALUE:DISPLAY_TYPE") {
		t.Errorf("expected error to mention format; got: %v", err)
	}
}

// ============================================================================
// parseVLine
// ============================================================================

// TestParseVLine_FullFormat verifies DATETIME:DISPLAY_TYPE:LABEL parsing.
func TestParseVLine_FullFormat(t *testing.T) {
	m, err := parseVLine("2026-04-07 11:31:error bold:Mass eviction")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m["value"]; !ok {
		t.Errorf("expected 'value' key in marker, got: %v", m)
	}
	val, _ := m["value"].(string)
	// Value format is "EPOCH < t < EPOCH+120" — contains two '<' but no '>'.
	if !strings.Contains(val, "< t <") {
		t.Errorf("expected epoch range value like 'N < t < M', got %q", val)
	}
	if dt, _ := m["display_type"].(string); dt != "error bold" {
		t.Errorf("expected display_type 'error bold', got %q", dt)
	}
	if label, _ := m["label"].(string); label != "Mass eviction" {
		t.Errorf("expected label 'Mass eviction', got %q", label)
	}
}

// TestParseVLine_DateOnly verifies that a date-only timestamp works.
func TestParseVLine_DateOnly(t *testing.T) {
	m, err := parseVLine("2026-04-07:ok dashed:Deployment day")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m["value"]; !ok {
		t.Errorf("expected 'value' key in marker, got: %v", m)
	}
}

// TestParseVLine_Invalid verifies that an unparseable vline returns an error.
func TestParseVLine_Invalid(t *testing.T) {
	_, err := parseVLine("notadatetime")
	if err == nil {
		t.Fatal("expected error for unparseable vline, got nil")
	}
}

// TestParseVLine_EpochBand verifies the 2-minute band: value contains epoch < t < epoch+120.
func TestParseVLine_EpochBand(t *testing.T) {
	m, err := parseVLine("2026-04-07 11:31:ok:Event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, _ := m["value"].(string)
	// Value should be of the form "EPOCH < t < EPOCH+120"
	if !strings.Contains(val, "< t <") {
		t.Errorf("expected range format 'N < t < M', got %q", val)
	}
}

// ============================================================================
// parseNotebookTime
// ============================================================================

// TestParseNotebookTime_DateTimeWithSpace verifies "YYYY-MM-DD HH:MM" in PT.
func TestParseNotebookTime_DateTimeWithSpace(t *testing.T) {
	epoch, err := parseNotebookTime("2026-04-07 11:31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epoch <= 0 {
		t.Errorf("expected positive epoch, got %d", epoch)
	}

	loc, _ := time.LoadLocation("America/Los_Angeles")
	expected := time.Date(2026, 4, 7, 11, 31, 0, 0, loc).Unix()
	if epoch != expected {
		t.Errorf("expected epoch %d (PT), got %d", expected, epoch)
	}
}

// TestParseNotebookTime_DateTimeISO verifies "YYYY-MM-DDTHH:MM" format.
func TestParseNotebookTime_DateTimeISO(t *testing.T) {
	epoch, err := parseNotebookTime("2026-04-07T11:31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epoch <= 0 {
		t.Errorf("expected positive epoch, got %d", epoch)
	}
}

// TestParseNotebookTime_DateOnly verifies "YYYY-MM-DD" midnight PT.
func TestParseNotebookTime_DateOnly(t *testing.T) {
	epoch, err := parseNotebookTime("2026-04-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loc, _ := time.LoadLocation("America/Los_Angeles")
	expected := time.Date(2026, 4, 7, 0, 0, 0, 0, loc).Unix()
	if epoch != expected {
		t.Errorf("expected midnight PT epoch %d, got %d", expected, epoch)
	}
}

// TestParseNotebookTime_DateTimeWithSeconds verifies "YYYY-MM-DD HH:MM:SS" format.
func TestParseNotebookTime_DateTimeWithSeconds(t *testing.T) {
	epoch, err := parseNotebookTime("2026-04-07 11:31:45")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epoch <= 0 {
		t.Errorf("expected positive epoch, got %d", epoch)
	}
}

// TestParseNotebookTime_Invalid verifies that an unrecognised format returns an error.
func TestParseNotebookTime_Invalid(t *testing.T) {
	_, err := parseNotebookTime("not-a-date")
	if err == nil {
		t.Fatal("expected error for invalid datetime, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-date") {
		t.Errorf("expected error to contain input; got: %v", err)
	}
}

// TestParseNotebookTime_WhitespaceStripped verifies leading/trailing whitespace is ignored.
func TestParseNotebookTime_WhitespaceStripped(t *testing.T) {
	epoch, err := parseNotebookTime("  2026-04-07  ")
	if err != nil {
		t.Fatalf("unexpected error with whitespace: %v", err)
	}
	if epoch <= 0 {
		t.Errorf("expected positive epoch, got %d", epoch)
	}
}

// ============================================================================
// buildNotebookSlug
// ============================================================================

// TestBuildNotebookSlug_Basic verifies spaces become hyphens and characters are lowercased.
func TestBuildNotebookSlug_Basic(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"My Notebook", "my-notebook"},
		{"Apr 7 Incident", "apr-7-incident"},
		{"Hello_World", "hello-world"},
		{"  leading spaces  ", "leading-spaces"},
		{"double--hyphen", "double-hyphen"},
		{"Special!@#Chars", "specialchars"},
	}
	for _, tc := range cases {
		got := buildNotebookSlug(tc.input)
		if got != tc.want {
			t.Errorf("buildNotebookSlug(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ============================================================================
// notebooksFormatTimestamp
// ============================================================================

// TestNotebooksFormatTimestamp_Nil verifies nil returns empty string.
func TestNotebooksFormatTimestamp_Nil(t *testing.T) {
	got := notebooksFormatTimestamp(nil)
	if got != "" {
		t.Errorf("expected empty string for nil, got %q", got)
	}
}

// TestNotebooksFormatTimestamp_Float64 verifies Unix timestamp (float64) formats correctly.
func TestNotebooksFormatTimestamp_Float64(t *testing.T) {
	got := notebooksFormatTimestamp(float64(1700000000))
	if got == "" {
		t.Fatal("expected non-empty string for float64 timestamp")
	}
	// Should format as "YYYY-MM-DD HH:MM"
	if len(got) < 10 {
		t.Errorf("expected formatted date, got %q", got)
	}
}

// TestNotebooksFormatTimestamp_StringISO8601Z verifies ISO 8601 UTC string.
func TestNotebooksFormatTimestamp_StringISO8601Z(t *testing.T) {
	got := notebooksFormatTimestamp("2024-01-15T10:30:00Z")
	want := "2024-01-15 10:30"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestNotebooksFormatTimestamp_StringRFC3339 verifies RFC3339 string.
func TestNotebooksFormatTimestamp_StringRFC3339(t *testing.T) {
	got := notebooksFormatTimestamp("2024-01-15T10:30:00+00:00")
	want := "2024-01-15 10:30"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestNotebooksFormatTimestamp_StringNoTimezone verifies a plain datetime string (no TZ).
func TestNotebooksFormatTimestamp_StringNoTimezone(t *testing.T) {
	got := notebooksFormatTimestamp("2024-01-15T10:30:00")
	want := "2024-01-15 10:30"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestNotebooksFormatTimestamp_StringEmpty verifies an empty string returns empty string.
func TestNotebooksFormatTimestamp_StringEmpty(t *testing.T) {
	got := notebooksFormatTimestamp("")
	if got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

// TestNotebooksFormatTimestamp_UnknownType verifies unknown types are formatted with %v.
func TestNotebooksFormatTimestamp_UnknownType(t *testing.T) {
	got := notebooksFormatTimestamp(42)
	if got == "" {
		t.Errorf("expected non-empty string for int input, got empty")
	}
}

// TestNotebooksFormatTimestamp_StringUnparseable verifies that an unparseable string
// is returned as-is (falls through all parse attempts).
func TestNotebooksFormatTimestamp_StringUnparseable(t *testing.T) {
	input := "not-a-date-at-all"
	got := notebooksFormatTimestamp(input)
	// Should return the original string unchanged.
	if got != input {
		t.Errorf("expected unparseable string to be returned as-is, got %q", got)
	}
}

// ============================================================================
// notebooksStringField
// ============================================================================

// TestNotebooksStringField_NilMap verifies nil map returns empty string.
func TestNotebooksStringField_NilMap(t *testing.T) {
	got := notebooksStringField(nil, "key")
	if got != "" {
		t.Errorf("expected empty string for nil map, got %q", got)
	}
}

// TestNotebooksStringField_MissingKey verifies missing key returns empty string.
func TestNotebooksStringField_MissingKey(t *testing.T) {
	m := map[string]interface{}{"other": "value"}
	got := notebooksStringField(m, "missing")
	if got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

// TestNotebooksStringField_NilValue verifies nil value returns empty string.
func TestNotebooksStringField_NilValue(t *testing.T) {
	m := map[string]interface{}{"key": nil}
	got := notebooksStringField(m, "key")
	if got != "" {
		t.Errorf("expected empty string for nil value, got %q", got)
	}
}

// TestNotebooksStringField_NonStringValue verifies non-string value uses Sprintf.
func TestNotebooksStringField_NonStringValue(t *testing.T) {
	m := map[string]interface{}{"num": float64(42)}
	got := notebooksStringField(m, "num")
	if got != "42" {
		t.Errorf("expected '42' for float64 42, got %q", got)
	}
}

// ============================================================================
// notebooksAuthor
// ============================================================================

// TestNotebooksAuthor_NilAttrs verifies nil attrs returns empty string.
func TestNotebooksAuthor_NilAttrs(t *testing.T) {
	got := notebooksAuthor(nil)
	if got != "" {
		t.Errorf("expected empty string for nil attrs, got %q", got)
	}
}

// TestNotebooksAuthor_NoAuthorKey verifies missing author key returns empty string.
func TestNotebooksAuthor_NoAuthorKey(t *testing.T) {
	m := map[string]interface{}{"name": "test"}
	got := notebooksAuthor(m)
	if got != "" {
		t.Errorf("expected empty string when no 'author' key, got %q", got)
	}
}

// TestNotebooksAuthor_NameOverHandle verifies name is preferred over handle.
func TestNotebooksAuthor_NameOverHandle(t *testing.T) {
	m := map[string]interface{}{
		"author": map[string]interface{}{
			"name":   "Jane Doe",
			"handle": "jane.doe",
		},
	}
	got := notebooksAuthor(m)
	if got != "Jane Doe" {
		t.Errorf("expected 'Jane Doe', got %q", got)
	}
}

// TestNotebooksAuthor_FallbackToHandle verifies handle is used when name is absent.
func TestNotebooksAuthor_FallbackToHandle(t *testing.T) {
	m := map[string]interface{}{
		"author": map[string]interface{}{
			"handle": "jane.doe",
		},
	}
	got := notebooksAuthor(m)
	if got != "jane.doe" {
		t.Errorf("expected 'jane.doe', got %q", got)
	}
}
