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

// ---- dashboards command group ----

var dashboardsCmd = &cobra.Command{
	Use:   "dashboards",
	Short: "Query dashboards from Datadog",
	Long:  `Query dashboards from Datadog.`,
	Example: `  # List all dashboards
  datadog-cli dashboards list

  # Get a specific dashboard by ID
  datadog-cli dashboards get abc-123-def

  # Search dashboards by title keyword
  datadog-cli dashboards search -q "system"`,
}

// ---- dashboards list ----

var dashboardsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List dashboards",
	Long: `List all dashboards from Datadog.

Uses GET /api/v1/dashboard.`,
	Example: `  # List all dashboards
  datadog-cli dashboards list

  # List dashboards in JSON format
  datadog-cli dashboards list --json`,
	RunE: runDashboardsList,
}

func runDashboardsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/dashboard", nil)
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

	dashboardsRaw, _ := raw["dashboards"].([]interface{})
	if len(dashboardsRaw) == 0 {
		fmt.Fprintln(os.Stdout, "No dashboards found.")
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(dashboardsRaw) > flagLimit {
		dashboardsRaw = dashboardsRaw[:flagLimit]
	}

	type dashboardRow struct {
		ID      string
		Title   string
		Author  string
		URL     string
		Created string
	}

	rows := make([]dashboardRow, 0, len(dashboardsRaw))
	tableRows := make([][]string, 0, len(dashboardsRaw))

	for _, item := range dashboardsRaw {
		d, _ := item.(map[string]interface{})
		id := dashboardStringField(d, "id")
		title := output.TruncateString(dashboardStringField(d, "title"), 45)
		author := dashboardStringField(d, "author_handle")
		author = truncateEmail(author, 25)
		rawURL := dashboardStringField(d, "url")
		created := dashboardsFormatTimestamp(d["created_at"])

		rows = append(rows, dashboardRow{
			ID:      id,
			Title:   title,
			Author:  author,
			URL:     rawURL,
			Created: created,
		})
		tableRows = append(tableRows, []string{id, title, author, rawURL, created})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 25},
		{Name: "Title", Width: 45},
		{Name: "Author", Width: 25},
		{Name: "URL", Width: 40},
		{Name: "Created", Width: 18},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- dashboards get ----

var dashboardsGetCmd = &cobra.Command{
	Use:   "get <dashboard_id>",
	Short: "Get dashboard details by ID",
	Long: `Get detailed information about a specific dashboard.

Uses GET /api/v1/dashboard/{id}.`,
	Example: `  # Get details for a dashboard
  datadog-cli dashboards get abc-123-def

  # Get dashboard details in JSON format
  datadog-cli dashboards get abc-123-def --json`,
	Args: cobra.ExactArgs(1),
	RunE: runDashboardsGet,
}

func runDashboardsGet(cmd *cobra.Command, args []string) error {
	dashboardID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/dashboard/"+dashboardID, nil)
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

	id := dashboardStringField(raw, "id")
	title := dashboardStringField(raw, "title")
	description := dashboardStringField(raw, "description")
	author := dashboardStringField(raw, "author_handle")
	layoutType := dashboardStringField(raw, "layout_type")
	rawURL := dashboardStringField(raw, "url")
	created := dashboardsFormatTimestamp(raw["created_at"])
	modified := dashboardsFormatTimestamp(raw["modified_at"])

	widgetCount := 0
	if widgets, ok := raw["widgets"].([]interface{}); ok {
		widgetCount = len(widgets)
	}

	templateVarCount := 0
	if tvars, ok := raw["template_variables"].([]interface{}); ok {
		templateVarCount = len(tvars)
	}

	isReadOnly := "No"
	if v, ok := raw["is_read_only"].(bool); ok && v {
		isReadOnly = "Yes"
	}

	fmt.Fprintf(os.Stdout, "Dashboard: %s\n\n", title)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Title", title},
		{"Description", description},
		{"Author", author},
		{"Layout Type", layoutType},
		{"URL", rawURL},
		{"Created", created},
		{"Modified", modified},
		{"Widgets", fmt.Sprintf("%d", widgetCount)},
		{"Template Variables", fmt.Sprintf("%d", templateVarCount)},
		{"Read Only", isReadOnly},
	}

	rows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" {
			continue
		}
		rows = append(rows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 22},
		{Name: "Value", Width: 80},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- dashboards search ----

