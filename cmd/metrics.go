package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- metrics command group ----

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Query and list metrics from Datadog",
	Long:  `Query and list metrics from Datadog.`,
	Example: `  # List active metrics from the last hour
  datadog-cli metrics list --from 1h

  # Query CPU usage timeseries
  datadog-cli metrics query -q "avg:system.cpu.user{*}"

  # Search for metrics matching a pattern
  datadog-cli metrics search -q "system.cpu"`,
}

// parseTimeSeconds parses a time string and returns Unix seconds.
// Reuses the same format as parseTime but divides milliseconds by 1000.
func parseTimeSeconds(s string) (int64, error) {
	ms, err := parseTime(s)
	if err != nil {
		return 0, err
	}
	return ms / 1000, nil
}

// ---- metrics list ----

var metricsListFrom string

var metricsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available metric names",
	Long: `List all metric names that have reported data to Datadog.

Uses GET /api/v1/metrics.

The --from flag limits results to metrics that have received data points
within the specified time window. Time is specified in Unix seconds.`,
	Example: `  # List all active metrics
  datadog-cli metrics list

  # List metrics active in the last hour
  datadog-cli metrics list --from 1h

  # List metrics active in the last 2 days as JSON
  datadog-cli metrics list --from 2d --json`,
	RunE: runMetricsList,
}

func runMetricsList(cmd *cobra.Command, args []string) error {
	fromSecs, err := parseTimeSeconds(metricsListFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}

	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("from", fmt.Sprintf("%d", fromSecs))

	respBytes, err := client.Get("/api/v1/metrics", params)
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

	metricsList, _ := raw["metrics"].([]interface{})
	if len(metricsList) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No metrics found.")
		return nil
	}

	// Apply limit
	limit := flagLimit
	if limit > 0 && len(metricsList) > limit {
		metricsList = metricsList[:limit]
	}

	type metricRow struct {
		MetricName string
	}

	rows := make([]metricRow, 0, len(metricsList))
	tableRows := make([][]string, 0, len(metricsList))
	for _, item := range metricsList {
		name, _ := item.(string)
		rows = append(rows, metricRow{MetricName: name})
		tableRows = append(tableRows, []string{name})
	}

	cols := []output.ColumnConfig{
		{Name: "Metric Name", Width: 80},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- metrics query ----

var (
	metricsQueryQuery string
	metricsQueryFrom  string
	metricsQueryTo    string
)

var metricsQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query metric timeseries data",
	Long: `Execute a metrics query and return timeseries data points.

Uses GET /api/v1/query.

The query uses Datadog's metrics query syntax. Time is in Unix seconds.`,
	Example: `  # Query average CPU usage across all hosts
  datadog-cli metrics query --query "avg:system.cpu.user{*}"

  # Query HTTP request hits for the API service over 2 hours
  datadog-cli metrics query -q "sum:trace.http.request.hits{service:api}" --from 2h

  # Query memory usage per host as JSON
  datadog-cli metrics query -q "avg:system.mem.used{*}" --from 1d --json`,
	RunE: runMetricsQuery,
}

