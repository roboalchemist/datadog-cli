package cmd

import (
	"strings"
	"testing"
	"time"
)

// ---- parseTime ----

func TestParseTimeNow(t *testing.T) {
	before := time.Now().UnixMilli()
	got, err := parseTime("now")
	after := time.Now().UnixMilli()

	if err != nil {
		t.Fatalf("parseTime(\"now\") returned error: %v", err)
	}
	if got < before || got > after {
		t.Errorf("parseTime(\"now\") = %d, want in [%d, %d]", got, before, after)
	}
}

func TestParseTimeRelativeMinutes(t *testing.T) {
	before := time.Now().Add(-16 * time.Minute).UnixMilli()
	got, err := parseTime("15m")
	after := time.Now().Add(-14 * time.Minute).UnixMilli()

	if err != nil {
		t.Fatalf("parseTime(\"15m\") returned error: %v", err)
	}
	if got < before || got > after {
		t.Errorf("parseTime(\"15m\") = %d, want in [%d, %d]", got, before, after)
	}
}

func TestParseTimeRelativeHours(t *testing.T) {
	before := time.Now().Add(-61 * time.Minute).UnixMilli()
	got, err := parseTime("1h")
	after := time.Now().Add(-59 * time.Minute).UnixMilli()

	if err != nil {
		t.Fatalf("parseTime(\"1h\") returned error: %v", err)
	}
	if got < before || got > after {
		t.Errorf("parseTime(\"1h\") = %d, want in [%d, %d]", got, before, after)
	}
}

func TestParseTimeRelativeDays(t *testing.T) {
	before := time.Now().Add(-49 * time.Hour).UnixMilli()
	got, err := parseTime("2d")
	after := time.Now().Add(-47 * time.Hour).UnixMilli()

	if err != nil {
		t.Fatalf("parseTime(\"2d\") returned error: %v", err)
	}
	if got < before || got > after {
		t.Errorf("parseTime(\"2d\") = %d, want in [%d, %d]", got, before, after)
	}
}

func TestParseTimeRelativeWeeks(t *testing.T) {
	before := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	got, err := parseTime("1w")
	after := time.Now().Add(-6 * 24 * time.Hour).UnixMilli()

	if err != nil {
		t.Fatalf("parseTime(\"1w\") returned error: %v", err)
	}
	if got < before || got > after {
		t.Errorf("parseTime(\"1w\") = %d, want in [%d, %d]", got, before, after)
	}
}

func TestParseTimeRelativeSeconds(t *testing.T) {
	before := time.Now().Add(-31 * time.Second).UnixMilli()
	got, err := parseTime("30s")
	after := time.Now().Add(-29 * time.Second).UnixMilli()

	if err != nil {
		t.Fatalf("parseTime(\"30s\") returned error: %v", err)
	}
	if got < before || got > after {
		t.Errorf("parseTime(\"30s\") = %d, want in [%d, %d]", got, before, after)
	}
}

func TestParseTimeISO8601UTC(t *testing.T) {
	got, err := parseTime("2024-01-15T10:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).UnixMilli()
	if got != want {
		t.Errorf("parseTime ISO-8601 UTC: got %d, want %d", got, want)
	}
}

func TestParseTimeISO8601WithOffset(t *testing.T) {
	got, err := parseTime("2024-01-15T10:30:00+00:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC).UnixMilli()
	if got != want {
		t.Errorf("parseTime ISO-8601 with offset: got %d, want %d", got, want)
	}
}

func TestParseTimeISO8601DateOnly(t *testing.T) {
	got, err := parseTime("2024-03-20")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC).UnixMilli()
	if got != want {
		t.Errorf("parseTime date-only: got %d, want %d", got, want)
	}
}

func TestParseTimeUnixSeconds(t *testing.T) {
	// 1700000000 is Nov 14 2023, well below the 32503680000 threshold
	got, err := parseTime("1700000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(1700000000) * 1000
	if got != want {
		t.Errorf("parseTime unix seconds: got %d, want %d", got, want)
	}
}

func TestParseTimeUnixMilliseconds(t *testing.T) {
	// 1700000000000 is clearly in milliseconds (>= 32503680000)
	got, err := parseTime("1700000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(1700000000000)
	if got != want {
		t.Errorf("parseTime unix ms: got %d, want %d", got, want)
	}
}

func TestParseTimeInvalidInput(t *testing.T) {
	cases := []string{
		"not-a-time",
		"",
		"abc",
		"2024/01/15",
	}
	for _, tc := range cases {
		_, err := parseTime(tc)
		if err == nil {
			t.Errorf("parseTime(%q): expected error, got nil", tc)
		}
	}
}

func TestParseTimeInvalidInputContainsInput(t *testing.T) {
	_, err := parseTime("badvalue")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
	if !strings.Contains(err.Error(), "badvalue") {
		t.Errorf("expected error message to contain input %q, got: %v", "badvalue", err)
	}
}

func TestParseTimeCaseInsensitive(t *testing.T) {
	got1, err1 := parseTime("NOW")
	got2, err2 := parseTime("now")
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	// Both should be within a millisecond of each other
	diff := got1 - got2
	if diff < -5 || diff > 5 {
		t.Errorf("NOW and now differ by %d ms (expected <5)", diff)
	}
}

// ---- formatID ----

func TestFormatIDFloat64(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{1234567890, "1234567890"},
		{0, "0"},
		{42, "42"},
		{9999999999, "9999999999"},
	}
	for _, tc := range cases {
		got := formatID(tc.input)
		if got != tc.want {
			t.Errorf("formatID(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatIDFloat64NoScientificNotation(t *testing.T) {
	// Large float64 IDs that would normally render as scientific notation
	got := formatID(float64(1234567890123456))
	if strings.Contains(got, "e") || strings.Contains(got, "E") {
		t.Errorf("formatID returned scientific notation: %s", got)
	}
}

func TestFormatIDString(t *testing.T) {
	got := formatID("abc-123")
	if got != "abc-123" {
		t.Errorf("formatID(string) = %q, want %q", got, "abc-123")
	}
}

func TestFormatIDStringEmpty(t *testing.T) {
	got := formatID("")
	if got != "" {
		t.Errorf("formatID(\"\") = %q, want empty string", got)
	}
}

func TestFormatIDNil(t *testing.T) {
	got := formatID(nil)
	// nil prints as "<nil>" via fmt.Sprintf("%v", nil)
	if got == "" {
		t.Error("formatID(nil) returned empty string, expected non-empty")
	}
}

func TestFormatIDOtherType(t *testing.T) {
	got := formatID(42) // int, not float64
	if got != "42" {
		t.Errorf("formatID(int 42) = %q, want %q", got, "42")
	}
}
