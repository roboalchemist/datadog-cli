package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
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

// ---- logs tail ----

var (
	logsTailQuery    string
	logsTailFrom     string
	logsTailInterval string
)

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream logs in real-time (polling-based)",
	Long: `Stream logs in real-time by polling Datadog for new entries.

Polls the Datadog logs search API at a configurable interval and prints
new log lines as they arrive, similar to 'kubectl logs -f'. Press Ctrl+C
to stop.

Output format (one line per log):
  TIMESTAMP  HOST  SERVICE  STATUS  MESSAGE`,
	Example: `  # Tail all logs for a service
  datadog-cli logs tail -q "service:vcerberus"

  # Tail errors in prod, starting from 5 minutes ago
  datadog-cli logs tail -q "service:vcerberus status:error" --from 5m

  # Tail with a faster poll interval
  datadog-cli logs tail -q "service:api-gateway" --interval 1s

  # Pipe to grep for keyword filtering
  datadog-cli logs tail -q "service:vcerberus" --plaintext | grep "lockdown"`,
	RunE: runLogsTail,
}

// colorForStatus returns an ANSI-colored version of a log status string.
// It reuses the color palette from output.ColorStatus but handles the extra
// Datadog-specific status values (info, debug, trace, notice, emergency).
func colorForStatus(status string, noColor bool) string {
	if noColor || color.NoColor {
		return status
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "critical", "emergency", "alert":
		return color.RedString(status)
	case "warn", "warning":
		return color.YellowString(status)
	case "info", "notice", "ok":
		return color.GreenString(status)
	case "debug", "trace":
		return color.CyanString(status)
	default:
		return status
	}
}

func runLogsTail(cmd *cobra.Command, args []string) error {
	// Parse interval duration.
	intervalDur, err := time.ParseDuration(logsTailInterval)
	if err != nil {
		return fmt.Errorf("--interval: invalid duration %q (use Go duration syntax: 1s, 2s, 3s)", logsTailInterval)
	}
	if intervalDur < 500*time.Millisecond {
		return fmt.Errorf("--interval: minimum allowed value is 500ms")
	}

	// Parse the initial start time (--from sets the beginning of the first window).
	startMs, err := parseTime(logsTailFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}

	client := newClient()
	opts := GetOutputOptions()

	// Set up Ctrl+C / SIGTERM handler.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// lastSeenMs tracks the newest timestamp we've already printed. We use
	// this to advance the search window each poll so we never miss logs even
	// if a single batch is cut short by --limit.
	lastSeenMs := startMs

	// Track IDs seen in the most-recent batch to de-duplicate log entries
	// that straddle a poll boundary (their timestamps are at the edge of
	// the last window, so the next window includes them again).
	seenIDs := map[string]bool{}

	if !flagQuiet {
		_, _ = fmt.Fprintf(os.Stderr, "Tailing logs (query: %q, interval: %s) — press Ctrl+C to stop\n",
			logsTailQuery, intervalDur)
	}

	ticker := time.NewTicker(intervalDur)
	defer ticker.Stop()

	// Run one poll immediately, then on each tick.
	for {
		toMs := time.Now().UnixMilli()

		// Build request — always sort ascending so we print in chronological order.
		pageSize := flagLimit
		if pageSize <= 0 || pageSize > maxLogsPageSize {
			pageSize = maxLogsPageSize
		}

		reqBody := map[string]interface{}{
			"filter": map[string]interface{}{
				"query": logsTailQuery,
				"from":  strconv.FormatInt(lastSeenMs, 10),
				"to":    strconv.FormatInt(toMs, 10),
			},
			"sort": "timestamp", // ascending — oldest first
			"page": map[string]interface{}{
				"limit": pageSize,
			},
		}

		respBytes, apiErr := client.Post("/api/v2/logs/events/search", reqBody, nil)
		if apiErr != nil {
			// Print the error to stderr but keep polling — transient failures
			// (network blip, rate limit) should not kill the tail session.
			_, _ = fmt.Fprintf(os.Stderr, "poll error: %v\n", apiErr)
		} else {
			var raw map[string]interface{}
			if jsonErr := json.Unmarshal(respBytes, &raw); jsonErr != nil {
				_, _ = fmt.Fprintf(os.Stderr, "parse error: %v\n", jsonErr)
			} else {
				data, _ := raw["data"].([]interface{})
				newSeenIDs := map[string]bool{}

				for _, item := range data {
					entry, _ := item.(map[string]interface{})
					id, _ := entry["id"].(string)

					// Skip entries we already printed.
					if id != "" && seenIDs[id] {
						newSeenIDs[id] = true
						continue
					}

					attrs, _ := entry["attributes"].(map[string]interface{})

					// Parse timestamp.
					tsStr := ""
					tsMs := int64(0)
					if tsRaw, ok := attrs["timestamp"].(string); ok && tsRaw != "" {
						if t, err := time.Parse(time.RFC3339Nano, tsRaw); err == nil {
							tsMs = t.UnixMilli()
							tsStr = t.Local().Format("2006-01-02 15:04:05")
						} else if t, err := time.Parse(time.RFC3339, tsRaw); err == nil {
							tsMs = t.UnixMilli()
							tsStr = t.Local().Format("2006-01-02 15:04:05")
						} else {
							tsStr = tsRaw
						}
					}

					host := stringField(attrs, "host")
					service := stringField(attrs, "service")
					status := stringField(attrs, "status")
					message := stringField(attrs, "message")

					// Print the log line.
					if opts.Plaintext {
						_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\t%s\n",
							tsStr, host, service, status, message)
					} else {
						coloredStatus := colorForStatus(status, opts.NoColor)
						_, _ = fmt.Fprintf(os.Stdout, "%s  %-20s  %-20s  %-8s  %s\n",
							tsStr, host, service, coloredStatus, message)
					}

					// Advance the watermark.
					if tsMs > lastSeenMs {
						lastSeenMs = tsMs
					}

					if id != "" {
						newSeenIDs[id] = true
					}
				}

				// Replace seenIDs with the IDs from this batch (keeps set small).
				seenIDs = newSeenIDs
			}
		}

		// Wait for the next tick or a termination signal.
		select {
		case <-sigCh:
			if !flagQuiet {
				_, _ = fmt.Fprintln(os.Stderr, "\nStopped.")
			}
			return nil
		case <-ticker.C:
			// continue polling
		}
	}
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

	// logs tail flags
	logsTailCmd.Flags().StringVarP(&logsTailQuery, "query", "q", "", "Log search query (required)")
	logsTailCmd.Flags().StringVar(&logsTailFrom, "from", "1m", "How far back to start tailing (e.g. 1m, 5m, 1h)")
	logsTailCmd.Flags().StringVar(&logsTailInterval, "interval", "3s", "Poll interval (e.g. 1s, 2s, 5s)")
	_ = logsTailCmd.MarkFlagRequired("query")

	// Add subcommands to logs
	logsCmd.AddCommand(logsSearchCmd)
	logsCmd.AddCommand(logsAggregateCmd)
	logsCmd.AddCommand(logsIndexesCmd)
	logsCmd.AddCommand(logsTailCmd)

	// Add logs to root
	rootCmd.AddCommand(logsCmd)
}
