package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- rum command group ----

var rumCmd = &cobra.Command{
	Use:   "rum",
	Short: "Query and aggregate RUM events from Datadog Real User Monitoring",
	Long: `Query RUM (Real User Monitoring) events from Datadog.

RUM events include sessions, views, actions, errors, resources, and long tasks
from frontend applications instrumented with Datadog RUM.`,
	Example: `  # Search for RUM errors in the last hour
  datadog-cli rum search -q "@type:error" --from 1h

  # Aggregate RUM events by type
  datadog-cli rum aggregate -q "*" --group-by @type --compute count`,
}

// ---- rum search ----

var (
	rumSearchQuery string
	rumSearchFrom  string
	rumSearchTo    string
	rumSearchSort  string
)

var rumSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search RUM events matching a query",
	Long: `Search RUM events using Datadog query syntax.

Uses POST /api/v2/rum/events/search. Timestamps are in milliseconds.`,
	Example: `  # Search for errors in a frontend service
  datadog-cli rum search --query "service:my-frontend"

  # Search for RUM errors in the last hour
  datadog-cli rum search -q "@type:error" --from 1h --to now

  # Search for 403 errors from the last day as JSON
  datadog-cli rum search -q "@error.message:*403*" --from 1d --json`,
	RunE: runRumSearch,
}

func runRumSearch(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(rumSearchFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(rumSearchTo)
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

	reqBody := map[string]interface{}{
		"filter": map[string]interface{}{
			"query": rumSearchQuery,
			"from":  strconv.FormatInt(fromMs, 10),
			"to":    strconv.FormatInt(toMs, 10),
		},
		"sort": rumSearchSort,
		"page": map[string]interface{}{
			"limit": pageSize,
		},
	}

	type rumRow struct {
		Timestamp   string
		Application string
		Type        string
		Action      string
		View        string
	}

	var rows []rumRow
	var lastRaw interface{}
	cursor := ""

	for len(rows) < flagLimit {
		if cursor != "" {
			if page, ok := reqBody["page"].(map[string]interface{}); ok {
				page["cursor"] = cursor
			}
		}

		respBytes, err := client.Post("/api/v2/rum/events/search", reqBody, nil)
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
			// RUM v2 double-nests: entry.attributes.attributes contains app/type/action/view
			innerAttrs, _ := attrs["attributes"].(map[string]interface{})
			if innerAttrs == nil {
				innerAttrs = attrs
			}

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

			// Extract application name
			application := ""
			if appMap, ok := innerAttrs["application"].(map[string]interface{}); ok {
				application = stringFieldFromMap(appMap, "name")
			}

			// Extract action type
			action := ""
			if actionMap, ok := innerAttrs["action"].(map[string]interface{}); ok {
				action = stringFieldFromMap(actionMap, "type")
			}

			// Extract view name
			view := ""
			if viewMap, ok := innerAttrs["view"].(map[string]interface{}); ok {
				view = stringFieldFromMap(viewMap, "name")
			}

			rows = append(rows, rumRow{
				Timestamp:   ts,
				Application: application,
				Type:        stringFieldFromMap(innerAttrs, "type"),
				Action:      action,
				View:        output.TruncateString(view, 40),
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
		return output.RenderJSON(lastRaw, opts)
	}

	if len(rows) == 0 {
		fmt.Fprintln(os.Stdout, "No RUM events found matching your query.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "Timestamp"},
		{Name: "Application", Width: 25},
		{Name: "Type", Width: 12},
		{Name: "Action", Width: 15},
		{Name: "View", Width: 40},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Timestamp, r.Application, r.Type, r.Action, r.View}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- rum aggregate ----

var (
	rumAggQuery   string
	rumAggFrom    string
	rumAggTo      string
	rumAggCompute string
	rumAggGroupBy string
)

var rumAggregateCmd = &cobra.Command{
	Use:   "aggregate",
	Short: "Aggregate RUM events by fields",
	Long: `Aggregate RUM events using Datadog RUM Analytics.

Uses POST /api/v2/rum/analytics/aggregate. Timestamps are in milliseconds.`,
	Example: `  # Count RUM events grouped by type
  datadog-cli rum aggregate -q "*" --group-by @type --compute count

  # Count errors by error source
  datadog-cli rum aggregate -q "@type:error" --group-by @error.source --compute count

  # Count events per view in an app from the last day
  datadog-cli rum aggregate -q "service:my-app" --from 1d --group-by @view.name --compute count`,
	RunE: runRumAggregate,
}

func runRumAggregate(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(rumAggFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(rumAggTo)
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
			"query": rumAggQuery,
			"from":  strconv.FormatInt(fromMs, 10),
			"to":    strconv.FormatInt(toMs, 10),
		},
		"compute": []map[string]interface{}{
			{
				"aggregation": rumAggCompute,
				"type":        "total",
			},
		},
	}

	if rumAggGroupBy != "" {
		reqBody["group_by"] = []map[string]interface{}{
			{
				"facet": rumAggGroupBy,
				"limit": flagLimit,
				"sort": map[string]interface{}{
					"type":        "measure",
					"aggregation": rumAggCompute,
					"order":       "desc",
				},
			},
		}
	}

	respBytes, err := client.Post("/api/v2/rum/analytics/aggregate", reqBody, nil)
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

	// Extract buckets — response: {"data": {"buckets": [...]}}
	dataField, _ := raw["data"].(map[string]interface{})
	bucketsRaw, _ := dataField["buckets"].([]interface{})

	if len(bucketsRaw) == 0 {
		fmt.Fprintln(os.Stdout, "No RUM events found for aggregation.")
		return nil
	}

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

// ---- init ----

func init() {
	// rum search flags
	rumSearchCmd.Flags().StringVarP(&rumSearchQuery, "query", "q", "", "RUM search query (required)")
	rumSearchCmd.Flags().StringVar(&rumSearchFrom, "from", "15m", "Start time (e.g. 15m, 1h, 2d, 1w, now, ISO-8601, unix)")
	rumSearchCmd.Flags().StringVar(&rumSearchTo, "to", "now", "End time (e.g. now, ISO-8601, unix)")
	rumSearchCmd.Flags().StringVar(&rumSearchSort, "sort", "-timestamp", "Sort order: 'timestamp' (asc) or '-timestamp' (desc)")
	_ = rumSearchCmd.MarkFlagRequired("query")

	// rum aggregate flags
	rumAggregateCmd.Flags().StringVarP(&rumAggQuery, "query", "q", "", "RUM search query (required)")
	rumAggregateCmd.Flags().StringVar(&rumAggFrom, "from", "15m", "Start time")
	rumAggregateCmd.Flags().StringVar(&rumAggTo, "to", "now", "End time")
	rumAggregateCmd.Flags().StringVar(&rumAggCompute, "compute", "count", "Aggregation type: count, sum, avg, min, max")
	rumAggregateCmd.Flags().StringVar(&rumAggGroupBy, "group-by", "", "Field to group by (e.g. @type, @view.name, @application.name)")
	_ = rumAggregateCmd.MarkFlagRequired("query")

	// Add subcommands to rum
	rumCmd.AddCommand(rumSearchCmd)
	rumCmd.AddCommand(rumAggregateCmd)

	// Add rum to root
	rootCmd.AddCommand(rumCmd)
}
