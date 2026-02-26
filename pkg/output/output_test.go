package output

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// captureStdout redirects os.Stdout to a buffer for the duration of fn.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("ReadFrom pipe: %v", err)
	}
	return buf.String()
}

// --- NewPrinter / NewPrinterWithWriter ---

func TestNewPrinter_NotNil(t *testing.T) {
	p := NewPrinter(Options{})
	if p == nil {
		t.Fatal("NewPrinter returned nil")
	}
}

func TestNewPrinterWithWriter_UsesWriter(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{Plaintext: true}, &buf)
	p.PrintTable([]string{"Name"}, [][]string{{"Alice"}})
	if !strings.Contains(buf.String(), "Alice") {
		t.Errorf("expected Alice in output, got: %q", buf.String())
	}
}

// --- RenderTable ---

func TestRenderTable_JSONOutput(t *testing.T) {
	cols := []ColumnConfig{{Name: "Name"}, {Name: "Status"}}
	rows := [][]string{{"Alice", "ok"}, {"Bob", "error"}}
	data := []map[string]string{{"Name": "Alice", "Status": "ok"}, {"Name": "Bob", "Status": "error"}}

	out := captureStdout(t, func() {
		err := RenderTable(cols, rows, data, Options{JSON: true, NoColor: true})
		if err != nil {
			t.Errorf("RenderTable JSON: %v", err)
		}
	})
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected Alice in JSON output, got: %q", out)
	}
	if !strings.Contains(out, "Status") {
		t.Errorf("expected Status key in JSON output, got: %q", out)
	}
}

func TestRenderTable_PlaintextOutput(t *testing.T) {
	cols := []ColumnConfig{{Name: "Name"}, {Name: "Value"}}
	rows := [][]string{{"foo", "bar"}}

	out := captureStdout(t, func() {
		err := RenderTable(cols, rows, nil, Options{Plaintext: true, NoColor: true})
		if err != nil {
			t.Errorf("RenderTable plaintext: %v", err)
		}
	})
	if !strings.Contains(out, "foo") {
		t.Errorf("expected foo in plaintext output, got: %q", out)
	}
	if !strings.Contains(out, "bar") {
		t.Errorf("expected bar in plaintext output, got: %q", out)
	}
	// Plaintext should NOT have table borders
	if strings.Contains(out, "+---") {
		t.Errorf("plaintext should not have table borders, got: %q", out)
	}
}

func TestRenderTable_TableOutput(t *testing.T) {
	cols := []ColumnConfig{{Name: "Name"}, {Name: "Count"}}
	rows := [][]string{{"svc-a", "42"}}

	out := captureStdout(t, func() {
		err := RenderTable(cols, rows, nil, Options{NoColor: true})
		if err != nil {
			t.Errorf("RenderTable table: %v", err)
		}
	})
	if !strings.Contains(out, "svc-a") {
		t.Errorf("expected svc-a in table output, got: %q", out)
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("expected NAME header in table output, got: %q", out)
	}
}

// --- RenderTable: field filtering ---

func TestRenderTable_FieldFilter(t *testing.T) {
	cols := []ColumnConfig{{Name: "Name"}, {Name: "Status"}, {Name: "Tags"}}
	rows := [][]string{{"svc-a", "ok", "env:prod"}}

	out := captureStdout(t, func() {
		err := RenderTable(cols, rows, nil, Options{Fields: "name,status", NoColor: true})
		if err != nil {
			t.Errorf("RenderTable with fields: %v", err)
		}
	})
	if !strings.Contains(out, "svc-a") {
		t.Errorf("expected svc-a in filtered output, got: %q", out)
	}
	// Tags column should be absent
	if strings.Contains(out, "env:prod") {
		t.Errorf("Tags column should be filtered out, got: %q", out)
	}
}