func runMetricsQuery(cmd *cobra.Command, args []string) error {
	if metricsQueryQuery == "" {
		return fmt.Errorf("--query is required")
	}

	fromSecs, err := parseTimeSeconds(metricsQueryFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toSecs, err := parseTimeSeconds(metricsQueryTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromSecs >= toSecs {
		return fmt.Errorf("--from time must be before --to time")
	}

	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("query", metricsQueryQuery)
	params.Set("from", fmt.Sprintf("%d", fromSecs))
	params.Set("to", fmt.Sprintf("%d", toSecs))

	respBytes, err := client.Get("/api/v1/query", params)
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

	seriesList, _ := raw["series"].([]interface{})
	if len(seriesList) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No timeseries data found.")
		return nil
	}

	type queryRow struct {
		Metric string
		Scope  string
		Points string
		Unit   string
	}

	rows := make([]queryRow, 0, len(seriesList))
	tableRows := make([][]string, 0, len(seriesList))
	for _, item := range seriesList {
		series, _ := item.(map[string]interface{})
		metric := stringField(series, "metric")
		if metric == "" {
			metric = stringField(series, "expression")
		}
		scope := stringField(series, "scope")

		// Count points in pointlist
		pointCount := 0
		if pl, ok := series["pointlist"].([]interface{}); ok {
			pointCount = len(pl)
		}

		// Extract unit from unit array
		unit := ""
		if unitRaw, ok := series["unit"].([]interface{}); ok && len(unitRaw) > 0 {
			if unitMap, ok := unitRaw[0].(map[string]interface{}); ok {
				unit = stringField(unitMap, "short_name")
				if unit == "" {
					unit = stringField(unitMap, "name")
				}
			}
		}

		row := queryRow{
			Metric: metric,
			Scope:  scope,
			Points: fmt.Sprintf("%d", pointCount),
			Unit:   unit,
		}
		rows = append(rows, row)
		tableRows = append(tableRows, []string{metric, scope, row.Points, unit})
	}

	// Apply limit
	limit := flagLimit
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
		tableRows = tableRows[:limit]
	}

	cols := []output.ColumnConfig{
		{Name: "Metric", Width: 50},
		{Name: "Scope", Width: 40},
		{Name: "Points"},
		{Name: "Unit"},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- metrics search ----

var metricsSearchQuery string

var metricsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search for metrics by name pattern",
	Long: `Search for metrics by name pattern using Datadog's search API.

Uses GET /api/v1/search.

The query can be prefixed with "metrics:" to search metric names,
or "hosts:" to search host names. If no prefix is provided, "metrics:"
is prepended automatically.`,
	Example: `  # Search for CPU-related metrics
  datadog-cli metrics search --query "system.cpu"

  # Search for AWS EC2 metrics
  datadog-cli metrics search -q "aws.ec2"

  # Search for trace HTTP metrics as JSON
  datadog-cli metrics search -q "trace.http" --json`,
	RunE: runMetricsSearch,
}

func runMetricsSearch(cmd *cobra.Command, args []string) error {
	if metricsSearchQuery == "" {
		return fmt.Errorf("--query is required")
	}

	client := newClient()
	opts := GetOutputOptions()

	// Prepend "metrics:" prefix if not already prefixed with a recognized namespace
	q := metricsSearchQuery
	if !strings.HasPrefix(q, "metrics:") && !strings.HasPrefix(q, "hosts:") {
		q = "metrics:" + q
	}

	params := url.Values{}
	params.Set("q", q)

	respBytes, err := client.Get("/api/v1/search", params)
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

	results, _ := raw["results"].(map[string]interface{})
	metricsList, _ := results["metrics"].([]interface{})
	if len(metricsList) == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "No metrics found matching %q.\n", metricsSearchQuery)
		return nil
	}

	// Apply limit
	limit := flagLimit
	if limit > 0 && len(metricsList) > limit {
		metricsList = metricsList[:limit]
	}

	type metricRow struct {
		MetricName string
	}

	rows := make([]metricRow, 0, len(metricsList))
	tableRows := make([][]string, 0, len(metricsList))
	for _, item := range metricsList {
		name, _ := item.(string)
		rows = append(rows, metricRow{MetricName: name})
		tableRows = append(tableRows, []string{name})
	}

	cols := []output.ColumnConfig{
		{Name: "Metric Name", Width: 80},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- init ----

func init() {
	// metrics list flags
	metricsListCmd.Flags().StringVar(&metricsListFrom, "from", "1h", "List metrics active since this time offset (e.g., '1h', '2d', '30m')")

	// metrics query flags
	metricsQueryCmd.Flags().StringVarP(&metricsQueryQuery, "query", "q", "", "Metrics query expression (required)")
	metricsQueryCmd.Flags().StringVar(&metricsQueryFrom, "from", "1h", "Start time as offset (e.g., '1h', '2d') or 'now'")
	metricsQueryCmd.Flags().StringVar(&metricsQueryTo, "to", "now", "End time as offset (e.g., '30m') or 'now'")

	// metrics search flags
	metricsSearchCmd.Flags().StringVarP(&metricsSearchQuery, "query", "q", "", "Search query for metric names (required)")

	// Wire up subcommands
	metricsCmd.AddCommand(metricsListCmd)
	metricsCmd.AddCommand(metricsQueryCmd)
	metricsCmd.AddCommand(metricsSearchCmd)

	// Register with root
	rootCmd.AddCommand(metricsCmd)
}
