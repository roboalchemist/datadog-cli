package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// maxLogsPageSize is the maximum page[limit] accepted by the Datadog logs search API.
const maxLogsPageSize = 1000

// ---- logs command group ----

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Query and aggregate logs from Datadog Log Explorer",
	Long:  `Query logs from Datadog Log Explorer.`,
	Example: `  # Search logs for errors in the last hour
  datadog-cli logs search -q "status:error" --from 1h

  # Aggregate logs by service
  datadog-cli logs aggregate -q "*" --group-by service --compute count

  # List configured log indexes
  datadog-cli logs indexes`,
}

// ---- logs search ----

var (
	logsSearchQuery   string
	logsSearchFrom    string
	logsSearchTo      string
	logsSearchSort    string
	logsSearchIndexes []string
	logsSearchAll     bool
)

var logsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search logs matching a query",
	Long:  `Search logs using Datadog query syntax.`,
	Example: `  # Search for errors in the my-app service
  datadog-cli logs search --query "service:my-app status:error"

  # Search with a time range
  datadog-cli logs search -q "service:api-gateway" --from 1h --to now

  # Search for HTTP 5xx errors and output JSON
  datadog-cli logs search -q "@http.status_code:>=500" --limit 50 --json`,
	RunE: runLogsSearch,
}

