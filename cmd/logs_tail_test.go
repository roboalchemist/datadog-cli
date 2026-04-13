package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestLogsTailFlagValidation verifies that invalid flag values are rejected early
// without making any HTTP calls.
func TestLogsTailFlagValidation(t *testing.T) {
	cases := []struct {
		name     string
		interval string
		from     string
		wantErr  string
	}{
		{
			name:     "invalid interval unit",
			interval: "3x",
			from:     "1m",
			wantErr:  "--interval",
		},
		{
			name:     "interval too short",
			interval: "100ms",
			from:     "1m",
			wantErr:  "minimum",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Save and restore flag vars.
			origInterval := logsTailInterval
			origFrom := logsTailFrom
			origQuery := logsTailQuery
			t.Cleanup(func() {
				logsTailInterval = origInterval
				logsTailFrom = origFrom
				logsTailQuery = origQuery
			})

			logsTailInterval = tc.interval
			logsTailFrom = tc.from
			logsTailQuery = "service:test"

			err := runLogsTail(nil, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestLogsTailSinglePoll exercises a single polling cycle against a mock server.
// It sets up a server that returns two log entries, then sends SIGINT after a
// short delay so the tail loop exits cleanly after printing those entries.
func TestLogsTailSinglePoll(t *testing.T) {
	// Build a mock server that returns two log items.
	item1 := makeLogItem("tail-1")
	item2 := makeLogItem("tail-2")
	// Set distinct timestamps so ordering is deterministic.
	item1["attributes"].(map[string]interface{})["timestamp"] = "2024-06-01T10:00:00Z"
	item1["attributes"].(map[string]interface{})["status"] = "info"
	item1["attributes"].(map[string]interface{})["message"] = "hello from tail-1"
	item2["attributes"].(map[string]interface{})["timestamp"] = "2024-06-01T10:00:01Z"
	item2["attributes"].(map[string]interface{})["status"] = "error"
	item2["attributes"].(map[string]interface{})["message"] = "hello from tail-2"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"data": []interface{}{item1, item2},
			"meta": map[string]interface{}{
				"page": map[string]interface{}{},
			},
		})
	}))
	defer srv.Close()

	setMockServer(t, srv)

	// Save and restore flag vars.
	origInterval := logsTailInterval
	origFrom := logsTailFrom
	origQuery := logsTailQuery
	origQuiet := flagQuiet
	origNoColor := flagNoColor
	origLimit := flagLimit
	t.Cleanup(func() {
		logsTailInterval = origInterval
		logsTailFrom = origFrom
		logsTailQuery = origQuery
		flagQuiet = origQuiet
		flagNoColor = origNoColor
		flagLimit = origLimit
	})

	logsTailInterval = "500ms"
	logsTailFrom = "1m"
	logsTailQuery = "service:test"
	flagQuiet = true  // suppress "Tailing logs…" banner on stderr
	flagNoColor = true
	flagLimit = 100

	// Send SIGINT after a short delay to terminate the tail loop.
	go func() {
		time.Sleep(600 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	out := captureStdout(t, func() {
		err := runLogsTail(nil, nil)
		if err != nil {
			t.Errorf("runLogsTail returned error: %v", err)
		}
	})

	// Both log messages should appear in the output.
	if !strings.Contains(out, "hello from tail-1") {
		t.Errorf("output missing 'hello from tail-1'; got:\n%s", out)
	}
	if !strings.Contains(out, "hello from tail-2") {
		t.Errorf("output missing 'hello from tail-2'; got:\n%s", out)
	}
}

// TestColorForStatus verifies that log status strings are colored correctly.
func TestColorForStatus(t *testing.T) {
	cases := []struct {
		status  string
		noColor bool
		// When noColor=false we just check the string contains the status text,
		// since ANSI codes are OS-specific.  When noColor=true we check exact match.
		wantContains string
		wantExact    string
	}{
		{status: "info", noColor: true, wantExact: "info"},
		{status: "error", noColor: true, wantExact: "error"},
		{status: "warn", noColor: true, wantExact: "warn"},
		{status: "debug", noColor: true, wantExact: "debug"},
		{status: "critical", noColor: true, wantExact: "critical"},
		{status: "info", noColor: false, wantContains: "info"},
		{status: "error", noColor: false, wantContains: "error"},
	}

	for _, tc := range cases {
		t.Run(tc.status+"_nocolor_"+boolStr(tc.noColor), func(t *testing.T) {
			got := colorForStatus(tc.status, tc.noColor)
			if tc.wantExact != "" && got != tc.wantExact {
				t.Errorf("colorForStatus(%q, %v) = %q, want %q", tc.status, tc.noColor, got, tc.wantExact)
			}
			if tc.wantContains != "" && !strings.Contains(got, tc.wantContains) {
				t.Errorf("colorForStatus(%q, %v) = %q, does not contain %q", tc.status, tc.noColor, got, tc.wantContains)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
