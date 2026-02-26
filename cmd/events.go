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

// ---- events command group ----

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Query events from the Datadog event stream",
	Long:  `Query events from the Datadog event stream.`,
	Example: `  # List events from the last day
  datadog-cli events list --start 1d

  # Filter events by tag in production
  datadog-cli events list --tags "env:production"

  # Get details for a specific event
  datadog-cli events get 12345`,
}

// ---- events list ----

var (
	eventsListStart    string
	eventsListEnd      string
	eventsListPriority string
	eventsListSources  string
	eventsListTags     string
)

var eventsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List events from the event stream",
	Long: `List events from the Datadog event stream within a time range.

Uses GET /api/v1/events. Time values are Unix seconds.`,
	Example: `  # List events from the last day
  datadog-cli events list --start 1d

  # List normal priority events from CI sources
  datadog-cli events list --priority normal --sources "jenkins,github"

  # List events tagged for production as JSON
  datadog-cli events list --tags "env:production" --json`,
	RunE: runEventsList,
}

func runEventsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	startSecs, err := parseTimeSeconds(eventsListStart)
	if err != nil {
		return fmt.Errorf("--start: %w", err)
	}

	endSecs, err := parseTimeSeconds(eventsListEnd)
	if err != nil {
		return fmt.Errorf("--end: %w", err)
	}

	params := url.Values{}
	params.Set("start", fmt.Sprintf("%d", startSecs))
	params.Set("end", fmt.Sprintf("%d", endSecs))

	if eventsListPriority != "" {
		params.Set("priority", eventsListPriority)
	}
	if eventsListSources != "" {
		params.Set("sources", eventsListSources)
	}
	if eventsListTags != "" {
		params.Set("tags", eventsListTags)
	}

	respBytes, err := client.Get("/api/v1/events", params)
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

	eventsRaw, _ := raw["events"].([]interface{})
	if len(eventsRaw) == 0 {
		fmt.Fprintln(os.Stdout, "No events found.")
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(eventsRaw) > flagLimit {
		eventsRaw = eventsRaw[:flagLimit]
	}

	type eventRow struct {
		ID       string
		Title    string
		Source   string
		Priority string
		Date     string
	}

	rows := make([]eventRow, 0, len(eventsRaw))
	tableRows := make([][]string, 0, len(eventsRaw))

	for _, item := range eventsRaw {
		e, _ := item.(map[string]interface{})
		id := formatID(e["id"])
		title := output.TruncateString(eventsStringField(e, "title"), 45)
		src := eventsStringField(e, "source_type_name")
		priority := eventsStringField(e, "priority")
		date := eventsFormatUnixTimestamp(e["date_happened"])

		rows = append(rows, eventRow{
			ID:       id,
			Title:    title,
			Source:   src,
			Priority: priority,
			Date:     date,
		})
		tableRows = append(tableRows, []string{id, title, src, priority, date})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 14},
		{Name: "Title", Width: 45},
		{Name: "Source", Width: 20},
		{Name: "Priority", Width: 10},
		{Name: "Date", Width: 18},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- events get ----

var eventsGetCmd = &cobra.Command{
	Use:   "get <event_id>",
	Short: "Get event details by ID",
	Long: `Get detailed information about a specific event.

Uses GET /api/v1/events/{id}.`,
	Example: `  # Get details for a specific event
  datadog-cli events get 12345

  # Get event details in JSON format
  datadog-cli events get 12345 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runEventsGet,
}

func runEventsGet(cmd *cobra.Command, args []string) error {
	eventID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/events/"+eventID, nil)
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

	// The v1 events get response wraps the event in an "event" key
	eventData, ok := raw["event"].(map[string]interface{})
	if !ok {
		eventData = raw
	}

	id := formatID(eventData["id"])
	title := eventsStringField(eventData, "title")
	text := output.TruncateString(eventsStringField(eventData, "text"), 200)
	src := eventsStringField(eventData, "source_type_name")
	alertType := eventsStringField(eventData, "alert_type")
	priority := eventsStringField(eventData, "priority")
	host := eventsStringField(eventData, "host")
	device := eventsStringField(eventData, "device_name")
	date := eventsFormatUnixTimestamp(eventData["date_happened"])
	rawURL := eventsStringField(eventData, "url")

	tags := ""
	if tagsRaw, ok := eventData["tags"].([]interface{}); ok {
		tagStrs := make([]string, 0, len(tagsRaw))
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tagStrs = append(tagStrs, s)
			}
		}
		tags = strings.Join(tagStrs, ", ")
	}

	fmt.Fprintf(os.Stdout, "Event: %s\n\n", title)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Title", title},
		{"Text", text},
		{"Date", date},
		{"Source", src},
		{"Alert Type", alertType},
		{"Priority", priority},
		{"Host", host},
		{"Device", device},
		{"Tags", tags},
		{"URL", rawURL},
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
		{Name: "Field", Width: 12},
		{Name: "Value", Width: 80},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

// eventsStringField safely extracts a string value from a map.
func eventsStringField(m map[string]interface{}, key string) string {
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

// eventsFormatUnixTimestamp formats a Unix timestamp (seconds, as float64 from JSON) for display.
func eventsFormatUnixTimestamp(ts interface{}) string {
	if ts == nil {
		return ""
	}
	switch v := ts.(type) {
	case float64:
		t := time.Unix(int64(v), 0)
		return t.UTC().Format("2006-01-02 15:04")
	case int64:
		t := time.Unix(v, 0)
		return t.UTC().Format("2006-01-02 15:04")
	case string:
		if v == "" {
			return ""
		}
		return v
	}
	return fmt.Sprintf("%v", ts)
}

// ---- init ----

func init() {
	// events list flags
	eventsListCmd.Flags().StringVar(&eventsListStart, "start", "1d", "Start time (e.g. '1d', '2h', ISO-8601, or Unix seconds)")
	eventsListCmd.Flags().StringVar(&eventsListEnd, "end", "now", "End time (e.g. 'now', ISO-8601, or Unix seconds)")
	eventsListCmd.Flags().StringVar(&eventsListPriority, "priority", "", "Filter by priority (low, normal, all)")
	eventsListCmd.Flags().StringVar(&eventsListSources, "sources", "", "Filter by source names (comma-separated)")
	eventsListCmd.Flags().StringVar(&eventsListTags, "tags", "", "Filter by tags (comma-separated, e.g. 'env:production')")

	// Wire up subcommands
	eventsCmd.AddCommand(eventsListCmd)
	eventsCmd.AddCommand(eventsGetCmd)

	// Register with root
	rootCmd.AddCommand(eventsCmd)
}