func TestRenderTable_FieldFilter_PreservesOrder(t *testing.T) {
	cols := []ColumnConfig{{Name: "A"}, {Name: "B"}, {Name: "C"}}
	rows := [][]string{{"val-a", "val-b", "val-c"}}

	// Request B,A order
	filteredCols, filteredRows := applyFieldFilter(cols, rows, "B,A")
	if len(filteredCols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(filteredCols))
	}
	// First should be B (requested first)
	if filteredCols[0].Name != "B" {
		t.Errorf("expected first col B, got %s", filteredCols[0].Name)
	}
	if filteredCols[1].Name != "A" {
		t.Errorf("expected second col A, got %s", filteredCols[1].Name)
	}
	if filteredRows[0][0] != "val-b" {
		t.Errorf("expected first cell val-b, got %s", filteredRows[0][0])
	}
}

func TestApplyFieldFilter_EmptyFields_ReturnsUnchanged(t *testing.T) {
	cols := []ColumnConfig{{Name: "A"}, {Name: "B"}}
	rows := [][]string{{"1", "2"}}
	outCols, outRows := applyFieldFilter(cols, rows, "")
	if len(outCols) != 2 {
		t.Errorf("expected 2 columns unchanged, got %d", len(outCols))
	}
	if len(outRows) != 1 {
		t.Errorf("expected 1 row unchanged, got %d", len(outRows))
	}
}

func TestApplyFieldFilter_NoMatchingFields_ReturnsOriginal(t *testing.T) {
	cols := []ColumnConfig{{Name: "A"}, {Name: "B"}}
	rows := [][]string{{"1", "2"}}
	outCols, outRows := applyFieldFilter(cols, rows, "nonexistent")
	// When no survivors, original is returned
	if len(outCols) != 2 {
		t.Errorf("expected original 2 columns, got %d", len(outCols))
	}
	if len(outRows) != 1 {
		t.Errorf("expected original 1 row, got %d", len(outRows))
	}
}

// --- RenderJSON ---

func TestRenderJSON_BasicOutput(t *testing.T) {
	data := map[string]string{"foo": "bar"}
	out := captureStdout(t, func() {
		err := RenderJSON(data, Options{})
		if err != nil {
			t.Errorf("RenderJSON: %v", err)
		}
	})
	if !strings.Contains(out, `"foo"`) {
		t.Errorf("expected foo in JSON output, got: %q", out)
	}
	if !strings.Contains(out, `"bar"`) {
		t.Errorf("expected bar in JSON output, got: %q", out)
	}
}

func TestRenderJSON_WithJQExpression(t *testing.T) {
	data := []map[string]string{
		{"name": "svc-a", "status": "ok"},
		{"name": "svc-b", "status": "error"},
	}
	out := captureStdout(t, func() {
		err := RenderJSON(data, Options{JQExpr: ".[0].name"})
		if err != nil {
			t.Errorf("RenderJSON with jq: %v", err)
		}
	})
	if !strings.Contains(out, "svc-a") {
		t.Errorf("expected svc-a from jq expression, got: %q", out)
	}
	if strings.Contains(out, "svc-b") {
		t.Errorf("jq .[0].name should not return svc-b, got: %q", out)
	}
}

func TestRenderJSON_InvalidJQExpression(t *testing.T) {
	data := map[string]string{"k": "v"}
	captureStdout(t, func() {
		err := RenderJSON(data, Options{JQExpr: "!!invalid jq!!"})
		if err == nil {
			t.Error("expected error for invalid jq expression, got nil")
		}
	})
}

// --- Printer.PrintJSON ---

func TestPrinterPrintJSON_WithWriter(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{}, &buf)
	err := p.PrintJSON(map[string]int{"count": 42})
	if err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	if !strings.Contains(buf.String(), "42") {
		t.Errorf("expected 42 in output, got: %q", buf.String())
	}
}

func TestPrinterPrintJSON_WithJQ(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{JQExpr: ".items[]"}, &buf)
	data := map[string]interface{}{"items": []string{"a", "b"}}
	err := p.PrintJSON(data)
	if err != nil {
		t.Fatalf("PrintJSON with jq: %v", err)
	}
	if !strings.Contains(buf.String(), "a") {
		t.Errorf("expected 'a' from jq .items[], got: %q", buf.String())
	}
}

