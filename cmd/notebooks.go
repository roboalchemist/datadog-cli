package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// maxNotebooksPageSize is the maximum count accepted by the Datadog notebooks v1 API.
const maxNotebooksPageSize = 100

// ---- notebooks command group ----

var notebooksCmd = &cobra.Command{
	Use:   "notebooks",
	Short: "Query notebooks from Datadog",
	Long:  `Query notebooks from Datadog.`,
	Example: `  # List all notebooks
  datadog-cli notebooks list

  # Get details for a specific notebook
  datadog-cli notebooks get 123456

  # List notebooks in JSON format
  datadog-cli notebooks list --json`,
}

// ---- notebooks list ----

var notebooksListAll bool

var notebooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notebooks",
	Long: `List all notebooks from Datadog.

Uses GET /api/v1/notebooks.`,
	Example: `  # List all notebooks
  datadog-cli notebooks list

  # List notebooks in JSON format
  datadog-cli notebooks list --json`,
	RunE: runNotebooksList,
}

func runNotebooksList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if notebooksListAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// First page size: min(effectiveLimit, maxNotebooksPageSize).
	// If unlimited (--all), use max page size.
	firstPageSize := effectiveLimit
	if firstPageSize == 0 || firstPageSize > maxNotebooksPageSize {
		firstPageSize = maxNotebooksPageSize
	}

	type notebookRow struct {
		ID       string
		Name     string
		Author   string
		Created  string
		Modified string
	}

	var rows []notebookRow
	var allData []interface{}
	var lastRaw map[string]interface{}
	start := 0
	pageNum := 0

	needsMorePages := func() bool {
		if notebooksListAll {
			return true // stop only when no more data
		}
		return len(rows) < effectiveLimit
	}

	for needsMorePages() {
		pageNum++

		pageSize := firstPageSize
		if !notebooksListAll {
			remaining := effectiveLimit - len(rows)
			if remaining < pageSize {
				pageSize = remaining
			}
		}

		params := url.Values{}
		params.Set("count", fmt.Sprintf("%d", pageSize))
		params.Set("start", fmt.Sprintf("%d", start))

		if pageNum > 1 && !flagQuiet {
			if notebooksListAll {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d notebooks so far)...\n", pageNum, len(rows))
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
			}
		}

		respBytes, err := client.Get("/api/v1/notebooks", params)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		data, _ := raw["data"].([]interface{})
		if len(data) == 0 {
			break
		}

		for _, item := range data {
			if !notebooksListAll && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)

			nb, _ := item.(map[string]interface{})
			attrs, _ := nb["attributes"].(map[string]interface{})

			id := formatID(nb["id"])
			name := output.TruncateString(notebooksStringField(attrs, "name"), 45)
			author := notebooksAuthor(attrs)
			created := notebooksFormatTimestamp(attrs["created"])
			modified := notebooksFormatTimestamp(attrs["modified"])

			rows = append(rows, notebookRow{
				ID:       id,
				Name:     name,
				Author:   author,
				Created:  created,
				Modified: modified,
			})
		}

		start += len(data)

		// If we got fewer items than the page size, we've reached the last page.
		if len(data) < pageSize {
			break
		}
	}

	if opts.JSON {
		merged := map[string]interface{}{
			"data": allData,
		}
		if lastRaw != nil {
			if meta, ok := lastRaw["meta"]; ok {
				merged["meta"] = meta
			}
		}
		return output.RenderJSON(merged, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No notebooks found.")
		return nil
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{r.ID, r.Name, r.Author, r.Created, r.Modified})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 10},
		{Name: "Name", Width: 45},
		{Name: "Author", Width: 25},
		{Name: "Created", Width: 18},
		{Name: "Modified", Width: 18},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- notebooks get ----

