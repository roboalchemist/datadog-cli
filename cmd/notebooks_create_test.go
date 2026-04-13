package cmd

import (
	"strings"
	"testing"
	"time"
)

// ---- parseHLine ----

func TestParseHLineBasic(t *testing.T) {
	m, err := parseHLine("175:ok dashed:Pre-incident baseline")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["value"] != "y = 175" {
		t.Errorf("value: got %q, want %q", m["value"], "y = 175")
	}
	if m["display_type"] != "ok dashed" {
		t.Errorf("display_type: got %q, want %q", m["display_type"], "ok dashed")
	}
	if m["label"] != "Pre-incident baseline" {
		t.Errorf("label: got %q, want %q", m["label"], "Pre-incident baseline")
	}
}

func TestParseHLineNoLabel(t *testing.T) {
	m, err := parseHLine("130:error dashed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["value"] != "y = 130" {
		t.Errorf("value: got %q, want %q", m["value"], "y = 130")
	}
	if m["display_type"] != "error dashed" {
		t.Errorf("display_type: got %q, want %q", m["display_type"], "error dashed")
	}
	if _, ok := m["label"]; ok {
		t.Error("expected no label field when label is empty")
	}
}

func TestParseHLineInvalid(t *testing.T) {
	cases := []string{
		"",
		"only-one-segment",
	}
	for _, tc := range cases {
		_, err := parseHLine(tc)
		if err == nil {
			t.Errorf("parseHLine(%q): expected error, got nil", tc)
		}
	}
}

func TestParseHLineDecimalValue(t *testing.T) {
	m, err := parseHLine("3.14:ok solid:pi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["value"] != "y = 3.14" {
		t.Errorf("value: got %q, want %q", m["value"], "y = 3.14")
	}
}

// ---- parseNotebookTime ----

func TestParseNotebookTimeDateHHMM(t *testing.T) {
	// "2026-04-07 07:00" should parse without error and return a reasonable epoch.
	ts, err := parseNotebookTime("2026-04-07 07:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The epoch should be within the year 2026.
	lo := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	hi := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	if ts < lo || ts > hi {
		t.Errorf("epoch %d out of expected range [%d, %d]", ts, lo, hi)
	}
}

func TestParseNotebookTimeDateOnly(t *testing.T) {
	ts, err := parseNotebookTime("2026-04-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts <= 0 {
		t.Errorf("expected positive epoch, got %d", ts)
	}
}

func TestParseNotebookTimeInvalid(t *testing.T) {
	cases := []string{
		"not-a-date",
		"",
		"Apr 7 2026",
	}
	for _, tc := range cases {
		_, err := parseNotebookTime(tc)
		if err == nil {
			t.Errorf("parseNotebookTime(%q): expected error, got nil", tc)
		}
	}
}

// ---- parseVLine ----

func TestParseVLineBasic(t *testing.T) {
	// "2026-04-07 11:31:error bold:Mass eviction"
	// Timestamp: "2026-04-07 11:31", display_type: "error bold", label: "Mass eviction"
	m, err := parseVLine("2026-04-07 11:31:error bold:Mass eviction")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	displayType, _ := m["display_type"].(string)
	if displayType != "error bold" {
		t.Errorf("display_type: got %q, want %q", displayType, "error bold")
	}
	label, _ := m["label"].(string)
	if label != "Mass eviction" {
		t.Errorf("label: got %q, want %q", label, "Mass eviction")
	}

	// Value should be epoch-based band "N < t < N+120"
	value, _ := m["value"].(string)
	if !strings.Contains(value, "< t <") {
		t.Errorf("value %q does not look like an epoch band", value)
	}
}

func TestParseVLineNoLabel(t *testing.T) {
	m, err := parseVLine("2026-04-07 11:31:error bold")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := m["label"]; ok {
		t.Error("expected no label when not specified")
	}
}

func TestParseVLineBandWidth(t *testing.T) {
	// Verify the band is exactly 120 seconds wide.
	m, err := parseVLine("2026-04-07 11:31:ok dashed:event")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	value, _ := m["value"].(string)
	// value is "EPOCH1 < t < EPOCH2"
	parts := strings.Split(value, " < t < ")
	if len(parts) != 2 {
		t.Fatalf("unexpected value format: %q", value)
	}
	var e1, e2 int64
	if _, err := parseEpochFromString(parts[0], &e1); err != nil {
		t.Fatalf("parsing epoch1: %v", err)
	}
	if _, err := parseEpochFromString(parts[1], &e2); err != nil {
		t.Fatalf("parsing epoch2: %v", err)
	}
	if e2-e1 != 120 {
		t.Errorf("band width: got %d seconds, want 120", e2-e1)
	}
}

// parseEpochFromString is a test helper that parses a decimal string into int64.
func parseEpochFromString(s string, out *int64) (int, error) {
	s = strings.TrimSpace(s)
	var v int64
	_, err := intSscanf(s, &v)
	if err != nil {
		return 0, err
	}
	*out = v
	return 0, nil
}

func intSscanf(s string, v *int64) (int, error) {
	var acc int64
	negative := false
	for i, c := range s {
		if i == 0 && c == '-' {
			negative = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, &parseError{s}
		}
		acc = acc*10 + int64(c-'0')
	}
	if negative {
		acc = -acc
	}
	*v = acc
	return 1, nil
}

type parseError struct{ s string }

func (e *parseError) Error() string { return "cannot parse: " + e.s }

func TestParseVLineInvalid(t *testing.T) {
	cases := []string{
		"",
		"no-colon-at-all",
	}
	for _, tc := range cases {
		_, err := parseVLine(tc)
		if err == nil {
			t.Errorf("parseVLine(%q): expected error, got nil", tc)
		}
	}
}

// ---- buildNotebookSlug ----

func TestBuildNotebookSlug(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Apr 7 Incident", "apr-7-incident"},
		{"My Notebook", "my-notebook"},
		{"Hello World!", "hello-world"},
		{"test__double__under", "test-double-under"},
		{"  leading trailing  ", "leading-trailing"},
		{"already-slug", "already-slug"},
		{"ALLCAPS", "allcaps"},
		{"multiple   spaces", "multiple-spaces"},
	}
	for _, tc := range cases {
		got := buildNotebookSlug(tc.input)
		if got != tc.want {
			t.Errorf("buildNotebookSlug(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