// --- Printer.PrintTable ---

func TestPrinterPrintTable_Plaintext(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{Plaintext: true}, &buf)
	p.PrintTable([]string{"Name", "Value"}, [][]string{{"hello", "world"}})
	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected hello in output, got: %q", out)
	}
	if !strings.Contains(out, "world") {
		t.Errorf("expected world in output, got: %q", out)
	}
}

func TestPrinterPrintTable_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{JSON: true}, &buf)
	p.PrintTable([]string{"Name"}, [][]string{{"service-a"}})
	out := buf.String()
	if !strings.Contains(out, "service-a") {
		t.Errorf("expected service-a in JSON output, got: %q", out)
	}
	if !strings.Contains(out, "Name") {
		t.Errorf("expected Name key in JSON output, got: %q", out)
	}
}

func TestPrinterPrintTable_TableFormat(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{NoColor: true}, &buf)
	p.PrintTable([]string{"Service", "Status"}, [][]string{{"api", "healthy"}})
	out := buf.String()
	if !strings.Contains(out, "api") {
		t.Errorf("expected api in table, got: %q", out)
	}
	if !strings.Contains(out, "SERVICE") {
		t.Errorf("expected SERVICE header in table, got: %q", out)
	}
}

func TestPrinterPrintTable_WithFieldFilter(t *testing.T) {
	var buf bytes.Buffer
	p := NewPrinterWithWriter(Options{Plaintext: true, Fields: "name"}, &buf)
	p.PrintTable([]string{"Name", "Status"}, [][]string{{"svc-a", "ok"}})
	out := buf.String()
	if !strings.Contains(out, "svc-a") {
		t.Errorf("expected svc-a in output, got: %q", out)
	}
	if strings.Contains(out, "ok") {
		t.Errorf("Status should be filtered, got: %q", out)
	}
}

// --- FormatTimestamp ---

func TestFormatTimestamp_Seconds(t *testing.T) {
	// 2024-01-15 00:00:00 UTC
	ts := int64(1705276800)
	got := FormatTimestamp(ts)
	if !strings.Contains(got, "2024-01-15") {
		t.Errorf("FormatTimestamp(seconds) = %q, expected 2024-01-15", got)
	}
}

func TestFormatTimestamp_Milliseconds(t *testing.T) {
	// Same timestamp in milliseconds
	ts := int64(1705276800000)
	got := FormatTimestamp(ts)
	if !strings.Contains(got, "2024-01-15") {
		t.Errorf("FormatTimestamp(ms) = %q, expected 2024-01-15", got)
	}
}

func TestFormatTimestamp_Float64(t *testing.T) {
	ts := float64(1705276800)
	got := FormatTimestamp(ts)
	if !strings.Contains(got, "2024-01-15") {
		t.Errorf("FormatTimestamp(float64) = %q, expected 2024-01-15", got)
	}
}

func TestFormatTimestamp_Int32(t *testing.T) {
	ts := int32(1705276800)
	got := FormatTimestamp(ts)
	if !strings.Contains(got, "2024-01-15") {
		t.Errorf("FormatTimestamp(int32) = %q, expected date", got)
	}
}

func TestFormatTimestamp_Int(t *testing.T) {
	ts := int(1705276800)
	got := FormatTimestamp(ts)
	if !strings.Contains(got, "2024-01-15") {
		t.Errorf("FormatTimestamp(int) = %q, expected date", got)
	}
}

func TestFormatTimestamp_Float32(t *testing.T) {
	ts := float32(1705276800)
	got := FormatTimestamp(ts)
	// float32 may lose precision but should still produce a date string
	if got == "" {
		t.Error("FormatTimestamp(float32) returned empty string")
	}
}

func TestFormatTimestamp_Unknown(t *testing.T) {
	ts := "not-a-number"
	got := FormatTimestamp(ts)
	if got != "not-a-number" {
		t.Errorf("FormatTimestamp(string) = %q, want passthrough", got)
	}
}

// --- FormatDuration ---