var dashboardsSearchQuery string

var dashboardsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search dashboards by title",
	Long: `Search dashboards by title (client-side filter).

Uses GET /api/v1/dashboard and filters results locally by title substring.`,
	Example: `  # Search dashboards by title keyword
  datadog-cli dashboards search --query "system"

  # Search for performance dashboards
  datadog-cli dashboards search -q "performance"

  # Search for monitoring dashboards and output as JSON
  datadog-cli dashboards search --query "monitoring" --json`,
	RunE: runDashboardsSearch,
}

func runDashboardsSearch(cmd *cobra.Command, args []string) error {
	if dashboardsSearchQuery == "" {
		return fmt.Errorf("--query / -q is required")
	}

	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}

	respBytes, err := client.Get("/api/v1/dashboard", params)
	if err != nil {
		return err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	dashboardsRaw, _ := raw["dashboards"].([]interface{})

	// Client-side filter by title substring
	queryLower := strings.ToLower(dashboardsSearchQuery)
	filtered := make([]interface{}, 0)
	for _, item := range dashboardsRaw {
		d, _ := item.(map[string]interface{})
		titleLower := strings.ToLower(dashboardStringField(d, "title"))
		descLower := strings.ToLower(dashboardStringField(d, "description"))
		if strings.Contains(titleLower, queryLower) || strings.Contains(descLower, queryLower) {
			filtered = append(filtered, item)
		}
	}

	if opts.JSON {
		return output.RenderJSON(map[string]interface{}{"dashboards": filtered}, opts)
	}

	if len(filtered) == 0 {
		fmt.Fprintf(os.Stdout, "No dashboards found matching %q.\n", dashboardsSearchQuery)
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(filtered) > flagLimit {
		filtered = filtered[:flagLimit]
	}

	type dashboardRow struct {
		ID     string
		Title  string
		Author string
	}

	rows := make([]dashboardRow, 0, len(filtered))
	tableRows := make([][]string, 0, len(filtered))

	for _, item := range filtered {
		d, _ := item.(map[string]interface{})
		id := dashboardStringField(d, "id")
		title := output.TruncateString(dashboardStringField(d, "title"), 50)
		author := truncateEmail(dashboardStringField(d, "author_handle"), 25)

		rows = append(rows, dashboardRow{
			ID:     id,
			Title:  title,
			Author: author,
		})
		tableRows = append(tableRows, []string{id, title, author})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 25},
		{Name: "Title", Width: 50},
		{Name: "Author", Width: 25},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

// dashboardStringField safely extracts a string value from a map.
func dashboardStringField(m map[string]interface{}, key string) string {
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

// dashboardsFormatTimestamp formats an ISO timestamp string for dashboards.
func dashboardsFormatTimestamp(ts interface{}) string {
	if ts == nil {
		return ""
	}
	s, ok := ts.(string)
	if !ok {
		return fmt.Sprintf("%v", ts)
	}
	if s == "" {
		return ""
	}
	// Normalize trailing Z
	normalized := s
	if strings.HasSuffix(normalized, "Z") {
		normalized = normalized[:len(normalized)-1] + "+00:00"
	}
	t, err := time.Parse(time.RFC3339, normalized)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return s
		}
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// truncateEmail shortens an email address for display.
func truncateEmail(email string, maxLen int) string {
	if len(email) <= maxLen {
		return email
	}
	if idx := strings.Index(email, "@"); idx > 0 {
		local := email[:idx]
		domain := email[idx:]
		if len(local) > maxLen-len(domain)-3 {
			available := maxLen - len(domain) - 3
			if available > 0 {
				return local[:available] + "..." + domain
			}
		}
	}
	return output.TruncateString(email, maxLen)
}

// ---- init ----

func init() {
	// dashboards search flags
	dashboardsSearchCmd.Flags().StringVarP(&dashboardsSearchQuery, "query", "q", "", "Search filter (required)")

	// Wire up subcommands
	dashboardsCmd.AddCommand(dashboardsListCmd)
	dashboardsCmd.AddCommand(dashboardsGetCmd)
	dashboardsCmd.AddCommand(dashboardsSearchCmd)

	// Register with root
	rootCmd.AddCommand(dashboardsCmd)
}
