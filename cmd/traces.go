package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- traces command group ----

var tracesCmd = &cobra.Command{
	Use:   "traces",
	Short: "Query APM traces and spans from Datadog",
	Long: `Query APM traces and spans from Datadog.

Note: The spans search and aggregate APIs are rate-limited to 300 requests/hour.

Subcommands:
  search     Search spans matching a query
  aggregate  Aggregate spans by fields
  get        Get a specific span by ID`,
}

// ---- traces search ----

var (
	tracesSearchQuery       string
	tracesSearchFrom        string
	tracesSearchTo          string
	tracesSearchSort        string
	tracesSearchFilterQuery string
)

var tracesSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search spans matching a query (rate limit: 300 req/hour)",
	Long: `Search APM spans using Datadog query syntax.

Uses POST /api/v2/spans/events/search.
Rate limit: 300 requests/hour.

Examples:
  datadog-cli traces search --query "service:my-app"
  datadog-cli traces search -q "service:api @duration:>1s" --from 1h
  datadog-cli traces search -q "service:api env:production" --from 2h --to 1h
  datadog-cli traces search -q "service:api" --sort -duration
  datadog-cli traces search -q "*" --limit 50 --json`,
	RunE: runTracesSearch,
}

func runTracesSearch(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(tracesSearchFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(tracesSearchTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromMs >= toMs {
		return fmt.Errorf("--from time must be before --to time")
	}

	client := newClient()
	opts := GetOutputOptions()

	pageSize := flagLimit
	if pageSize > 1000 {
		pageSize = 1000
	}

	// Build filter block
	filter := map[string]interface{}{
		"query": tracesSearchQuery,
		"from":  strconv.FormatInt(fromMs, 10),
		"to":    strconv.FormatInt(toMs, 10),
	}
	if tracesSearchFilterQuery != "" {
		filter["query"] = tracesSearchFilterQuery
	}

	attributes := map[string]interface{}{
		"filter": filter,
		"page": map[string]interface{}{
			"limit": pageSize,
		},
	}

	// Handle sort flag
	if tracesSearchSort != "" {
		sort := tracesSearchSort
		// Normalize: "-duration" → "-@duration", "duration" → "@duration"
		if strings.HasPrefix(sort, "-") {
			field := sort[1:]
			if !strings.HasPrefix(field, "@") {
				sort = "-@" + field
			}
		} else if !strings.HasPrefix(sort, "@") && sort != "timestamp" && sort != "-timestamp" {
			sort = "@" + sort
		}
		attributes["sort"] = sort
	}

	reqBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "search_request",
			"attributes": attributes,
		},
	}

	// Table row struct
	type spanRow struct {
		Timestamp string
		Service   string
		Resource  string
		Duration  string
		Status    string
	}

	var rows []spanRow
	var lastRaw interface{}
	cursor := ""

	for len(rows) < flagLimit {
		if cursor != "" {
			if pageMap, ok := attributes["page"].(map[string]interface{}); ok {
				pageMap["cursor"] = cursor
			}
		}

		respBytes, err := client.Post("/api/v2/spans/events/search", reqBody, nil)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		if opts.JSON && cursor == "" {
			return output.RenderJSON(raw, opts)
		}

		data, _ := raw["data"].([]interface{})
		for _, item := range data {
			if len(rows) >= flagLimit {
				break
			}
			entry, _ := item.(map[string]interface{})
			attrs, _ := entry["attributes"].(map[string]interface{})

			// Timestamp
			ts := ""
			if tsRaw, ok := attrs["timestamp"].(string); ok && tsRaw != "" {
				if t, err := time.Parse(time.RFC3339Nano, tsRaw); err == nil {
					ts = t.UTC().Format("2006-01-02 15:04:05")
				} else if t, err := time.Parse(time.RFC3339, tsRaw); err == nil {
					ts = t.UTC().Format("2006-01-02 15:04:05")
				} else {
					ts = tsRaw
				}
			}

			// Duration from nanoseconds
			dur := formatSpanDuration(attrs["duration"])

			// HTTP status or error status
			status := ""
			if meta, ok := attrs["meta"].(map[string]interface{}); ok {
				if sc, ok := meta["http.status_code"].(string); ok {
					status = sc
				}
			}
			if status == "" {
				if errVal, ok := attrs["error"]; ok {
					switch v := errVal.(type) {
					case float64:
						if v == 1 {
							status = "error"
						} else {
							status = "ok"
						}
					case bool:
						if v {
							status = "error"
						} else {
							status = "ok"
						}
					}
				}
			}

			rows = append(rows, spanRow{
				Timestamp: ts,
				Service:   stringFieldFromMap(attrs, "service"),
				Resource:  output.TruncateString(stringFieldFromMap(attrs, "resource_name"), 60),
				Duration:  dur,
				Status:    status,
			})
		}

		// Check for next cursor
		meta, _ := raw["meta"].(map[string]interface{})
		page, _ := meta["page"].(map[string]interface{})
		nextCursor, _ := page["after"].(string)
		if nextCursor == "" || len(data) == 0 {
			break
		}
		cursor = nextCursor
	}

	if opts.JSON {
		return output.RenderJSON(lastRaw, opts)
	}

	if len(rows) == 0 {
		fmt.Fprintln(os.Stdout, "No spans found matching your query.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "Timestamp"},
		{Name: "Service", Width: 25},
		{Name: "Resource", Width: 60},
		{Name: "Duration"},
		{Name: "Status"},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Timestamp, r.Service, r.Resource, r.Duration, r.Status}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- traces aggregate ----

var (
	tracesAggQuery   string
	tracesAggFrom    string
	tracesAggTo      string
	tracesAggCompute string
	tracesAggGroupBy string
)

var tracesAggregateCmd = &cobra.Command{
	Use:   "aggregate",
	Short: "Aggregate spans by fields (rate limit: 300 req/hour)",
	Long: `Aggregate APM spans using Datadog Analytics.

Uses POST /api/v2/spans/analytics/aggregate.
Rate limit: 300 requests/hour.

Examples:
  datadog-cli traces aggregate -q "service:api" --group-by service --compute count
  datadog-cli traces aggregate -q "env:production" --group-by resource_name
  datadog-cli traces aggregate -q "service:api" --group-by service --compute avg --from 2h
  datadog-cli traces aggregate -q "*" --from 1d --group-by service --json`,
	RunE: runTracesAggregate,
}

func runTracesAggregate(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(tracesAggFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(tracesAggTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromMs >= toMs {
		return fmt.Errorf("--from time must be before --to time")
	}

	client := newClient()
	opts := GetOutputOptions()

	attributes := map[string]interface{}{
		"filter": map[string]interface{}{
			"query": tracesAggQuery,
			"from":  strconv.FormatInt(fromMs, 10),
			"to":    strconv.FormatInt(toMs, 10),
		},
		"compute": []map[string]interface{}{
			{
				"aggregation": tracesAggCompute,
				"type":        "total",
			},
		},
	}

	if tracesAggGroupBy != "" {
		attributes["group_by"] = []map[string]interface{}{
			{
				"facet": tracesAggGroupBy,
				"limit": flagLimit,
				"sort": map[string]interface{}{
					"type":        "measure",
					"aggregation": tracesAggCompute,
					"order":       "desc",
				},
			},
		}
	}

	reqBody := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "aggregate_request",
			"attributes": attributes,
		},
	}

	respBytes, err := client.Post("/api/v2/spans/analytics/aggregate", reqBody, nil)
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

	// Extract buckets — response: {"data": [{"type":"bucket","attributes":{"by":{...},"compute":{...}}}]}
	dataField, _ := raw["data"].([]interface{})

	if len(dataField) == 0 {
		fmt.Fprintln(os.Stdout, "No spans found for aggregation.")
		return nil
	}

	type aggRow map[string]string
	var aggRows []aggRow
	var keyOrder []string

	for i, b := range dataField {
		bucket, _ := b.(map[string]interface{})
		attrs, _ := bucket["attributes"].(map[string]interface{})
		row := make(aggRow)

		byMap, _ := attrs["by"].(map[string]interface{})
		computes, _ := attrs["compute"].(map[string]interface{})

		if i == 0 {
			for k := range byMap {
				keyOrder = append(keyOrder, k)
			}
			for k := range computes {
				keyOrder = append(keyOrder, k)
			}
		}

		for k, v := range byMap {
			row[k] = fmt.Sprintf("%v", v)
		}
		for k, v := range computes {
			row[k] = fmt.Sprintf("%v", v)
		}
		aggRows = append(aggRows, row)
	}

	cols := make([]output.ColumnConfig, len(keyOrder))
	for i, k := range keyOrder {
		header := strings.ReplaceAll(k, "_", " ")
		header = strings.Title(header) //nolint:staticcheck
		cols[i] = output.ColumnConfig{Name: header}
	}

	tableRows := make([][]string, len(aggRows))
	for i, row := range aggRows {
		tableRow := make([]string, len(keyOrder))
		for j, k := range keyOrder {
			tableRow[j] = row[k]
		}
		tableRows[i] = tableRow
	}

	return output.RenderTable(cols, tableRows, aggRows, opts)
}

// ---- traces get ----

var tracesGetCmd = &cobra.Command{
	Use:   "get <span_id>",
	Short: "Get a specific span by ID",
	Long: `Retrieve detailed information about a specific span by its span ID.

Uses GET /api/v2/spans/events/{span_id}.

Examples:
  datadog-cli traces get abc123def456
  datadog-cli traces get abc123def456 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTracesGet,
}

func runTracesGet(cmd *cobra.Command, args []string) error {
	spanID := args[0]

	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/spans/events/"+spanID, nil)
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

	// Extract span data from response
	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		fmt.Fprintln(os.Stdout, "Span not found.")
		return nil
	}

	attrs, _ := data["attributes"].(map[string]interface{})

	// Build display rows from span attributes
	type detailRow struct {
		Field string
		Value string
	}

	rows := []detailRow{
		{Field: "Span ID", Value: spanID},
		{Field: "Trace ID", Value: stringFieldFromMap(attrs, "trace_id")},
		{Field: "Service", Value: stringFieldFromMap(attrs, "service")},
		{Field: "Resource", Value: stringFieldFromMap(attrs, "resource_name")},
		{Field: "Operation", Value: stringFieldFromMap(attrs, "name")},
		{Field: "Duration", Value: formatSpanDuration(attrs["duration"])},
		{Field: "Timestamp", Value: stringFieldFromMap(attrs, "timestamp")},
	}

	// Append HTTP status if present
	if meta, ok := attrs["meta"].(map[string]interface{}); ok {
		if sc, ok := meta["http.status_code"].(string); ok && sc != "" {
			rows = append(rows, detailRow{Field: "HTTP Status", Value: sc})
		}
	}

	// Append error status
	if errVal, ok := attrs["error"]; ok {
		errStr := fmt.Sprintf("%v", errVal)
		rows = append(rows, detailRow{Field: "Error", Value: errStr})
	}

	cols := []output.ColumnConfig{
		{Name: "Field"},
		{Name: "Value", Width: 80},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Field, r.Value}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

// formatSpanDuration formats a duration value (nanoseconds as float64 or int64)
// to a compact human-readable string (e.g., "12.3ms", "1.5s").
func formatSpanDuration(v interface{}) string {
	if v == nil {
		return ""
	}
	var ns int64
	switch val := v.(type) {
	case float64:
		ns = int64(val)
	case int64:
		ns = val
	case int:
		ns = int64(val)
	case json.Number:
		if n, err := val.Int64(); err == nil {
			ns = n
		} else {
			return fmt.Sprintf("%v", v)
		}
	default:
		return fmt.Sprintf("%v", v)
	}
	return output.FormatDuration(ns)
}

// stringFieldFromMap safely extracts a string value from a map.
func stringFieldFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// ---- init ----

func init() {
	// traces search flags
	tracesSearchCmd.Flags().StringVarP(&tracesSearchQuery, "query", "q", "", "Span search query (required)")
	tracesSearchCmd.Flags().StringVar(&tracesSearchFrom, "from", "15m", "Start time (e.g. 15m, 1h, 2d, 1w, now, ISO-8601, unix)")
	tracesSearchCmd.Flags().StringVar(&tracesSearchTo, "to", "now", "End time (e.g. now, ISO-8601, unix)")
	tracesSearchCmd.Flags().StringVar(&tracesSearchSort, "sort", "", "Sort field (e.g. duration, -duration, timestamp)")
	tracesSearchCmd.Flags().StringVar(&tracesSearchFilterQuery, "filter-query", "", "Additional filter query to apply")
	_ = tracesSearchCmd.MarkFlagRequired("query")

	// traces aggregate flags
	tracesAggregateCmd.Flags().StringVarP(&tracesAggQuery, "query", "q", "", "Span search query (required)")
	tracesAggregateCmd.Flags().StringVar(&tracesAggFrom, "from", "15m", "Start time")
	tracesAggregateCmd.Flags().StringVar(&tracesAggTo, "to", "now", "End time")
	tracesAggregateCmd.Flags().StringVar(&tracesAggCompute, "compute", "count", "Aggregation type: count, sum, avg, min, max")
	tracesAggregateCmd.Flags().StringVar(&tracesAggGroupBy, "group-by", "", "Field to group by (e.g. service, resource_name)")
	_ = tracesAggregateCmd.MarkFlagRequired("query")

	// Add subcommands to traces
	tracesCmd.AddCommand(tracesSearchCmd)
	tracesCmd.AddCommand(tracesAggregateCmd)
	tracesCmd.AddCommand(tracesGetCmd)

	// Add traces to root
	rootCmd.AddCommand(tracesCmd)
}