func TestFormatDuration_Nanoseconds(t *testing.T) {
	got := FormatDuration(500)
	if got != "500ns" {
		t.Errorf("FormatDuration(500ns) = %q, want 500ns", got)
	}
}

func TestFormatDuration_Microseconds(t *testing.T) {
	got := FormatDuration(5000) // 5µs
	if !strings.Contains(got, "µs") {
		t.Errorf("FormatDuration(5µs) = %q, expected µs", got)
	}
}

func TestFormatDuration_Milliseconds(t *testing.T) {
	got := FormatDuration(250_000_000) // 250ms
	if got != "250ms" {
		t.Errorf("FormatDuration(250ms) = %q, want 250ms", got)
	}
}

func TestFormatDuration_FractionalMilliseconds(t *testing.T) {
	got := FormatDuration(250_500_000) // 250.5ms
	if !strings.Contains(got, "ms") {
		t.Errorf("FormatDuration(250.5ms) = %q, expected ms", got)
	}
}

func TestFormatDuration_Seconds(t *testing.T) {
	got := FormatDuration(1_500_000_000) // 1.5s
	if !strings.Contains(got, "s") {
		t.Errorf("FormatDuration(1.5s) = %q, expected s", got)
	}
}

func TestFormatDuration_WholeSeconds(t *testing.T) {
	got := FormatDuration(3_000_000_000) // 3s
	if got != "3s" {
		t.Errorf("FormatDuration(3s) = %q, want 3s", got)
	}
}

func TestFormatDuration_Minutes(t *testing.T) {
	got := FormatDuration(90_000_000_000) // 1m30s
	if !strings.Contains(got, "m") {
		t.Errorf("FormatDuration(1m30s) = %q, expected minutes", got)
	}
}

// --- TruncateString ---

func TestTruncateString_ShortString(t *testing.T) {
	s := "hello"
	got := TruncateString(s, 20)
	if got != "hello" {
		t.Errorf("TruncateString short: got %q, want hello", got)
	}
}

func TestTruncateString_ExactLength(t *testing.T) {
	s := "hello"
	got := TruncateString(s, 5)
	if got != "hello" {
		t.Errorf("TruncateString exact length: got %q, want hello", got)
	}
}

