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

// ---- monitors command group ----

var monitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "Query monitors from Datadog",
	Long: `Query monitors from Datadog.

Subcommands:
  list    List monitors
  get     Get monitor details by ID
  search  Search monitors by query`,
}

// ---- monitors list ----

var (
	monitorsListGroupStates string
	monitorsListName        string
	monitorsListTags        string
)

var monitorsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List monitors",
	Long: `List monitors from Datadog.

Uses GET /api/v1/monitor.

Examples:
  datadog-cli monitors list
  datadog-cli monitors list --tags "env:production"
  datadog-cli monitors list --name "CPU"
  datadog-cli monitors list --group-states "alert,warn"
  datadog-cli monitors list --json`,
	RunE: runMonitorsList,
}

func runMonitorsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("page_size", fmt.Sprintf("%d", flagLimit))

	if monitorsListGroupStates != "" {
		params.Set("group_states", monitorsListGroupStates)
	}
	if monitorsListName != "" {
		params.Set("name", monitorsListName)
	}
	if monitorsListTags != "" {
		params.Set("monitor_tags", monitorsListTags)
	}

	respBytes, err := client.Get("/api/v1/monitor", params)
	if err != nil {
		return err
	}

	// The monitors list API returns a JSON array directly
	var raw []interface{}
	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if opts.JSON {
		return output.RenderJSON(raw, opts)
	}

	if len(raw) == 0 {
		fmt.Fprintln(os.Stdout, "No monitors found.")
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(raw) > flagLimit {
		raw = raw[:flagLimit]
	}

	type monitorRow struct {
		ID      string
		Name    string
		Type    string
		Status  string
		Creator string
	}

	rows := make([]monitorRow, 0, len(raw))
	tableRows := make([][]string, 0, len(raw))

	for _, item := range raw {
		m, _ := item.(map[string]interface{})
		id := fmt.Sprintf("%v", m["id"])
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

Uses GET /api/v1/monitor/{id}.

Examples:
  datadog-cli monitors get 12345
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
	id := fmt.Sprintf("%v", raw["id"])
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

	fmt.Fprintf(os.Stdout, "Monitor: %s\n\n", name)

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

// ---- monitors search ----

var monitorsSearchQuery string

var monitorsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search monitors by query",
	Long: `Search monitors by name, tags, or query string.

Uses GET /api/v1/monitor/search.

Examples:
  datadog-cli monitors search --query "cpu"
  datadog-cli monitors search -q "env:production"
  datadog-cli monitors search --query "disk" --json`,
	RunE: runMonitorsSearch,
}

func runMonitorsSearch(cmd *cobra.Command, args []string) error {
	if monitorsSearchQuery == "" {
		return fmt.Errorf("--query / -q is required")
	}

	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("query", monitorsSearchQuery)
	params.Set("per_page", fmt.Sprintf("%d", flagLimit))

	respBytes, err := client.Get("/api/v1/monitor/search", params)
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

	// Response shape: {"monitors": [...], "metadata": {...}}
	monitorsRaw, _ := raw["monitors"].([]interface{})

	if len(monitorsRaw) == 0 {
		fmt.Fprintf(os.Stdout, "No monitors found matching %q.\n", monitorsSearchQuery)
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(monitorsRaw) > flagLimit {
		monitorsRaw = monitorsRaw[:flagLimit]
	}

	type monitorRow struct {
		ID     string
		Name   string
		Type   string
		Status string
	}

	rows := make([]monitorRow, 0, len(monitorsRaw))
	tableRows := make([][]string, 0, len(monitorsRaw))

	for _, item := range monitorsRaw {
		m, _ := item.(map[string]interface{})
		id := fmt.Sprintf("%v", m["id"])
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

	// monitors search flags
	monitorsSearchCmd.Flags().StringVarP(&monitorsSearchQuery, "query", "q", "", "Search query (required)")

	// Wire up subcommands
	monitorsCmd.AddCommand(monitorsListCmd)
	monitorsCmd.AddCommand(monitorsGetCmd)
	monitorsCmd.AddCommand(monitorsSearchCmd)

	// Register with root
	rootCmd.AddCommand(monitorsCmd)
}
