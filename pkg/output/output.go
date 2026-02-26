package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/fatih/color"
	"github.com/itchyny/gojq"
	"github.com/olekukonko/tablewriter"
)

// Options controls output formatting.
type Options struct {
	JSON      bool
	Plaintext bool
	NoColor   bool
	Fields    string // comma-separated column names to include
	JQExpr    string // jq expression for filtering/transforming JSON output
	Debug     bool
}

// ColumnConfig defines display configuration for a single table column.
type ColumnConfig struct {
	Name      string              // column header label
	Width     int                 // max width (0 = auto)
	Color     *color.Color        // optional color for the column values
	Formatter func(string) string // optional transform applied to each cell value
}

// Printer handles formatted output.
type Printer struct {
	opts   Options
	writer io.Writer
}

// NewPrinter creates a new Printer with the given options.
func NewPrinter(opts Options) *Printer {
	if opts.NoColor {
		color.NoColor = true
	}
	return &Printer{
		opts:   opts,
		writer: os.Stdout,
	}
}

// NewPrinterWithWriter creates a new Printer writing to the given writer.
func NewPrinterWithWriter(opts Options, w io.Writer) *Printer {
	if opts.NoColor {
		color.NoColor = true
	}
	return &Printer{
		opts:   opts,
		writer: w,
	}
}

// RenderTable renders tabular data to the printer's writer.
//
// columns defines headers, widths, colors, and per-cell formatters.
// rows is the raw string data (one slice per row, matching columns).
// rawData is the original structured data used for JSON output.
// opts overrides the printer's own options when non-zero.
func RenderTable(columns []ColumnConfig, rows [][]string, rawData interface{}, opts Options) error {
	if opts.NoColor {
		color.NoColor = true
	}

	w := os.Stdout

	// Filter columns if --fields is set.
	columns, rows = applyFieldFilter(columns, rows, opts.Fields)

	if opts.JSON {
		return RenderJSON(rawData, opts)
	}

	if opts.Plaintext {
		renderPlaintext(w, columns, rows)
		return nil
	}

	renderColorTable(w, columns, rows, opts)
	return nil
}

// RenderJSON marshals data as indented JSON, applying opts.JQExpr if set.
func RenderJSON(data interface{}, opts Options) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	if opts.JQExpr != "" {
		return applyJQ(os.Stdout, b, opts.JQExpr)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return fmt.Errorf("indenting JSON: %w", err)
	}
	_, _ = fmt.Fprintln(os.Stdout, buf.String())
	return nil
}

// PrintError prints a user-friendly error message to stderr.
// In debug mode the full error chain is preserved; otherwise only the
// top-level message is shown without stack traces.
func PrintError(err error) {
	if err == nil {
		return
	}
	if color.NoColor {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "%s %v\n", color.RedString("Error:"), err)
	}
}

// PrintErrorf prints a formatted error message to stderr.
func PrintErrorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if color.NoColor {
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("Error:"), msg)
	}
}

// --- Printer methods (instance-based API) ---

// PrintJSON outputs data as formatted JSON, optionally applying a jq expression.
func (p *Printer) PrintJSON(data interface{}) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	if p.opts.JQExpr != "" {
		return applyJQ(p.writer, b, p.opts.JQExpr)
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return fmt.Errorf("indenting JSON: %w", err)
	}
	_, _ = fmt.Fprintln(p.writer, buf.String())
	return nil
}

// PrintTable renders data as a table using the printer's options.
// headers is the column names, rows is the data rows.
func (p *Printer) PrintTable(headers []string, rows [][]string) {
	if p.opts.JSON {
		result := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			m := make(map[string]string, len(headers))
			for i, h := range headers {
				if i < len(row) {
					m[h] = row[i]
				}
			}
			result = append(result, m)
		}
		_ = p.PrintJSON(result)
		return
	}

	// Build ColumnConfig slice from plain headers.
	cols := make([]ColumnConfig, len(headers))
	for i, h := range headers {
		cols[i] = ColumnConfig{Name: h}
	}

	// Apply field filter.
	cols, rows = applyFieldFilter(cols, rows, p.opts.Fields)

	if p.opts.Plaintext {
		renderPlaintext(p.writer, cols, rows)
		return
	}

	renderColorTable(p.writer, cols, rows, p.opts)
}

// --- Helper functions ---

// FormatTimestamp formats a unix timestamp (int or float, seconds or milliseconds)
// to a human-readable string such as "2024-01-15 09:30:00".
func FormatTimestamp(ts interface{}) string {
	var secs int64
	switch v := ts.(type) {
	case int:
		secs = int64(v)
	case int32:
		secs = int64(v)
	case int64:
		secs = v
	case float32:
		secs = int64(v)
	case float64:
		secs = int64(v)
	default:
		return fmt.Sprintf("%v", ts)
	}

	// Detect milliseconds: unix epoch in ms is ~13 digits; in seconds ~10.
	if secs > 1e12 {
		secs = secs / 1000
	}
	t := time.Unix(secs, 0).UTC()
	return t.Format("2006-01-02 15:04:05")
}

// FormatDuration formats nanoseconds to a compact human-readable string.
// Examples: 250000000 → "250ms", 1500000000 → "1.5s", 90000000000 → "1m30s".
func FormatDuration(ns int64) string {
	d := time.Duration(ns)

	if d < time.Microsecond {
		return fmt.Sprintf("%dns", ns)
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(ns)/float64(time.Microsecond))
	}
	if d < time.Second {
		ms := float64(ns) / float64(time.Millisecond)
		if ms == float64(int(ms)) {
			return fmt.Sprintf("%dms", int(ms))
		}
		return fmt.Sprintf("%.1fms", ms)
	}
	if d < time.Minute {
		s := float64(ns) / float64(time.Second)
		if s == float64(int(s)) {
			return fmt.Sprintf("%ds", int(s))
		}
		return fmt.Sprintf("%.1fs", s)
	}
	// Use Go's built-in for longer durations.
	return d.Round(time.Second).String()
}

