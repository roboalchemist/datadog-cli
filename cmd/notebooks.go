package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
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

	// Wire up subcommands
	notebooksCmd.AddCommand(notebooksListCmd)
	notebooksCmd.AddCommand(notebooksGetCmd)

	// Register with root
	rootCmd.AddCommand(notebooksCmd)
}