var notebooksGetCmd = &cobra.Command{
	Use:   "get <notebook_id>",
	Short: "Get notebook details by ID",
	Long: `Get detailed information about a specific notebook.

Uses GET /api/v1/notebooks/{id}.`,
	Example: `  # Get details for a specific notebook
  datadog-cli notebooks get 123456

  # Get notebook details in JSON format
  datadog-cli notebooks get 123456 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runNotebooksGet,
}

func runNotebooksGet(cmd *cobra.Command, args []string) error {
	notebookID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/notebooks/"+notebookID, nil)
	if err != nil {
		return err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if opts.JSON {
		return output.RenderJSON(raw, opts)
	}

	// v1 API wraps response in "data"
	nb, ok := raw["data"].(map[string]interface{})
	if !ok {
		nb = raw
	}
	attrs, _ := nb["attributes"].(map[string]interface{})

	id := formatID(nb["id"])
	name := notebooksStringField(attrs, "name")
	author := notebooksAuthor(attrs)
	authorHandle := ""
	if authorMap, ok := attrs["author"].(map[string]interface{}); ok {
		authorHandle = notebooksStringField(authorMap, "handle")
	}
	status := notebooksStringField(attrs, "status")
	created := notebooksFormatTimestamp(attrs["created"])
	modified := notebooksFormatTimestamp(attrs["modified"])

	cellCount := 0
	if cells, ok := attrs["cells"].([]interface{}); ok {
		cellCount = len(cells)
	}

	isTemplate := "No"
	notebookType := ""
	if metadata, ok := attrs["metadata"].(map[string]interface{}); ok {
		if v, ok := metadata["is_template"].(bool); ok && v {
			isTemplate = "Yes"
		}
		notebookType = notebooksStringField(metadata, "type")
	}

	_, _ = fmt.Fprintf(os.Stdout, "Notebook: %s\n\n", name)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Name", name},
		{"Author", author},
		{"Author Handle", authorHandle},
		{"Status", status},
		{"Cell Count", fmt.Sprintf("%d", cellCount)},
		{"Created", created},
		{"Modified", modified},
		{"Is Template", isTemplate},
		{"Type", notebookType},
	}

	rows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" || d.v == "0" {
			continue
		}
		rows = append(rows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 15},
		{Name: "Value", Width: 80},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- notebooks create ----

// notebooksCreateFlags holds the flag values for the create subcommand.
var notebooksCreateFlags struct {
	name     string
	metric   string
	start    string
	end      string
	liveSpan string
	hlines   []string
	vlines   []string
	note     string
}

var notebooksCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new notebook",
	Long: `Create a new notebook in Datadog.

Uses POST /api/v1/notebooks.`,
	Example: `  # Create a notebook with a metric timeseries over an absolute time range
  datadog-cli notebooks create \
    --name "Apr 7 Incident" \
    --metric "avg:karpenter.cluster_state.node_count{*}" \
    --start "2026-04-07 07:00" --end "2026-04-08 17:00" \
    --hline "175:ok dashed:Pre-incident baseline" \
    --vline "2026-04-07 11:31:error bold:Mass eviction" \
    --note "## Annotation\n\nNotes here."

  # Create a note-only notebook with live span
  datadog-cli notebooks create --name "My Notes" --live-span "1h" --note "## Notes"

  # Output raw API response as JSON
  datadog-cli notebooks create --name "Test" --metric "avg:system.cpu.user{*}" --json`,
	RunE: runNotebooksCreate,
}

// parseHLine parses a --hline value of the form "VALUE:DISPLAY_TYPE:LABEL"
// into a Datadog notebook marker map.
func parseHLine(s string) (map[string]interface{}, error) {
	// Split on ":" — first segment is the numeric/string value, second is display_type,
	// third and beyond are the label (allow colons in label).
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid --hline %q: expected VALUE:DISPLAY_TYPE[:LABEL]", s)
	}
	value := strings.TrimSpace(parts[0])
	displayType := strings.TrimSpace(parts[1])
	label := ""
	if len(parts) == 3 {
		label = strings.TrimSpace(parts[2])
	}
	marker := map[string]interface{}{
		"value":        "y = " + value,
		"display_type": displayType,
	}
	if label != "" {
		marker["label"] = label
	}
	return marker, nil
}

// parseVLine parses a --vline value of the form "DATETIME:DISPLAY_TYPE[:LABEL]"
// into a Datadog notebook marker map using epoch seconds in a 2-minute band.
func parseVLine(s string) (map[string]interface{}, error) {
	// The datetime part may contain a space (e.g. "2026-04-07 11:31"), so we
	// cannot simply split on ":". Strategy: try progressively longer prefixes
	// until a valid datetime is found.
	//
	// Supported datetime formats (all treated as PT → UTC conversion is
	// performed by parseNotebookTime which loads the America/Los_Angeles zone).
	// We find the split point by trying to parse up to the first potential
	// DISPLAY_TYPE separator. The display_type and label come after.

	// Find the last occurrence of a segment that could be a display_type keyword
	// by iterating from the right.  We look for a ":" that separates the timestamp
	// part from the style part.  The style part is either the last token or
	// second-to-last (when a label is also present).
	//
	// Algorithm: split everything by ":", then try combining prefix tokens as the
	// timestamp and see if the timestamp parses.

	colonParts := strings.Split(s, ":")
	if len(colonParts) < 2 {
		return nil, fmt.Errorf("invalid --vline %q: expected DATETIME:DISPLAY_TYPE[:LABEL]", s)
	}

	// Try increasing numbers of colon-joined prefix tokens as the timestamp.
	// A valid timestamp needs at least 1 token (bare date) and the remaining
	// tokens provide display_type and optional label.
	type candidate struct {
		ts          string
		displayType string
		label       string
	}
	var chosen *candidate

	for prefixLen := 1; prefixLen <= len(colonParts)-1; prefixLen++ {
		ts := strings.Join(colonParts[:prefixLen], ":")
		rest := colonParts[prefixLen:]

		// Try to parse the timestamp — we accept "YYYY-MM-DD HH:MM" style,
		// which means it may span two colon-parts (date part + HH + MM).
		// parseNotebookTime accepts "2006-01-02 15:04" format.
		if _, err := parseNotebookTime(ts); err == nil {
			displayType := strings.TrimSpace(rest[0])
			label := ""
			if len(rest) > 1 {
				label = strings.TrimSpace(strings.Join(rest[1:], ":"))
			}
			chosen = &candidate{ts: ts, displayType: displayType, label: label}
			// Keep going — prefer longer timestamp matches (e.g. full "HH:MM").
		}
	}

	if chosen == nil {
		return nil, fmt.Errorf("invalid --vline %q: could not parse a valid datetime prefix", s)
	}

	epochSecs, err := parseNotebookTime(chosen.ts)
	if err != nil {
		return nil, fmt.Errorf("invalid --vline timestamp %q: %w", chosen.ts, err)
	}

	// Use a 2-minute band centred on the event time.
	epochStart := epochSecs
	epochEnd := epochSecs + 120

	marker := map[string]interface{}{
		"value":        fmt.Sprintf("%d < t < %d", epochStart, epochEnd),
		"display_type": chosen.displayType,
	}
	if chosen.label != "" {
		marker["label"] = chosen.label
	}
	return marker, nil
}

// parseNotebookTime parses a human-readable datetime string (assumed PT) into
// Unix epoch seconds.  Supports the following formats:
//   - "YYYY-MM-DD HH:MM"   (PT, assumed — loads America/Los_Angeles)
//   - "YYYY-MM-DDTHH:MM"   (same, ISO-style without seconds)
//   - "YYYY-MM-DD"         (midnight PT)
func parseNotebookTime(s string) (int64, error) {
	s = strings.TrimSpace(s)

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fall back to UTC if TZ data unavailable.
		loc = time.UTC
	}

	formats := []string{
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("unrecognised datetime %q (supported: \"YYYY-MM-DD HH:MM\" or \"YYYY-MM-DD\")", s)
}

func runNotebooksCreate(cmd *cobra.Command, args []string) error {
	if notebooksCreateFlags.name == "" {
		return fmt.Errorf("--name is required")
	}

	client := newClient()
	opts := GetOutputOptions()

	// ---- time range ----
	var notebookTime map[string]interface{}
	if notebooksCreateFlags.liveSpan != "" {
		notebookTime = map[string]interface{}{
			"live_span": notebooksCreateFlags.liveSpan,
		}
	} else if notebooksCreateFlags.start != "" && notebooksCreateFlags.end != "" {
		startSecs, err := parseNotebookTime(notebooksCreateFlags.start)
		if err != nil {
			return fmt.Errorf("invalid --start: %w", err)
		}
		endSecs, err := parseNotebookTime(notebooksCreateFlags.end)
		if err != nil {
			return fmt.Errorf("invalid --end: %w", err)
		}
		notebookTime = map[string]interface{}{
			"start": strconv.FormatInt(startSecs, 10),
			"end":   strconv.FormatInt(endSecs, 10),
		}
	} else if notebooksCreateFlags.start != "" || notebooksCreateFlags.end != "" {
		return fmt.Errorf("both --start and --end are required when specifying an absolute time range")
	} else {
		// Default to last 1 hour
		notebookTime = map[string]interface{}{
			"live_span": "1h",
		}
	}

	// ---- markers ----
	var markers []map[string]interface{}
	for _, h := range notebooksCreateFlags.hlines {
		m, err := parseHLine(h)
		if err != nil {
			return err
		}
		markers = append(markers, m)
	}
	for _, v := range notebooksCreateFlags.vlines {
		m, err := parseVLine(v)
		if err != nil {
			return err
		}
		markers = append(markers, m)
	}

	// ---- cells ----
	var cells []map[string]interface{}

	// Note cell (optional, placed first)
	if notebooksCreateFlags.note != "" {
		noteCell := map[string]interface{}{
			"type": "notebook_cells",
			"attributes": map[string]interface{}{
				"definition": map[string]interface{}{
					"type": "markdown",
					"text": notebooksCreateFlags.note,
				},
			},
		}
		cells = append(cells, noteCell)
	}

	// Timeseries cell (optional)
	if notebooksCreateFlags.metric != "" {
		tsDefinition := map[string]interface{}{
			"type": "timeseries",
			"requests": []map[string]interface{}{
				{
					"q":            notebooksCreateFlags.metric,
					"display_type": "line",
				},
			},
		}
		if len(markers) > 0 {
			tsDefinition["markers"] = markers
		}
		tsCell := map[string]interface{}{
			"type": "notebook_cells",
			"attributes": map[string]interface{}{
				"definition": tsDefinition,
			},
		}
		cells = append(cells, tsCell)
	}

	// If no cells at all, create an empty markdown cell as a placeholder.
	if len(cells) == 0 {
		cells = append(cells, map[string]interface{}{
			"type": "notebook_cells",
			"attributes": map[string]interface{}{
				"definition": map[string]interface{}{
					"type": "markdown",
					"text": "",
				},
			},
		})
	}

	// ---- request body ----
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "notebooks",
			"attributes": map[string]interface{}{
				"name":  notebooksCreateFlags.name,
				"time":  notebookTime,
				"cells": cells,
			},
		},
	}

	respBytes, err := client.Post("/api/v1/notebooks", body, nil)
	if err != nil {
		return err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if opts.JSON {
		return output.RenderJSON(raw, opts)
	}

	// Extract notebook ID from response and print URL.
	nb, ok := raw["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected response shape: missing \"data\"")
	}
	id := formatID(nb["id"])
	if id == "" || id == "<nil>" {
		return fmt.Errorf("could not extract notebook ID from response")
	}

	// Build URL: name becomes a URL slug (lowercase, spaces→hyphens, strip non-word chars).
	slug := buildNotebookSlug(notebooksCreateFlags.name)
	notebookURL := fmt.Sprintf("https://app.datadoghq.com/notebook/%s/%s", id, slug)
	_, _ = fmt.Fprintln(os.Stdout, notebookURL)
	return nil
}

// buildNotebookSlug converts a notebook name into a URL-friendly slug.
func buildNotebookSlug(name string) string {
	slug := strings.ToLower(name)
	// Replace spaces and underscores with hyphens.
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	// Keep only alphanumeric characters and hyphens.
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	// Collapse consecutive hyphens.
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	return result
}

// ---- helpers ----

// notebooksStringField safely extracts a string value from a map.
func notebooksStringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// notebooksAuthor extracts the author name or handle from notebook attributes.
func notebooksAuthor(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	authorMap, ok := attrs["author"].(map[string]interface{})
	if !ok {
		return ""
	}
	name := notebooksStringField(authorMap, "name")
	if name != "" {
		return output.TruncateString(name, 25)
	}
	handle := notebooksStringField(authorMap, "handle")
	return output.TruncateString(handle, 25)
}

// notebooksFormatTimestamp formats an ISO 8601 or Unix timestamp for display.
func notebooksFormatTimestamp(ts interface{}) string {
	if ts == nil {
		return ""
	}
	switch v := ts.(type) {
	case float64:
		t := time.Unix(int64(v), 0)
		return t.UTC().Format("2006-01-02 15:04")
	case string:
		if v == "" {
			return ""
		}
		normalized := v
		if strings.HasSuffix(normalized, "Z") {
			normalized = normalized[:len(normalized)-1] + "+00:00"
		}
		t, err := time.Parse(time.RFC3339, normalized)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05", v)
			if err != nil {
				return v
			}
		}
		return t.UTC().Format("2006-01-02 15:04")
	}
	return fmt.Sprintf("%v", ts)
}

// ---- init ----

func init() {
	// notebooks list flags
	notebooksListCmd.Flags().BoolVar(&notebooksListAll, "all", false, "Fetch all pages until exhausted (overrides --limit)")

	// notebooks create flags
	cf := notebooksCreateCmd.Flags()
	cf.StringVar(&notebooksCreateFlags.name, "name", "", "Notebook name (required)")
	cf.StringVar(&notebooksCreateFlags.metric, "metric", "", "Metric query for timeseries cell (e.g. avg:system.cpu.user{*})")
	cf.StringVar(&notebooksCreateFlags.start, "start", "", "Absolute start time: \"YYYY-MM-DD HH:MM\" (PT)")
	cf.StringVar(&notebooksCreateFlags.end, "end", "", "Absolute end time: \"YYYY-MM-DD HH:MM\" (PT)")
	cf.StringVar(&notebooksCreateFlags.liveSpan, "live-span", "", "Relative time window e.g. 1h, 4h, 1d (alternative to --start/--end)")
	cf.StringArrayVar(&notebooksCreateFlags.hlines, "hline", nil, "Horizontal marker: VALUE:DISPLAY_TYPE[:LABEL] (repeatable)")
	cf.StringArrayVar(&notebooksCreateFlags.vlines, "vline", nil, "Vertical marker: DATETIME:DISPLAY_TYPE[:LABEL] (repeatable)")
	cf.StringVar(&notebooksCreateFlags.note, "note", "", "Markdown text for a note cell placed before the timeseries cell")

	// Wire up subcommands
	notebooksCmd.AddCommand(notebooksListCmd)
	notebooksCmd.AddCommand(notebooksGetCmd)
	notebooksCmd.AddCommand(notebooksCreateCmd)

	// Register with root
	rootCmd.AddCommand(notebooksCmd)
}