// TruncateString truncates s at a word boundary so the result (including "…")
// is at most maxLen runes. If s already fits, it is returned unchanged.
func TruncateString(s string, maxLen int) string {
	const suffix = "..."
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	available := maxLen - len([]rune(suffix))
	if available <= 0 {
		return suffix[:maxLen]
	}

	truncated := string(runes[:available])
	// Back up to the last word boundary.
	lastSpace := strings.LastIndexFunc(truncated, unicode.IsSpace)
	if lastSpace > available/2 {
		truncated = truncated[:lastSpace]
	}

	return strings.TrimRight(truncated, " \t") + suffix
}

// ColorStatus returns the status string colored according to its severity:
//   - green: ok, active, healthy, up, success, passed
//   - red:   alert, error, critical, down, failed, failure
//   - yellow: warn, warning, pending, unknown, muted
//   - no color: everything else
func ColorStatus(status string) string {
	if color.NoColor {
		return status
	}
	lower := strings.ToLower(strings.TrimSpace(status))
	switch lower {
	case "ok", "active", "healthy", "up", "success", "passed", "enabled":
		return color.GreenString(status)
	case "alert", "error", "critical", "down", "failed", "failure", "no data":
		return color.RedString(status)
	case "warn", "warning", "pending", "unknown", "muted", "ignored":
		return color.YellowString(status)
	default:
		return status
	}
}

// --- Internal helpers ---

// applyFieldFilter reduces columns and rows to only the requested fields.
// fieldsCSV is a comma-separated list of column names (case-insensitive).
// If fieldsCSV is empty, original columns and rows are returned unchanged.
func applyFieldFilter(columns []ColumnConfig, rows [][]string, fieldsCSV string) ([]ColumnConfig, [][]string) {
	if fieldsCSV == "" {
		return columns, rows
	}

	// Build a set of requested names (lowercase).
	requested := make(map[string]int) // name → desired order index
	parts := strings.Split(fieldsCSV, ",")
	for i, p := range parts {
		requested[strings.ToLower(strings.TrimSpace(p))] = i
	}

	// Find which original column indices survive, preserving requested order.
	type colIndex struct {
		origIdx int
		col     ColumnConfig
		order   int
	}
	var survivors []colIndex
	for i, col := range columns {
		lower := strings.ToLower(strings.TrimSpace(col.Name))
		if order, ok := requested[lower]; ok {
			survivors = append(survivors, colIndex{origIdx: i, col: col, order: order})
		}
	}

	// Sort by requested order.
	for i := 0; i < len(survivors)-1; i++ {
		for j := i + 1; j < len(survivors); j++ {
			if survivors[j].order < survivors[i].order {
				survivors[i], survivors[j] = survivors[j], survivors[i]
			}
		}
	}

	if len(survivors) == 0 {
		return columns, rows
	}

	// Build filtered columns.
	filteredCols := make([]ColumnConfig, len(survivors))
	idxMap := make([]int, len(survivors))
	for i, s := range survivors {
		filteredCols[i] = s.col
		idxMap[i] = s.origIdx
	}

	// Build filtered rows.
	filteredRows := make([][]string, len(rows))
	for r, row := range rows {
		newRow := make([]string, len(survivors))
		for i, origIdx := range idxMap {
			if origIdx < len(row) {
				newRow[i] = row[origIdx]
			}
		}
		filteredRows[r] = newRow
	}

	return filteredCols, filteredRows
}

// renderPlaintext outputs tab-separated values without headers or borders.
func renderPlaintext(w io.Writer, columns []ColumnConfig, rows [][]string) {
	_ = columns // headers suppressed in plaintext mode
	for _, row := range rows {
		cells := make([]string, len(row))
		copy(cells, row)
		_, _ = fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
}

// renderColorTable renders a bordered, optionally colorized table.
func renderColorTable(w io.Writer, columns []ColumnConfig, rows [][]string, opts Options) {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.Name
	}

	table := tablewriter.NewWriter(w)
	table.SetHeader(headers)
	table.SetBorder(true)
	table.SetRowLine(false)
	table.SetAutoWrapText(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	// Apply per-column max widths if set.
	for i, col := range columns {
		if col.Width > 0 {
			table.SetColMinWidth(i, 0)
			// tablewriter doesn't support per-column max, so we'll truncate in rows.
			_ = col.Width
		}
	}

	for _, row := range rows {
		formatted := make([]string, len(columns))
		for i, col := range columns {
			var cell string
			if i < len(row) {
				cell = row[i]
			}

			// Apply formatter if set.
			if col.Formatter != nil {
				cell = col.Formatter(cell)
			}

			// Truncate if column has a max width.
			if col.Width > 0 && len([]rune(cell)) > col.Width {
				cell = TruncateString(cell, col.Width)
			}

			// Apply color if set and colors are enabled.
			if !opts.NoColor && col.Color != nil {
				cell = col.Color.Sprint(cell)
			}

			formatted[i] = cell
		}
		table.Append(formatted)
	}

	table.Render()
}

// applyJQ parses and runs a jq expression against JSON bytes, writing each
// result value as indented JSON to w.
func applyJQ(w io.Writer, data []byte, expr string) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("parsing jq expression %q: %w", expr, err)
	}

	var input interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("parsing JSON for jq: %w", err)
	}

	iter := query.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if jqErr, ok := v.(error); ok {
			return fmt.Errorf("jq error: %w", jqErr)
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling jq result: %w", err)
		}
		_, _ = fmt.Fprintln(w, string(b))
	}
	return nil
}
