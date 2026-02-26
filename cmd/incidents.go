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

// ---- incidents command group ----

var incidentsCmd = &cobra.Command{
	Use:   "incidents",
	Short: "Query incidents from Datadog",
	Long:  `Query incidents from Datadog.`,
	Example: `  # List all incidents
  datadog-cli incidents list

  # Get details for a specific incident
  datadog-cli incidents get abc12345-1234-5678-abcd-1234567890ab

  # List incidents in JSON format
  datadog-cli incidents list --json`,
}

// ---- incidents list ----

var incidentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List incidents",
	Long: `List all incidents from Datadog.

Uses GET /api/v2/incidents.`,
	Example: `  # List all incidents
  datadog-cli incidents list

  # List incidents in JSON format
  datadog-cli incidents list --json`,
	RunE: runIncidentsList,
}

func runIncidentsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("page[size]", fmt.Sprintf("%d", min(flagLimit, 100)))

	respBytes, err := client.Get("/api/v2/incidents", params)
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

	incidentsRaw, _ := raw["data"].([]interface{})
	if len(incidentsRaw) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No incidents found.")
		return nil
	}

	// Apply limit
	if flagLimit > 0 && len(incidentsRaw) > flagLimit {
		incidentsRaw = incidentsRaw[:flagLimit]
	}

	type incidentRow struct {
		ID       string
		Title    string
		Severity string
		Status   string
		Created  string
	}

	rows := make([]incidentRow, 0, len(incidentsRaw))
	tableRows := make([][]string, 0, len(incidentsRaw))

	for _, item := range incidentsRaw {
		inc, _ := item.(map[string]interface{})
		attrs, _ := inc["attributes"].(map[string]interface{})

		id := incidentsPublicID(inc, attrs)
		title := output.TruncateString(incidentsStringField(attrs, "title"), 45)
		severity := incidentsSeverity(attrs)
		status := incidentsStatus(attrs)
		created := incidentsFormatTimestamp(attrs["created"])

		rows = append(rows, incidentRow{
			ID:       id,
			Title:    title,
			Severity: severity,
			Status:   status,
			Created:  created,
		})
		tableRows = append(tableRows, []string{id, title, severity, status, created})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 10},
		{Name: "Title", Width: 45},
		{Name: "Severity", Width: 10},
		{Name: "Status", Width: 12},
		{Name: "Created", Width: 18},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- incidents get ----

var incidentsGetCmd = &cobra.Command{
	Use:   "get <incident_id>",
	Short: "Get incident details by ID",
	Long: `Get detailed information about a specific incident.

Uses GET /api/v2/incidents/{id}.`,
	Example: `  # Get details for a specific incident
  datadog-cli incidents get abc12345-1234-5678-abcd-1234567890ab

  # Get incident details in JSON format
  datadog-cli incidents get abc12345-1234-5678-abcd-1234567890ab --json`,
	Args: cobra.ExactArgs(1),
	RunE: runIncidentsGet,
}

func runIncidentsGet(cmd *cobra.Command, args []string) error {
	incidentID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/incidents/"+incidentID, nil)
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
	inc, ok := raw["data"].(map[string]interface{})
	if !ok {
		inc = raw
	}
	attrs, _ := inc["attributes"].(map[string]interface{})

	id := incidentsPublicID(inc, attrs)
	uuid := incidentsStringField(inc, "id")
	title := incidentsStringField(attrs, "title")
	severity := incidentsSeverity(attrs)
	status := incidentsStatus(attrs)
	customerImpacted := "No"
	if v, ok := attrs["customer_impacted"].(bool); ok && v {
		customerImpacted = "Yes"
	}
	customerImpactScope := incidentsStringField(attrs, "customer_impact_scope")
	created := incidentsFormatTimestamp(attrs["created"])
	modified := incidentsFormatTimestamp(attrs["modified"])
	detected := incidentsFormatTimestamp(attrs["detected"])
	resolved := incidentsFormatTimestamp(attrs["resolved"])
	visibility := incidentsStringField(attrs, "visibility")

	timeToDetect := ""
	if v, ok := attrs["time_to_detect"].(float64); ok && v > 0 {
		timeToDetect = fmt.Sprintf("%.0fs", v)
	}
	timeToResolve := ""
	if v, ok := attrs["time_to_resolve"].(float64); ok && v > 0 {
		timeToResolve = fmt.Sprintf("%.0fs", v)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Incident: %s\n\n", title)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"UUID", uuid},
		{"Title", title},
		{"Severity", severity},
		{"Status", status},
		{"Customer Impacted", customerImpacted},
		{"Customer Impact Scope", customerImpactScope},
		{"Created", created},
		{"Modified", modified},
		{"Detected", detected},
		{"Resolved", resolved},
		{"Time to Detect", timeToDetect},
		{"Time to Resolve", timeToResolve},
		{"Visibility", visibility},
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

// incidentsStringField safely extracts a string value from a map.
func incidentsStringField(m map[string]interface{}, key string) string {
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

// incidentsPublicID extracts a human-readable ID from the incident.
func incidentsPublicID(inc, attrs map[string]interface{}) string {
	if attrs != nil {
		if pid, ok := attrs["public_id"]; ok && pid != nil {
			return fmt.Sprintf("%v", pid)
		}
	}
	if inc != nil {
		if id, ok := inc["id"].(string); ok && len(id) > 8 {
			return id[:8]
		}
		if id, ok := inc["id"]; ok && id != nil {
			return fmt.Sprintf("%v", id)
		}
	}
	return ""
}

// incidentsSeverity extracts the severity from incident attributes.
func incidentsSeverity(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	if sev := incidentsStringField(attrs, "severity"); sev != "" {
		return sev
	}
	// Try fields map
	if fields, ok := attrs["fields"].(map[string]interface{}); ok {
		if sevField, ok := fields["severity"].(map[string]interface{}); ok {
			return incidentsStringField(sevField, "value")
		}
	}
	return ""
}

// incidentsStatus extracts the state/status from incident attributes.
func incidentsStatus(attrs map[string]interface{}) string {
	if attrs == nil {
		return ""
	}
	if state := incidentsStringField(attrs, "state"); state != "" {
		return state
	}
	// Try fields map
	if fields, ok := attrs["fields"].(map[string]interface{}); ok {
		if stateField, ok := fields["state"].(map[string]interface{}); ok {
			return incidentsStringField(stateField, "value")
		}
	}
	return ""
}

// incidentsFormatTimestamp formats an ISO 8601 timestamp string for display.
func incidentsFormatTimestamp(ts interface{}) string {
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
	incidentsCmd.AddCommand(incidentsListCmd)
	incidentsCmd.AddCommand(incidentsGetCmd)

	// Register with root
	rootCmd.AddCommand(incidentsCmd)
}
