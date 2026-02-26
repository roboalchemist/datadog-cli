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

// ---- usage command group ----

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Query usage metering from Datadog",
	Long:  `Query usage metering from Datadog.`,
	Example: `  # Get usage summary for January 2024
  datadog-cli usage summary --start-month 2024-01

  # Get top custom metrics for the current month
  datadog-cli usage top-metrics`,
}

// ---- usage summary ----

var (
	usageSummaryStartMonth string
	usageSummaryEndMonth   string
)

var usageSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Get usage summary",
	Long: `Get usage summary across products for a given month range.

Uses GET /api/v1/usage/summary. Requires --start-month (YYYY-MM).

Note: This endpoint requires usage_read scope and is only accessible
for parent-level organizations.`,
	Example: `  # Get usage summary for January 2024
  datadog-cli usage summary --start-month 2024-01

  # Get usage summary for a quarter
  datadog-cli usage summary --start-month 2024-01 --end-month 2024-03

  # Get usage summary as JSON
  datadog-cli usage summary --start-month 2024-01 --json`,
	RunE: runUsageSummary,
}

func runUsageSummary(cmd *cobra.Command, args []string) error {
	if usageSummaryStartMonth == "" {
		return fmt.Errorf("--start-month is required (format: YYYY-MM)")
	}

	client := newClient()
	opts := GetOutputOptions()

	// Validate month format
	if _, err := time.Parse("2006-01", usageSummaryStartMonth); err != nil {
		return fmt.Errorf("--start-month %q: use YYYY-MM format (e.g. 2024-01)", usageSummaryStartMonth)
	}

	params := url.Values{}
	params.Set("start_month", usageSummaryStartMonth)
	if usageSummaryEndMonth != "" {
		if _, err := time.Parse("2006-01", usageSummaryEndMonth); err != nil {
			return fmt.Errorf("--end-month %q: use YYYY-MM format (e.g. 2024-03)", usageSummaryEndMonth)
		}
		params.Set("end_month", usageSummaryEndMonth)
	}

	respBytes, err := client.Get("/api/v1/usage/summary", params)
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

	// Extract per-month usage entries from the "usage" array
	usageArr, _ := raw["usage"].([]interface{})

	if len(usageArr) == 0 {
		fmt.Fprintln(os.Stdout, "No usage data found.")
		return nil
	}

	type usageRow struct {
		Month         string
		Hosts         string
		Containers    string
		CustomMetrics string
		Logs          string
	}

	rows := make([]usageRow, 0, len(usageArr))
	tableRows := make([][]string, 0, len(usageArr))

	for _, item := range usageArr {
		u, _ := item.(map[string]interface{})
		month := usageStringField(u, "date")
		if month == "" {
			month = usageStringField(u, "start_date")
		}
		// Trim to YYYY-MM
		if len(month) > 7 {
			month = month[:7]
		}

		hosts := usageFormatInt(u["infra_host_top99p"])
		containers := usageFormatInt(u["container_count_sum"])
		customMetrics := usageFormatInt(u["custom_ts_sum"])
		logs := usageFormatInt(u["logs_indexed_logs_usage_sum"])

		rows = append(rows, usageRow{
			Month:         month,
			Hosts:         hosts,
			Containers:    containers,
			CustomMetrics: customMetrics,
			Logs:          logs,
		})
		tableRows = append(tableRows, []string{month, hosts, containers, customMetrics, logs})
	}

	cols := []output.ColumnConfig{
		{Name: "Month", Width: 10},
		{Name: "Hosts", Width: 12},
		{Name: "Containers", Width: 12},
		{Name: "Custom Metrics", Width: 16},
		{Name: "Logs", Width: 16},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- usage top-metrics ----

var (
	usageTopMetricsMonth      string
	usageTopMetricsMetricName string
)

var usageTopMetricsCmd = &cobra.Command{
	Use:   "top-metrics",
	Short: "Get top average custom metrics",
	Long: `Get all custom metrics by hourly average for a given month.

Uses GET /api/v1/usage/top_avg_metrics.`,
	Example: `  # Get top custom metrics for January 2024
  datadog-cli usage top-metrics --month 2024-01

  # Filter top metrics by name
  datadog-cli usage top-metrics --month 2024-01 --metric-name "my.metric"

  # Get top metrics as JSON
  datadog-cli usage top-metrics --json`,
	RunE: runUsageTopMetrics,
}

func runUsageTopMetrics(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}

	if usageTopMetricsMonth != "" {
		if _, err := time.Parse("2006-01", usageTopMetricsMonth); err != nil {
			return fmt.Errorf("--month %q: use YYYY-MM format (e.g. 2024-01)", usageTopMetricsMonth)
		}
		params.Set("month", usageTopMetricsMonth)
	} else {
		// Default to current month
		params.Set("month", time.Now().Format("2006-01"))
	}

	if usageTopMetricsMetricName != "" {
		params.Set("names[]", usageTopMetricsMetricName)
	}

	respBytes, err := client.Get("/api/v1/usage/top_avg_metrics", params)
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

	usageArr, _ := raw["usage"].([]interface{})

	if len(usageArr) == 0 {
		fmt.Fprintln(os.Stdout, "No top metrics data found.")
		return nil
	}

	if flagLimit > 0 && len(usageArr) > flagLimit {
		usageArr = usageArr[:flagLimit]
	}

	type metricRow struct {
		Metric   string
		AvgCount string
		MaxCount string
	}

	rows := make([]metricRow, 0, len(usageArr))
	tableRows := make([][]string, 0, len(usageArr))

	for _, item := range usageArr {
		m, _ := item.(map[string]interface{})
		metricName := output.TruncateString(usageStringField(m, "metric_name"), 60)
		avgCount := usageFormatFloat(m["avg_metric_hour"])
		maxCount := usageFormatFloat(m["max_metric_hour"])

		rows = append(rows, metricRow{Metric: metricName, AvgCount: avgCount, MaxCount: maxCount})
		tableRows = append(tableRows, []string{metricName, avgCount, maxCount})
	}

	cols := []output.ColumnConfig{
		{Name: "Metric", Width: 60},
		{Name: "Avg Count", Width: 14},
		{Name: "Max Count", Width: 14},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

func usageStringField(m map[string]interface{}, key string) string {
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

func usageFormatInt(v interface{}) string {
	if v == nil {
		return ""
	}
	switch n := v.(type) {
	case float64:
		return fmt.Sprintf("%d", int64(n))
	case int64:
		return fmt.Sprintf("%d", n)
	case int:
		return fmt.Sprintf("%d", n)
	default:
		s := fmt.Sprintf("%v", v)
		return strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
}

func usageFormatFloat(v interface{}) string {
	if v == nil {
		return ""
	}
	if n, ok := v.(float64); ok {
		return fmt.Sprintf("%.2f", n)
	}
	return fmt.Sprintf("%v", v)
}

// ---- init ----

func init() {
	usageSummaryCmd.Flags().StringVar(&usageSummaryStartMonth, "start-month", "", "Start month in YYYY-MM format (required)")
	usageSummaryCmd.Flags().StringVar(&usageSummaryEndMonth, "end-month", "", "End month in YYYY-MM format (optional)")

	usageTopMetricsCmd.Flags().StringVar(&usageTopMetricsMonth, "month", "", "Month in YYYY-MM format (default: current month)")
	usageTopMetricsCmd.Flags().StringVar(&usageTopMetricsMetricName, "metric-name", "", "Filter by metric name")

	usageCmd.AddCommand(usageSummaryCmd)
	usageCmd.AddCommand(usageTopMetricsCmd)

	rootCmd.AddCommand(usageCmd)
}
