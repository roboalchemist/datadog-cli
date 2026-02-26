package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- pipelines command group ----

var pipelinesCmd = &cobra.Command{
	Use:   "pipelines",
	Short: "Query log pipeline configurations from Datadog",
	Long: `Query log pipeline configurations from Datadog.

Subcommands:
  list  List log pipelines
  get   Get pipeline details by ID`,
}

// ---- pipelines list ----

var pipelinesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List log pipelines",
	Long: `List log pipelines configured in your Datadog account.

Uses GET /api/v1/logs/config/pipelines.
Required scope: logs_read_config

Examples:
  datadog-cli pipelines list
  datadog-cli pipelines list --json`,
	RunE: runPipelinesList,
}

func runPipelinesList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/logs/config/pipelines", nil)
	if err != nil {
		return err
	}

	// API returns a JSON array directly
	var rawArr []interface{}
	if err := json.Unmarshal(respBytes, &rawArr); err != nil {
		// Try object wrapper as fallback
		var rawObj map[string]interface{}
		if err2 := json.Unmarshal(respBytes, &rawObj); err2 != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		if opts.JSON {
			return output.RenderJSON(rawObj, opts)
		}
		fmt.Fprintln(os.Stdout, "No pipelines found.")
		return nil
	}

	if opts.JSON {
		return output.RenderJSON(rawArr, opts)
	}

	if len(rawArr) == 0 {
		fmt.Fprintln(os.Stdout, "No log pipelines found.")
		return nil
	}

	if flagLimit > 0 && len(rawArr) > flagLimit {
		rawArr = rawArr[:flagLimit]
	}

	type pipelineRow struct {
		ID      string
		Name    string
		Type    string
		Enabled string
		Filter  string
	}

	rows := make([]pipelineRow, 0, len(rawArr))
	tableRows := make([][]string, 0, len(rawArr))

	for _, item := range rawArr {
		p, _ := item.(map[string]interface{})
		id := output.TruncateString(pipelinesStringField(p, "id"), 24)
		name := output.TruncateString(pipelinesStringField(p, "name"), 30)
		pType := pipelinesStringField(p, "type")
		if pType == "" {
			pType = "pipeline"
		}
		enabled := pipelinesEnabledField(p)
		filter := pipelinesExtractFilter(p)

		rows = append(rows, pipelineRow{ID: id, Name: name, Type: pType, Enabled: enabled, Filter: filter})
		tableRows = append(tableRows, []string{id, name, pType, enabled, filter})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 24},
		{Name: "Name", Width: 30},
		{Name: "Type", Width: 12},
		{Name: "Enabled", Width: 10},
		{Name: "Filter", Width: 40},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- pipelines get ----

var pipelinesGetCmd = &cobra.Command{
	Use:   "get <pipeline_id>",
	Short: "Get log pipeline details by ID",
	Long: `Get detailed information about a specific log pipeline.

Uses GET /api/v1/logs/config/pipelines/{id}.
Required scope: logs_read_config

Examples:
  datadog-cli pipelines get abc123-def456
  datadog-cli pipelines get abc123-def456 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPipelinesGet,
}

func runPipelinesGet(cmd *cobra.Command, args []string) error {
	pipelineID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/logs/config/pipelines/"+pipelineID, nil)
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

	name := pipelinesStringField(raw, "name")
	fmt.Fprintf(os.Stdout, "Pipeline: %s\n\n", name)

	processors, _ := raw["processors"].([]interface{})
	processorCount := fmt.Sprintf("%d", len(processors))

	// Collect unique processor types
	procTypes := make([]string, 0)
	seen := map[string]bool{}
	for _, proc := range processors {
		if pm, ok := proc.(map[string]interface{}); ok {
			pt := pipelinesStringField(pm, "type")
			if pt != "" && !seen[pt] {
				procTypes = append(procTypes, pt)
				seen[pt] = true
			}
		}
	}

	details := []struct{ k, v string }{
		{"ID", pipelinesStringField(raw, "id")},
		{"Name", name},
		{"Type", pipelinesStringField(raw, "type")},
		{"Enabled", pipelinesEnabledField(raw)},
		{"Read Only", pipelinesBoolField(raw, "is_read_only")},
		{"Filter", pipelinesExtractFilter(raw)},
		{"Processor Count", processorCount},
		{"Processor Types", strings.Join(procTypes, ", ")},
	}

	type detailRow struct {
		Field string
		Value string
	}

	detailRows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" {
			continue
		}
		detailRows = append(detailRows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 18},
		{Name: "Value", Width: 60},
	}

	return output.RenderTable(cols, tableRows, detailRows, opts)
}

// ---- helpers ----

func pipelinesStringField(m map[string]interface{}, key string) string {
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

func pipelinesEnabledField(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	v, ok := m["is_enabled"]
	if !ok || v == nil {
		return ""
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	if b {
		return output.ColorStatus("Enabled")
	}
	return "Disabled"
}

func pipelinesBoolField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	if b {
		return "Yes"
	}
	return "No"
}

func pipelinesExtractFilter(p map[string]interface{}) string {
	if p == nil {
		return ""
	}
	filterObj, ok := p["filter"].(map[string]interface{})
	if !ok {
		return ""
	}
	query := pipelinesStringField(filterObj, "query")
	return output.TruncateString(query, 40)
}

// ---- init ----

func init() {
	pipelinesCmd.AddCommand(pipelinesListCmd)
	pipelinesCmd.AddCommand(pipelinesGetCmd)

	rootCmd.AddCommand(pipelinesCmd)
}
