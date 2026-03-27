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

// ---- monitors command group ----

var monitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "Query monitors from Datadog",
	Long:  `Query monitors from Datadog.`,
	Example: `  # List all monitors
  datadog-cli monitors list

  # Get details for a specific monitor
  datadog-cli monitors get 12345

  # Search monitors by keyword
  datadog-cli monitors search -q "cpu"`,
}

// maxMonitorsPageSize is the maximum page_size accepted by the Datadog monitors list API.
const maxMonitorsPageSize = 100

// ---- monitors list ----

var (
	monitorsListGroupStates string
	monitorsListName        string
	monitorsListTags        string
	monitorsListAll         bool
)

var monitorsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List monitors",
	Long: `List monitors from Datadog.

Uses GET /api/v1/monitor with automatic pagination.
The API page_size cap is 100; use --all to fetch every monitor.`,
	Example: `  # List all monitors
  datadog-cli monitors list

  # Fetch every monitor regardless of count
  datadog-cli monitors list --all

  # Filter monitors by tag
  datadog-cli monitors list --tags "env:production"

  # List only alerting and warning monitors
  datadog-cli monitors list --group-states "alert,warn" --json`,
	RunE: runMonitorsList,
}

func runMonitorsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if monitorsListAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// Page size is min(effectiveLimit, maxMonitorsPageSize); if unlimited use max.
	pageSize := effectiveLimit
	if pageSize == 0 || pageSize > maxMonitorsPageSize {
		pageSize = maxMonitorsPageSize
	}

	type monitorRow struct {
		ID      string
		Name    string
		Type    string
		Status  string
		Creator string
	}

	var rows []monitorRow
	var tableRows [][]string
	var allData []interface{}
	pageNum := 0 // Datadog monitors list uses 0-indexed page numbers

	for {
		params := url.Values{}
		params.Set("page_size", fmt.Sprintf("%d", pageSize))
		params.Set("page", fmt.Sprintf("%d", pageNum))

		if monitorsListGroupStates != "" {
			params.Set("group_states", monitorsListGroupStates)
		}
		if monitorsListName != "" {
			params.Set("name", monitorsListName)
		}
		if monitorsListTags != "" {
			params.Set("monitor_tags", monitorsListTags)
		}

		if pageNum > 0 && !flagQuiet {
			if monitorsListAll {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d monitors so far)...\n", pageNum+1, len(rows))
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum+1, len(rows), effectiveLimit)
			}
		}

		respBytes, err := client.Get("/api/v1/monitor", params)
		if err != nil {
			return err
		}

		// The monitors list API returns a JSON array directly
		var page []interface{}
		if err := json.Unmarshal(respBytes, &page); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		for _, item := range page {
			if !monitorsListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)
			m, _ := item.(map[string]interface{})
			id := formatID(m["id"])
			name := output.TruncateString(monitorStringField(m, "name"), 45)
			mtype := monitorStringField(m, "type")
			status := monitorStringField(m, "overall_state")
			creator := ""
			if creatorMap, ok := m["creator"].(map[string]interface{}); ok {
				creator = monitorStringField(creatorMap, "email")
				if creator == "" {
					creator = monitorStringField(creatorMap, "handle")
				}
			}
			rows = append(rows, monitorRow{
				ID:      id,
				Name:    name,
				Type:    mtype,
				Status:  status,
				Creator: creator,
			})
			tableRows = append(tableRows, []string{id, name, mtype, output.ColorStatus(status), creator})
		}

		// Stop if we have enough, the page was short (last page), or empty.
		if (!monitorsListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit) ||
			len(page) < pageSize || len(page) == 0 {
			break
		}

		pageNum++
	}

	if opts.JSON {
		return output.RenderJSON(allData, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No monitors found.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 12},
		{Name: "Name", Width: 45},
		{Name: "Type", Width: 20},
		{Name: "Status", Width: 12},
		{Name: "Creator", Width: 35},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- monitors get ----

var monitorsGetCmd = &cobra.Command{
	Use:   "get <monitor_id>",
	Short: "Get monitor details by ID",
	Long: `Get detailed information about a specific monitor.

Uses GET /api/v1/monitor/{id}.`,
	Example: `  # Get details for a monitor
  datadog-cli monitors get 12345

  # Get monitor details in JSON format
  datadog-cli monitors get 12345 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runMonitorsGet,
}

func runMonitorsGet(cmd *cobra.Command, args []string) error {
	monitorID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/monitor/"+monitorID, nil)
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

	// Extract fields for display
	id := formatID(raw["id"])
	name := monitorStringField(raw, "name")
	mtype := monitorStringField(raw, "type")
	status := monitorStringField(raw, "overall_state")
	query := monitorStringField(raw, "query")
	message := output.TruncateString(monitorStringField(raw, "message"), 200)
	priority := fmt.Sprintf("%v", raw["priority"])
	if priority == "<nil>" || priority == "0" {
		priority = ""
	}

	tags := ""
	if tagsRaw, ok := raw["tags"].([]interface{}); ok {
		tagStrs := make([]string, 0, len(tagsRaw))
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tagStrs = append(tagStrs, s)
			}
		}
		tags = strings.Join(tagStrs, ", ")
	}

	created := monitorsFormatTimestamp(raw["created"])
	modified := monitorsFormatTimestamp(raw["modified"])

	creator := ""
	if creatorMap, ok := raw["creator"].(map[string]interface{}); ok {
		creator = monitorStringField(creatorMap, "email")
		if creator == "" {
			creator = monitorStringField(creatorMap, "handle")
		}
	}

	// Thresholds
	criticalThreshold := ""
	warningThreshold := ""
	notifyNoData := ""
	if options, ok := raw["options"].(map[string]interface{}); ok {
		if thresholds, ok := options["thresholds"].(map[string]interface{}); ok {
			if v, ok := thresholds["critical"]; ok && v != nil {
				criticalThreshold = fmt.Sprintf("%v", v)
			}
			if v, ok := thresholds["warning"]; ok && v != nil {
				warningThreshold = fmt.Sprintf("%v", v)
			}
		}
		if v, ok := options["notify_no_data"].(bool); ok {
			if v {
				notifyNoData = "true"
			} else {
				notifyNoData = "false"
			}
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "Monitor: %s\n\n", name)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Name", name},
		{"Type", mtype},
		{"Status", output.ColorStatus(status)},
		{"Query", query},
		{"Message", message},
		{"Priority", priority},
		{"Tags", tags},
		{"Creator", creator},
		{"Created", created},
		{"Modified", modified},
		{"Critical Threshold", criticalThreshold},
		{"Warning Threshold", warningThreshold},
		{"Notify No Data", notifyNoData},
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

// maxMonitorsSearchPageSize is the maximum per_page accepted by the monitors search API.
const maxMonitorsSearchPageSize = 100

// ---- monitors search ----

var (
	monitorsSearchQuery string
	monitorsSearchAll   bool
)

var monitorsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search monitors by query",
	Long: `Search monitors by name, tags, or query string.

Uses GET /api/v1/monitor/search with automatic pagination.
The API per_page cap is 100; use --all to fetch every matching monitor.`,
	Example: `  # Search for CPU-related monitors
  datadog-cli monitors search --query "cpu"

  # Fetch all matching monitors regardless of count
  datadog-cli monitors search -q "env:production" --all

  # Search for disk monitors and output as JSON
  datadog-cli monitors search --query "disk" --json`,
	RunE: runMonitorsSearch,
}

func runMonitorsSearch(cmd *cobra.Command, args []string) error {
	if monitorsSearchQuery == "" {
		return fmt.Errorf("--query / -q is required")
	}

	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if monitorsSearchAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// Page size is min(effectiveLimit, maxMonitorsSearchPageSize); if unlimited use max.
	perPage := effectiveLimit
	if perPage == 0 || perPage > maxMonitorsSearchPageSize {
		perPage = maxMonitorsSearchPageSize
	}

	type monitorRow struct {
		ID     string
		Name   string
		Type   string
		Status string
	}

	var rows []monitorRow
	var tableRows [][]string
	var allMonitors []interface{}
	var lastRaw map[string]interface{}
	pageNum := 0 // 0-indexed

	for {
		params := url.Values{}
		params.Set("query", monitorsSearchQuery)
		params.Set("per_page", fmt.Sprintf("%d", perPage))
		params.Set("page", fmt.Sprintf("%d", pageNum))

		if pageNum > 0 && !flagQuiet {
			if monitorsSearchAll {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d monitors so far)...\n", pageNum+1, len(rows))
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum+1, len(rows), effectiveLimit)
			}
		}

		respBytes, err := client.Get("/api/v1/monitor/search", params)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		// Response shape: {"monitors": [...], "metadata": {...}}
		monitorsRaw, _ := raw["monitors"].([]interface{})

		for _, item := range monitorsRaw {
			if !monitorsSearchAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
				break
			}
			allMonitors = append(allMonitors, item)
			m, _ := item.(map[string]interface{})
			id := formatID(m["id"])
			name := output.TruncateString(monitorStringField(m, "name"), 45)
			mtype := monitorStringField(m, "type")
			status := monitorStringField(m, "status")
			if status == "" {
				status = monitorStringField(m, "overall_state")
			}
			rows = append(rows, monitorRow{
				ID:     id,
				Name:   name,
				Type:   mtype,
				Status: status,
			})
			tableRows = append(tableRows, []string{id, name, mtype, output.ColorStatus(status)})
		}

		// Stop if we have enough, the page was short (last page), or empty.
		if (!monitorsSearchAll && effectiveLimit > 0 && len(rows) >= effectiveLimit) ||
			len(monitorsRaw) < perPage || len(monitorsRaw) == 0 {
			break
		}

		pageNum++
	}

	if opts.JSON {
		// Return merged response with all monitors but metadata from the last page.
		merged := map[string]interface{}{
			"monitors": allMonitors,
		}
		if lastRaw != nil {
			if meta, ok := lastRaw["metadata"]; ok {
				merged["metadata"] = meta
			}
		}
		return output.RenderJSON(merged, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "No monitors found matching %q.\n", monitorsSearchQuery)
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 12},
		{Name: "Name", Width: 45},
		{Name: "Type", Width: 20},
		{Name: "Status", Width: 12},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

// monitorStringField safely extracts a string value from a map.
func monitorStringField(m map[string]interface{}, key string) string {
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

// monitorsFormatTimestamp formats an ISO timestamp string.
func monitorsFormatTimestamp(ts interface{}) string {
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
	// Handle trailing Z
	if strings.HasSuffix(s, "Z") {
		s = s[:len(s)-1] + "+00:00"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try without timezone
		t, err = time.Parse("2006-01-02T15:04:05", s)
		if err != nil {
			return s
		}
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// ---- init ----

func init() {
	// monitors list flags
	monitorsListCmd.Flags().StringVar(&monitorsListGroupStates, "group-states", "", "Filter by group states (comma-separated, e.g. 'alert,warn')")
	monitorsListCmd.Flags().StringVar(&monitorsListName, "name", "", "Filter by monitor name (substring match)")
	monitorsListCmd.Flags().StringVar(&monitorsListTags, "tags", "", "Filter by tags (comma-separated, e.g. 'env:prod,team:backend')")
	monitorsListCmd.Flags().BoolVar(&monitorsListAll, "all", false, "Fetch all pages until no more monitors remain (overrides --limit)")

	// monitors search flags
	monitorsSearchCmd.Flags().StringVarP(&monitorsSearchQuery, "query", "q", "", "Search query (required)")
	monitorsSearchCmd.Flags().BoolVar(&monitorsSearchAll, "all", false, "Fetch all pages until no more monitors remain (overrides --limit)")

	// Wire up subcommands
	monitorsCmd.AddCommand(monitorsListCmd)
	monitorsCmd.AddCommand(monitorsGetCmd)
	monitorsCmd.AddCommand(monitorsSearchCmd)

	// Register with root
	rootCmd.AddCommand(monitorsCmd)
}
