//go:build integration

package main_test

// Integration tests for notebooks write commands (create, update, delete).
//
// These tests use method-aware httptest mock servers so that the same path
// (/api/v1/notebooks and /api/v1/notebooks/{id}) can handle both read-only
// requests (GET) and write requests (POST, PUT, DELETE) independently.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockCreateResponse is the POST /api/v1/notebooks response body.
const mockNotebookCreateResponse = `{
  "data": {
    "id": 12345678,
    "type": "notebooks",
    "attributes": {
      "name": "Test Notebook",
      "status": "published",
      "cells": [],
      "created": "2026-04-13T00:00:00Z",
      "modified": "2026-04-13T00:00:00Z"
    }
  }
}`

// mockUpdateResponse is the PUT /api/v1/notebooks/{id} response body.
const mockNotebookUpdateResponse = `{
  "data": {
    "id": 99999999,
    "type": "notebooks",
    "attributes": {
      "name": "Updated Notebook",
      "status": "published",
      "cells": [],
      "created": "2026-04-13T00:00:00Z",
      "modified": "2026-04-13T01:00:00Z"
    }
  }
}`

// newNotebookWriteServer creates an httptest server that handles notebooks write
// operations alongside the standard read-only endpoints used by other tests.
// It also accepts an optional callback to capture and inspect the request body.
func newNotebookWriteServer(t *testing.T, onPost func(r *http.Request), onPut func(r *http.Request)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}

	// POST /api/v1/notebooks — create
	// NOTE: must be registered before the trailing-slash wildcard below.
	mux.HandleFunc("/api/v1/notebooks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if onPost != nil {
				onPost(r)
			}
			writeJSON(w, http.StatusOK, mockNotebookCreateResponse)
		case http.MethodGet:
			// List endpoint (used by existing notebooks list tests)
			writeJSON(w, http.StatusOK, `{"data":[{"id":11111,"type":"notebooks","attributes":{"name":"Old Notebook","status":"published","created":"2024-01-01T00:00:00Z","modified":"2024-06-01T00:00:00Z"}}]}`)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// PUT and DELETE /api/v1/notebooks/{id} — update / delete
	mux.HandleFunc("/api/v1/notebooks/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Individual notebook GET (used by existing notebooks get tests)
			writeJSON(w, http.StatusOK, `{"data":{"id":99999999,"type":"notebooks","attributes":{"name":"Updated Notebook","status":"published","cells":[],"created":"2024-01-01T00:00:00Z","modified":"2024-06-01T00:00:00Z"}}}`)
		case http.MethodPut:
			if onPut != nil {
				onPut(r)
			}
			writeJSON(w, http.StatusOK, mockNotebookUpdateResponse)
		case http.MethodDelete:
			// 204 No Content — Datadog notebooks delete returns empty body
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 403 server — for auth-error tests
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newNotebook403Server creates a server that always returns 403 for POST /api/v1/notebooks.
func newNotebook403Server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notebooks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"errors":["Forbidden"]}`)
	})
	mux.HandleFunc("/api/v1/notebooks/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"errors":["Forbidden"]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newNotebook404Server creates a server that always returns 404 for
// PUT /api/v1/notebooks/{id} and DELETE /api/v1/notebooks/{id}.
func newNotebook404Server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notebooks/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"errors":["Notebook not found"]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---- TestIntegration_NotebooksCreate ----

// TestIntegration_NotebooksCreate verifies the happy path: POST returns 200 with
// notebook ID 12345678 and the CLI prints a URL containing that ID.
func TestIntegration_NotebooksCreate(t *testing.T) {
	srv := newNotebookWriteServer(t, nil, nil)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "create",
		"--name", "Test Notebook",
	)
	assertSuccess(t, "notebooks create", stdout, stderr, err)

	if !strings.Contains(stdout, "12345678") {
		t.Errorf("expected stdout to contain notebook ID 12345678, got: %q", stdout)
	}
	if !strings.Contains(stdout, "https://app.datadoghq.com/notebook/") {
		t.Errorf("expected stdout to contain Datadog notebook URL, got: %q", stdout)
	}
}

// TestIntegration_NotebooksCreate_JSON verifies that --json outputs valid JSON
// containing data.id == 12345678.
func TestIntegration_NotebooksCreate_JSON(t *testing.T) {
	srv := newNotebookWriteServer(t, nil, nil)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "create",
		"--name", "Test Notebook",
		"--json",
	)
	assertSuccess(t, "notebooks create --json", stdout, stderr, err)
	assertValidJSON(t, "notebooks create --json stdout", stdout)

	// Parse and validate data.id
	var result map[string]interface{}
	if parseErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); parseErr != nil {
		t.Fatalf("json.Unmarshal failed: %v", parseErr)
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data key in JSON response, got: %v", result)
	}
	id, ok := data["id"]
	if !ok {
		t.Fatalf("expected id key in data, got: %v", data)
	}
	// JSON numbers decode as float64
	idFloat, ok := id.(float64)
	if !ok {
		t.Fatalf("expected id to be a number, got: %T %v", id, id)
	}
	if int(idFloat) != 12345678 {
		t.Errorf("expected data.id == 12345678, got %v", idFloat)
	}
}

// TestIntegration_NotebooksCreate_WithMarkers verifies that --hline, --vline, and
// --note flags are accepted and the command succeeds (the mock echoes the same
// response regardless of request body, but we verify exit 0 + URL output).
func TestIntegration_NotebooksCreate_WithMarkers(t *testing.T) {
	srv := newNotebookWriteServer(t, nil, nil)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "create",
		"--name", "Marker Notebook",
		"--metric", "avg:system.cpu.user{*}",
		"--live-span", "1h",
		"--hline", "175:ok dashed:Baseline",
		"--vline", "2026-04-07 11:31:error bold:Event",
		"--note", "## Notes",
	)
	assertSuccess(t, "notebooks create with markers", stdout, stderr, err)

	if !strings.Contains(stdout, "12345678") {
		t.Errorf("expected stdout to contain notebook ID 12345678, got: %q", stdout)
	}
}

// TestIntegration_NotebooksCreate_AuthError verifies that a 403 response from the
// server results in a non-zero exit code and an error message on stderr.
func TestIntegration_NotebooksCreate_AuthError(t *testing.T) {
	srv := newNotebook403Server(t)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "create",
		"--name", "Should Fail",
	)
	// The command should fail with a non-zero exit code
	if err == nil {
		t.Errorf("expected command to fail on 403, but it succeeded\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	// Error message should be on stderr (the CLI prints errors there)
	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "auth") &&
		!strings.Contains(strings.ToLower(combined), "forbidden") &&
		!strings.Contains(strings.ToLower(combined), "permission") &&
		!strings.Contains(strings.ToLower(combined), "denied") {
		t.Errorf("expected auth/forbidden error in output, got stdout=%q stderr=%q", stdout, stderr)
	}
}

// ---- TestIntegration_NotebooksUpdate ----

// TestIntegration_NotebooksUpdate verifies the happy path: PUT returns 200 with
// notebook ID 99999999 and the CLI prints a URL containing that ID.
func TestIntegration_NotebooksUpdate(t *testing.T) {
	srv := newNotebookWriteServer(t, nil, nil)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "update", "99999999",
		"--name", "Updated Notebook",
	)
	assertSuccess(t, "notebooks update", stdout, stderr, err)

	if !strings.Contains(stdout, "99999999") {
		t.Errorf("expected stdout to contain notebook ID 99999999, got: %q", stdout)
	}
	if !strings.Contains(stdout, "https://app.datadoghq.com/notebook/") {
		t.Errorf("expected stdout to contain Datadog notebook URL, got: %q", stdout)
	}
}

// TestIntegration_NotebooksUpdate_JSON verifies --json outputs valid JSON for the
// update command.
func TestIntegration_NotebooksUpdate_JSON(t *testing.T) {
	srv := newNotebookWriteServer(t, nil, nil)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "update", "99999999",
		"--name", "Updated Notebook",
		"--json",
	)
	assertSuccess(t, "notebooks update --json", stdout, stderr, err)
	assertValidJSON(t, "notebooks update --json stdout", stdout)
}

// TestIntegration_NotebooksUpdate_NotFound verifies that a 404 response results
// in a non-zero exit code with a clear error message.
func TestIntegration_NotebooksUpdate_NotFound(t *testing.T) {
	srv := newNotebook404Server(t)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "update", "00000001",
		"--name", "Ghost Notebook",
	)
	if err == nil {
		t.Errorf("expected command to fail on 404, but it succeeded\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "not found") &&
		!strings.Contains(strings.ToLower(combined), "404") {
		t.Errorf("expected not-found error in output, got stdout=%q stderr=%q", stdout, stderr)
	}
}

// ---- TestIntegration_NotebooksDelete ----

// TestIntegration_NotebooksDelete_NoConfirm verifies that running delete without
// --confirm prints a helpful message and exits 0 (no API call made).
func TestIntegration_NotebooksDelete_NoConfirm(t *testing.T) {
	// Track whether DELETE was called — it should NOT be.
	deleteCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/notebooks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	stdout, stderr, err := runCLI(t, srv.URL, "notebooks", "delete", "123456")
	// Must exit 0
	assertSuccess(t, "notebooks delete no-confirm", stdout, stderr, err)

	// Should print a helpful message telling the user to use --confirm
	if !strings.Contains(stdout, "--confirm") && !strings.Contains(stderr, "--confirm") {
		t.Errorf("expected --confirm hint in output, got stdout=%q stderr=%q", stdout, stderr)
	}
	// Must NOT have called the API
	if deleteCalled {
		t.Error("DELETE API was called even without --confirm flag")
	}
}

// TestIntegration_NotebooksDelete_Confirm verifies the happy path: DELETE returns
// 204 and the CLI prints a success message containing the notebook ID.
func TestIntegration_NotebooksDelete_Confirm(t *testing.T) {
	srv := newNotebookWriteServer(t, nil, nil)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "delete", "123456",
		"--confirm",
	)
	assertSuccess(t, "notebooks delete --confirm", stdout, stderr, err)

	if !strings.Contains(stdout, "123456") {
		t.Errorf("expected stdout to contain notebook ID 123456, got: %q", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "deleted") {
		t.Errorf("expected 'deleted' in stdout, got: %q", stdout)
	}
}

// TestIntegration_NotebooksDelete_NotFound verifies that a 404 response when
// deleting results in a non-zero exit and clear error message.
func TestIntegration_NotebooksDelete_NotFound(t *testing.T) {
	srv := newNotebook404Server(t)

	stdout, stderr, err := runCLI(t, srv.URL,
		"notebooks", "delete", "00000001",
		"--confirm",
	)
	if err == nil {
		t.Errorf("expected command to fail on 404, but it succeeded\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "not found") &&
		!strings.Contains(strings.ToLower(combined), "404") {
		t.Errorf("expected not-found error in output, got stdout=%q stderr=%q", stdout, stderr)
	}
}
