package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- downtimes command group ----

var downtimesCmd = &cobra.Command{
	Use:   "downtimes",
	Short: "Query downtimes (maintenance windows) from Datadog",
	Long:  `Query downtimes (maintenance windows) from Datadog.`,
	Example: `  # List all downtimes
  datadog-cli downtimes list

  # Get details for a specific downtime
  datadog-cli downtimes get abc123

  # List downtimes in JSON format
  datadog-cli downtimes list --json`,
}

// ---- downtimes list ----

var downtimesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List downtimes",
	Long: `List all downtimes (maintenance windows) from Datadog.

Uses GET /api/v2/downtime.`,
	Example: `  # List all active and scheduled downtimes
  datadog-cli downtimes list

  # List downtimes in JSON format
  datadog-cli downtimes list --json`,
	RunE: runDowntimesList,
}

func runDowntimesList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/downtime", nil)
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

	downtimesRaw, _ := raw["data"].([]interface{})
	if len(downtimesRaw) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No downtimes found.")
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(downtimesRaw) > flagLimit {
		downtimesRaw = downtimesRaw[:flagLimit]
	}

	type downtimeRow struct {
		ID      string
		Scope   string
		Message string
		Start   string
		End     string
		Active  string
	}

	rows := make([]downtimeRow, 0, len(downtimesRaw))
	tableRows := make([][]string, 0, len(downtimesRaw))

	for _, item := range downtimesRaw {
		dt, _ := item.(map[string]interface{})
		attrs, _ := dt["attributes"].(map[string]interface{})

		id := formatID(dt["id"])
		scope := downtimesScope(attrs)
		message := output.TruncateString(downtimesStringField(attrs, "message"), 35)
		schedule, _ := attrs["schedule"].(map[string]interface{})
		start := downtimesFormatTimestamp(schedule["start"])
		end := downtimesFormatTimestamp(schedule["end"])
		active := downtimesActive(attrs)

		rows = append(rows, downtimeRow{
			ID:      id,
			Scope:   scope,
			Message: message,
			Start:   start,
			End:     end,
			Active:  active,
		})
		tableRows = append(tableRows, []string{id, scope, message, start, end, active})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 38},
		{Name: "Scope", Width: 25},
		{Name: "Message", Width: 35},
		{Name: "Start", Width: 18},
		{Name: "End", Width: 18},
		{Name: "Active", Width: 10},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- downtimes get ----

var downtimesGetCmd = &cobra.Command{
	Use:   "get <downtime_id>",
	Short: "Get downtime details by ID",
	Long: `Get detailed information about a specific downtime.

Uses GET /api/v2/downtime/{id}.`,
	Example: `  # Get details for a specific downtime
  datadog-cli downtimes get abc123

  # Get downtime details in JSON format
  datadog-cli downtimes get abc123 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runDowntimesGet,
}

func runDowntimesGet(cmd *cobra.Command, args []string) error {
	downtimeID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/downtime/"+downtimeID, nil)
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

	// v2 API wraps response in "data"
	dt, ok := raw["data"].(map[string]interface{})
	if !ok {
		dt = raw
	}
	attrs, _ := dt["attributes"].(map[string]interface{})

	id := formatID(dt["id"])
	scope := downtimesScope(attrs)
	message := downtimesStringField(attrs, "message")
	schedule, _ := attrs["schedule"].(map[string]interface{})
	start := downtimesFormatTimestamp(schedule["start"])
	end := downtimesFormatTimestamp(schedule["end"])
	active := downtimesActive(attrs)
	timezone := downtimesStringField(attrs, "timezone")
	notifyEndStates := downtimesNotifyEndStates(attrs)
	notifyEndTypes := downtimesNotifyEndTypes(attrs)

	// Creator
	creatorHandle := ""
	if creatorRel, ok := dt["relationships"].(map[string]interface{}); ok {
		if createdBy, ok := creatorRel["created_by"].(map[string]interface{}); ok {
			if createdByData, ok := createdBy["data"].(map[string]interface{}); ok {
				creatorHandle = formatID(createdByData["id"])
			}
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "Downtime: %s\n\n", scope)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Scope", scope},
		{"Message", message},
		{"Start", start},
		{"End", end},
		{"Active", active},
		{"Timezone", timezone},
		{"Notify End States", notifyEndStates},
		{"Notify End Types", notifyEndTypes},
		{"Creator", creatorHandle},
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

// ---- helpers ----

// downtimesStringField safely extracts a string value from a map.
func downtimesStringField(m map[string]interface{}, key string) string {
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

// downtimesScope extracts the scope from downtime attributes (v2 uses monitor_identifier).
func downtimesScope(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	// v2 API uses monitor_identifier for scope
	if mi, ok := attrs["monitor_identifier"].(map[string]interface{}); ok {
		// monitor_tags or monitor_id
		if tags, ok := mi["monitor_tags"].([]interface{}); ok && len(tags) > 0 {
			parts := make([]string, 0, len(tags))
			for _, t := range tags {
				parts = append(parts, fmt.Sprintf("%v", t))
			}
			return strings.Join(parts, ", ")
		}
		if monID, ok := mi["monitor_id"]; ok && monID != nil {
			return fmt.Sprintf("monitor:%v", monID)
		}
	}
	// Fallback: try scope array (v1-style)
	if scopeRaw, ok := attrs["scope"].([]interface{}); ok && len(scopeRaw) > 0 {
		parts := make([]string, 0, len(scopeRaw))
		for _, s := range scopeRaw {
			parts = append(parts, fmt.Sprintf("%v", s))
		}
		return strings.Join(parts, ", ")
	}
	return "All"
}

// downtimesActive returns a human-readable active status from attributes.
func downtimesActive(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	if v, ok := attrs["status"].(string); ok {
		switch strings.ToLower(v) {
		case "active":
			return "Active"
		case "scheduled":
			return "Scheduled"
		case "canceled":
			return "Canceled"
		case "ended":
			return "Ended"
		default:
			return v
		}
	}
	// Fallback boolean
	if v, ok := attrs["active"].(bool); ok {
		if v {
			return "Active"
		}
		return "Inactive"
	}
	return ""
}

// downtimesNotifyEndStates extracts notify_end_states as a comma-separated string.
func downtimesNotifyEndStates(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	raw, ok := attrs["notify_end_states"].([]interface{})
	if !ok || len(raw) == 0 {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, v := range raw {
		parts = append(parts, fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, ", ")
}

// downtimesNotifyEndTypes extracts notify_end_types as a comma-separated string.
func downtimesNotifyEndTypes(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	raw, ok := attrs["notify_end_types"].([]interface{})
	if !ok || len(raw) == 0 {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, v := range raw {
		parts = append(parts, fmt.Sprintf("%v", v))
	}
	return strings.Join(parts, ", ")
}

// downtimesFormatTimestamp formats an ISO 8601 timestamp for display.
func downtimesFormatTimestamp(ts interface{}) string {
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

// ---- init ----

func init() {
	// Wire up subcommands
	downtimesCmd.AddCommand(downtimesListCmd)
	downtimesCmd.AddCommand(downtimesGetCmd)

	// Register with root
	rootCmd.AddCommand(downtimesCmd)
}
