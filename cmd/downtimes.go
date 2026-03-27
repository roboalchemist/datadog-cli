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

// maxDowntimesPageSize is the maximum page[limit] accepted by the Datadog v2 downtimes API.
const maxDowntimesPageSize = 100

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

var downtimesListAll bool

var downtimesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List downtimes",
	Long: `List all downtimes (maintenance windows) from Datadog.

Uses GET /api/v2/downtime with offset-based pagination (page[offset] / page[limit]).
The API returns up to 100 items per page. Use --all to transparently fetch all pages,
or --limit N to cap results (default 100).`,
	Example: `  # List downtimes (up to --limit, default 100)
  datadog-cli downtimes list

  # Fetch every downtime regardless of count
  datadog-cli downtimes list --all

  # List downtimes in JSON format
  datadog-cli downtimes list --json`,
	RunE: runDowntimesList,
}

func runDowntimesList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if downtimesListAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// Page size: min(effectiveLimit, maxDowntimesPageSize).
	// If unlimited (--all), use max page size.
	pageSize := effectiveLimit
	if pageSize == 0 || pageSize > maxDowntimesPageSize {
		pageSize = maxDowntimesPageSize
	}

	type downtimeRow struct {
		ID      string
		Scope   string
		Message string
		Start   string
		End     string
		Active  string
	}

	var rows []downtimeRow
	var allData []interface{}
	var lastRaw map[string]interface{}
	offset := 0
	pageNum := 0

	for {
		pageNum++

		// Print progress to stderr on subsequent pages (unless --quiet).
		if pageNum > 1 && !flagQuiet {
			if downtimesListAll {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d downtimes so far)...\n", pageNum, len(rows))
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
			}
		}

		// Adjust page size for the last page when a limit is set.
		thisPageSize := pageSize
		if !downtimesListAll && effectiveLimit > 0 {
			remaining := effectiveLimit - len(rows)
			if remaining < thisPageSize {
				thisPageSize = remaining
			}
		}

		params := url.Values{}
		params.Set("page[limit]", fmt.Sprintf("%d", thisPageSize))
		params.Set("page[offset]", fmt.Sprintf("%d", offset))

		respBytes, err := client.Get("/api/v2/downtime", params)
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
			if !downtimesListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)

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
		}

		// Stop if we have enough results.
		if !downtimesListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
			break
		}

		// Stop if this page was smaller than the page size (last page).
		if len(data) < thisPageSize {
			break
		}

		offset += len(data)
	}

	if opts.JSON {
		// Build a merged response combining all pages' data, keeping metadata from last response.
		merged := map[string]interface{}{
			"data": allData,
		}
		if lastRaw != nil {
			if metaVal, ok := lastRaw["meta"]; ok {
				merged["meta"] = metaVal
			}
		}
		return output.RenderJSON(merged, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No downtimes found.")
		return nil
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{r.ID, r.Scope, r.Message, r.Start, r.End, r.Active})
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
			return "monitor:" + formatID(monID)
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
	// downtimes list flags
	downtimesListCmd.Flags().BoolVar(&downtimesListAll, "all", false, "Fetch all pages until no results remain (overrides --limit)")

	// Wire up subcommands
	downtimesCmd.AddCommand(downtimesListCmd)
	downtimesCmd.AddCommand(downtimesGetCmd)

	// Register with root
	rootCmd.AddCommand(downtimesCmd)
}
