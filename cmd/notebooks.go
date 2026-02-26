package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/output"
)

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

	params := url.Values{}
	params.Set("count", fmt.Sprintf("%d", min(flagLimit, 100)))

	respBytes, err := client.Get("/api/v1/notebooks", params)
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

	notebooksRaw, _ := raw["data"].([]interface{})
	if len(notebooksRaw) == 0 {
		fmt.Fprintln(os.Stdout, "No notebooks found.")
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(notebooksRaw) > flagLimit {
		notebooksRaw = notebooksRaw[:flagLimit]
	}

	type notebookRow struct {
		ID       string
		Name     string
		Author   string
		Created  string
		Modified string
	}

	rows := make([]notebookRow, 0, len(notebooksRaw))
	tableRows := make([][]string, 0, len(notebooksRaw))

	for _, item := range notebooksRaw {
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
		tableRows = append(tableRows, []string{id, name, author, created, modified})
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

	fmt.Fprintf(os.Stdout, "Notebook: %s\n\n", name)

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
	// Wire up subcommands
	notebooksCmd.AddCommand(notebooksListCmd)
	notebooksCmd.AddCommand(notebooksGetCmd)

	// Register with root
	rootCmd.AddCommand(notebooksCmd)
}