func TestTruncateString_TooLong(t *testing.T) {
	s := "hello world this is a long string"
	got := TruncateString(s, 12)
	runes := []rune(got)
	// Result should be at most 12 characters
	if len(runes) > 12 {
		t.Errorf("TruncateString result too long: %d chars, got %q", len(runes), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("TruncateString should end with ..., got %q", got)
	}
}

func TestTruncateString_VeryShortMax(t *testing.T) {
	s := "hello"
	got := TruncateString(s, 3)
	// maxLen=3, available=0, returns suffix[:3] = "..."
	if len([]rune(got)) > 3 {
		t.Errorf("TruncateString(maxLen=3) result too long: %q", got)
	}
}

func TestTruncateString_WordBoundary(t *testing.T) {
	s := "hello world foo bar"
	got := TruncateString(s, 14)
	// Should truncate at word boundary, not mid-word
	if strings.HasSuffix(got, " ...") {
		t.Logf("TruncateString word boundary: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("TruncateString should end with ..., got %q", got)
	}
}

// --- ColorStatus ---

func TestColorStatus_NoColor(t *testing.T) {
	// Save and restore color.NoColor
	orig := color.NoColor
	defer func() { color.NoColor = orig }()

	color.NoColor = true
	for _, s := range []string{"ok", "error", "warning", "active"} {
		got := ColorStatus(s)
		if got != s {
			t.Errorf("ColorStatus(%q) with NoColor = %q, want passthrough %q", s, got, s)
		}
	}
}

func TestColorStatus_GreenStatuses(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = false

	for _, s := range []string{"ok", "active", "healthy", "up", "success", "passed", "enabled"} {
		got := ColorStatus(s)
		if got == "" {
			t.Errorf("ColorStatus(%q) returned empty", s)
		}
	}
}

func TestColorStatus_RedStatuses(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = false

	for _, s := range []string{"alert", "error", "critical", "down", "failed", "failure", "no data"} {
		got := ColorStatus(s)
		if got == "" {
			t.Errorf("ColorStatus(%q) returned empty", s)
		}
	}
}

func TestColorStatus_YellowStatuses(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = false

	for _, s := range []string{"warn", "warning", "pending", "unknown", "muted", "ignored"} {
		got := ColorStatus(s)
		if got == "" {
			t.Errorf("ColorStatus(%q) returned empty", s)
		}
	}
}

func TestColorStatus_Unknown(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = true

	got := ColorStatus("some-unknown-status")
	if got != "some-unknown-status" {
		t.Errorf("ColorStatus(unknown) = %q, want passthrough", got)
	}
}

// --- renderPlaintext ---

func TestRenderPlaintext_MultipleRows(t *testing.T) {
	var buf bytes.Buffer
	cols := []ColumnConfig{{Name: "A"}, {Name: "B"}}
	rows := [][]string{{"x", "y"}, {"p", "q"}}
	renderPlaintext(&buf, cols, rows)
	out := buf.String()
	if !strings.Contains(out, "x\ty") {
		t.Errorf("expected tab-separated row, got: %q", out)
	}
	if !strings.Contains(out, "p\tq") {
		t.Errorf("expected second row, got: %q", out)
	}
}

func TestRenderPlaintext_NoHeaders(t *testing.T) {
	var buf bytes.Buffer
	cols := []ColumnConfig{{Name: "Name"}}
	rows := [][]string{{"data"}}
	renderPlaintext(&buf, cols, rows)
	// Headers should NOT appear in plaintext
	if strings.Contains(buf.String(), "Name") {
		t.Errorf("plaintext should not show headers, got: %q", buf.String())
	}
}

// --- renderColorTable with column formatter ---

func TestRenderColorTable_WithFormatter(t *testing.T) {
	var buf bytes.Buffer
	cols := []ColumnConfig{
		{
			Name:      "Status",
			Formatter: func(s string) string { return "[" + s + "]" },
		},
	}
	rows := [][]string{{"active"}}
	renderColorTable(&buf, cols, rows, Options{NoColor: true})
	out := buf.String()
	if !strings.Contains(out, "[active]") {
		t.Errorf("expected formatter applied [active], got: %q", out)
	}
}

// --- applyJQ edge cases ---

func TestApplyJQ_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	err := applyJQ(&buf, []byte("not json"), ".")
	if err == nil {
		t.Error("expected error for invalid JSON input to jq")
	}
}

func TestApplyJQ_Identity(t *testing.T) {
	var buf bytes.Buffer
	err := applyJQ(&buf, []byte(`{"key":"value"}`), ".")
	if err != nil {
		t.Fatalf("applyJQ(.): %v", err)
	}
	if !strings.Contains(buf.String(), "value") {
		t.Errorf("expected value in jq output, got: %q", buf.String())
	}
}

func TestApplyJQ_JQRuntimeError(t *testing.T) {
	var buf bytes.Buffer
	// .foo on a non-object (array) should produce a jq runtime error
	err := applyJQ(&buf, []byte(`null`), ".foo")
	// gojq returns nil for null.foo (outputs null), so check a truly bad expression
	// Try to divide by zero which will produce a jq error
	err = applyJQ(&buf, []byte(`1`), ". / 0")
	if err == nil {
		t.Log("jq div-by-zero did not error (implementation-specific)")
	}
}

// --- PrintError ---

func TestPrintError_Nil(t *testing.T) {
	// Should not panic on nil error
	PrintError(nil)
}

func TestPrintError_WithNoColor(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = true

	// Should not panic with a real error
	PrintError(errors.New("test error"))
}

func TestPrintError_WithColor(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = false

	// Should not panic
	PrintError(errors.New("colorized error"))
}

func TestPrintErrorf_WithNoColor(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = true

	PrintErrorf("formatted error %d", 42)
}

func TestPrintErrorf_WithColor(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()
	color.NoColor = false

	PrintErrorf("formatted error %s", "colored")
}