func runLogsSearch(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(logsSearchFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(logsSearchTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromMs >= toMs {
		return fmt.Errorf("--from time must be before --to time")
	}

	// Validate sort value
	if logsSearchSort != "timestamp" && logsSearchSort != "-timestamp" {
		return fmt.Errorf("--sort must be 'timestamp' or '-timestamp', got %q", logsSearchSort)
	}

	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if logsSearchAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// First page size: min(effectiveLimit, maxLogsPageSize).
	// If unlimited (--all), use max page size.
	firstPageSize := effectiveLimit
	if firstPageSize == 0 || firstPageSize > maxLogsPageSize {
		firstPageSize = maxLogsPageSize
	}

	// Build request body
	filter := map[string]interface{}{
		"query": logsSearchQuery,
		"from":  strconv.FormatInt(fromMs, 10),
		"to":    strconv.FormatInt(toMs, 10),
	}
	if len(logsSearchIndexes) > 0 {
		filter["indexes"] = logsSearchIndexes
	}

	pageMap := map[string]interface{}{
		"limit": firstPageSize,
	}

	reqBody := map[string]interface{}{
		"filter": filter,
		"sort":   logsSearchSort,
		"page":   pageMap,
	}

	// Table row struct
	type logRow struct {
		Timestamp string
		Host      string
		Service   string
		Status    string
		Message   string
	}

	var rows []logRow
	// allData accumulates raw log items for JSON output across pages.
	var allData []interface{}
	// lastRaw holds the most recent raw response for metadata.
	var lastRaw map[string]interface{}
	cursor := ""
	pageNum := 0

	needsMorePages := func() bool {
		if logsSearchAll {
			return true // stop only when cursor is gone
		}
		return len(rows) < effectiveLimit
	}

	for needsMorePages() {
		pageNum++

		if cursor != "" {
			pageMap["cursor"] = cursor
			// Adjust page size for the last page when limit is set.
			if !logsSearchAll {
				remaining := effectiveLimit - len(rows)
				ps := remaining
				if ps > maxLogsPageSize {
					ps = maxLogsPageSize
				}
				pageMap["limit"] = ps
			}
			// Print progress to stderr (unless --quiet).
			if !flagQuiet {
				if logsSearchAll {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d logs so far)...\n", pageNum, len(rows))
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
				}
			}
		}

		respBytes, err := client.Post("/api/v2/logs/events/search", reqBody, nil)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		data, _ := raw["data"].([]interface{})

		// Collect items for JSON output and table rows simultaneously.
		for _, item := range data {
			if !logsSearchAll && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)

			entry, _ := item.(map[string]interface{})
			attrs, _ := entry["attributes"].(map[string]interface{})

			// Format timestamp
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

			rows = append(rows, logRow{
				Timestamp: ts,
				Host:      stringField(attrs, "host"),
				Service:   stringField(attrs, "service"),
				Status:    stringField(attrs, "status"),
				Message:   stringField(attrs, "message"),
			})
		}

		// Check for next page cursor
		meta, _ := raw["meta"].(map[string]interface{})
		page, _ := meta["page"].(map[string]interface{})
		nextCursor, _ := page["after"].(string)
		if nextCursor == "" || len(data) == 0 {
			break
		}
		cursor = nextCursor
	}

	if opts.JSON {
		// Build a merged response: combine all pages' data arrays, keep metadata
		// from the last response so cursor/count reflects final state.
		merged := map[string]interface{}{
			"data": allData,
		}
		if lastRaw != nil {
			if links, ok := lastRaw["links"]; ok {
				merged["links"] = links
			}
			if metaVal, ok := lastRaw["meta"]; ok {
				merged["meta"] = metaVal
			}
		}
		return output.RenderJSON(merged, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No logs found matching your query.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "Timestamp"},
		{Name: "Host", Width: 25},
		{Name: "Service", Width: 20},
		{Name: "Status"},
		{Name: "Message", Width: 80},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Timestamp, r.Host, r.Service, r.Status, r.Message}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- logs aggregate ----

var (
	logsAggQuery   string
	logsAggFrom    string
	logsAggTo      string
	logsAggCompute string
	logsAggGroupBy string
)

var logsAggregateCmd = &cobra.Command{
	Use:   "aggregate",
	Short: "Aggregate logs by fields",
	Long:  `Aggregate logs using Datadog Log Analytics.`,
	Example: `  # Count logs by service
  datadog-cli logs aggregate -q "service:*" --group-by service --compute count

  # Count errors grouped by host
  datadog-cli logs aggregate -q "status:error" --group-by host --compute count

  # Count all logs from the last day grouped by status
  datadog-cli logs aggregate -q "*" --from 1d --group-by status`,
	RunE: runLogsAggregate,
}

func runLogsAggregate(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(logsAggFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(logsAggTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromMs >= toMs {
		return fmt.Errorf("--from time must be before --to time")
	}

	client := newClient()
	opts := GetOutputOptions()

	reqBody := map[string]interface{}{
		"filter": map[string]interface{}{
			"query":   logsAggQuery,
			"from":    strconv.FormatInt(fromMs, 10),
			"to":      strconv.FormatInt(toMs, 10),
			"indexes": []string{"*"},
		},
		"compute": []map[string]interface{}{
			{
				"aggregation": logsAggCompute,
				"type":        "total",
			},
		},
	}

	if logsAggGroupBy != "" {
		reqBody["group_by"] = []map[string]interface{}{
			{
				"facet": logsAggGroupBy,
				"limit": flagLimit,
				"sort": map[string]interface{}{
					"type":        "measure",
					"aggregation": logsAggCompute,
					"order":       "desc",
				},
			},
		}
	}

	respBytes, err := client.Post("/api/v2/logs/analytics/aggregate", reqBody, nil)
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

	// Extract buckets
	dataField, _ := raw["data"].(map[string]interface{})
	bucketsRaw, _ := dataField["buckets"].([]interface{})

	if len(bucketsRaw) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No logs found for aggregation.")
		return nil
	}

	// Build rows dynamically; collect ordered key set from first bucket
	type aggRow map[string]string
	var aggRows []aggRow
	var keyOrder []string

	for i, b := range bucketsRaw {
		bucket, _ := b.(map[string]interface{})
		row := make(aggRow)

		byMap, _ := bucket["by"].(map[string]interface{})
		computes, _ := bucket["computes"].(map[string]interface{})

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

	// Build columns from key order
	cols := make([]output.ColumnConfig, len(keyOrder))
	for i, k := range keyOrder {
		header := strings.ReplaceAll(k, "_", " ")
		header = cases.Title(language.Und).String(header)
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

// ---- logs indexes ----

var logsIndexesCmd = &cobra.Command{
	Use:   "indexes",
	Short: "List configured log indexes",
	Long:  `List all log indexes configured in your Datadog account.`,
	Example: `  # List all log indexes
  datadog-cli logs indexes

  # List indexes in JSON format
  datadog-cli logs indexes --json`,
	RunE: runLogsIndexes,
}

func runLogsIndexes(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/logs/indexes", nil)
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

	indexList, _ := raw["indexes"].([]interface{})
	if len(indexList) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No log indexes found.")
		return nil
	}

	type indexRow struct {
		Name             string
		NumRetentionDays string
		IsRateLimited    string
	}

	rows := make([]indexRow, 0, len(indexList))
	for _, item := range indexList {
		idx, _ := item.(map[string]interface{})
		name := stringField(idx, "name")
		retention := ""
		if v, ok := idx["num_retention_days"]; ok {
			retention = fmt.Sprintf("%v", v)
		}
		rateLimited := "false"
		if v, ok := idx["is_rate_limited"].(bool); ok && v {
			rateLimited = "true"
		}
		rows = append(rows, indexRow{
			Name:             name,
			NumRetentionDays: retention,
			IsRateLimited:    rateLimited,
		})
	}

	cols := []output.ColumnConfig{
		{Name: "Name"},
		{Name: "Retention Days"},
		{Name: "Rate Limited"},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Name, r.NumRetentionDays, r.IsRateLimited}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

// stringField safely extracts a string value from a map.
func stringField(m map[string]interface{}, key string) string {
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
	// logs search flags
	logsSearchCmd.Flags().StringVarP(&logsSearchQuery, "query", "q", "", "Log search query (required)")
	logsSearchCmd.Flags().StringVar(&logsSearchFrom, "from", "15m", "Start time (e.g. 15m, 1h, 2d, 1w, now, ISO-8601, unix)")
	logsSearchCmd.Flags().StringVar(&logsSearchTo, "to", "now", "End time (e.g. now, ISO-8601, unix)")
	logsSearchCmd.Flags().StringVar(&logsSearchSort, "sort", "-timestamp", "Sort order: 'timestamp' (asc) or '-timestamp' (desc)")
	logsSearchCmd.Flags().StringSliceVar(&logsSearchIndexes, "indexes", nil, "Log indexes to search (default: all)")
	logsSearchCmd.Flags().BoolVar(&logsSearchAll, "all", false, "Fetch all pages until no cursor remains (overrides --limit)")
	_ = logsSearchCmd.MarkFlagRequired("query")

	// logs aggregate flags
	logsAggregateCmd.Flags().StringVarP(&logsAggQuery, "query", "q", "", "Log search query (required)")
	logsAggregateCmd.Flags().StringVar(&logsAggFrom, "from", "15m", "Start time")
	logsAggregateCmd.Flags().StringVar(&logsAggTo, "to", "now", "End time")
	logsAggregateCmd.Flags().StringVar(&logsAggCompute, "compute", "count", "Aggregation type: count, sum, avg, min, max")
	logsAggregateCmd.Flags().StringVar(&logsAggGroupBy, "group-by", "", "Field to group by (e.g. service, host, status)")
	_ = logsAggregateCmd.MarkFlagRequired("query")

	// Add subcommands to logs
	logsCmd.AddCommand(logsSearchCmd)
	logsCmd.AddCommand(logsAggregateCmd)
	logsCmd.AddCommand(logsIndexesCmd)

	// Add logs to root
	rootCmd.AddCommand(logsCmd)
}
