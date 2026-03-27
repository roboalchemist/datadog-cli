package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- slos command group ----

var slosCmd = &cobra.Command{
	Use:   "slos",
	Short: "Query SLOs (Service Level Objectives) from Datadog",
	Long:  `Query SLOs (Service Level Objectives) from Datadog.`,
	Example: `  # List all SLOs
  datadog-cli slos list

  # Get details for a specific SLO
  datadog-cli slos get abc123def456

  # Get SLO history for the last 30 days
  datadog-cli slos history abc123def456 --from 30d`,
}

// maxSLOsPageSize is the maximum limit accepted by the Datadog SLO list API.
const maxSLOsPageSize = 1000

// ---- slos list ----

var (
	slosListIDs      string
	slosListTagQuery string
	slosListAll      bool
)

var slosListCmd = &cobra.Command{
	Use:   "list",
	Short: "List SLOs",
	Long: `List SLOs from Datadog.

Uses GET /api/v1/slo with offset-based pagination.`,
	Example: `  # List all SLOs
  datadog-cli slos list

  # Fetch every SLO in the account
  datadog-cli slos list --all

  # Filter SLOs by tag
  datadog-cli slos list --tags-query "env:production"

  # List specific SLOs by ID as JSON
  datadog-cli slos list --ids "abc123,def456" --json`,
	RunE: runSLOsList,
}

func runSLOsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if slosListAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// First page size: min(effectiveLimit, maxSLOsPageSize).
	// If unlimited (--all), use max page size.
	pageSize := effectiveLimit
	if pageSize == 0 || pageSize > maxSLOsPageSize {
		pageSize = maxSLOsPageSize
	}

	type sloRow struct {
		ID     string
		Name   string
		Type   string
		Target string
		Status string
	}

	var rows []sloRow
	var allData []interface{}
	var lastRaw map[string]interface{}
	offset := 0
	pageNum := 0

	for {
		pageNum++

		// Progress output on pages after the first.
		if pageNum > 1 && !flagQuiet {
			if slosListAll {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d SLOs so far)...\n", pageNum, len(rows))
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
			}
		}

		// Adjust page size for the last partial page when limit is set.
		fetchSize := pageSize
		if !slosListAll && effectiveLimit > 0 {
			remaining := effectiveLimit - len(rows)
			if remaining <= 0 {
				break
			}
			if remaining < fetchSize {
				fetchSize = remaining
			}
		}

		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", fetchSize))
		params.Set("offset", fmt.Sprintf("%d", offset))

		if slosListIDs != "" {
			params.Set("ids", slosListIDs)
		}
		if slosListTagQuery != "" {
			params.Set("tags_query", slosListTagQuery)
		}

		respBytes, err := client.Get("/api/v1/slo", params)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		pageData, _ := raw["data"].([]interface{})
		if len(pageData) == 0 {
			break
		}

		for _, item := range pageData {
			if !slosListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)

			s, _ := item.(map[string]interface{})
			id := slosStringField(s, "id")
			name := output.TruncateString(slosStringField(s, "name"), 40)
			sloType := slosStringField(s, "type")
			target := slosExtractTarget(s)
			status := slosExtractStatus(s)
			rows = append(rows, sloRow{ID: id, Name: name, Type: sloType, Target: target, Status: status})
		}

		// Check if there are more pages via metadata.pagination.total_count.
		totalCount := -1
		if meta, ok := raw["metadata"].(map[string]interface{}); ok {
			if pg, ok := meta["pagination"].(map[string]interface{}); ok {
				if tc, ok := pg["total_count"].(float64); ok {
					totalCount = int(tc)
				}
			}
		}

		offset += len(pageData)

		// Stop if we've fetched everything or reached the effective limit.
		if !slosListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
			break
		}
		if totalCount >= 0 && offset >= totalCount {
			break
		}
		if len(pageData) < fetchSize {
			// Received fewer items than requested — no more pages.
			break
		}
	}

	if opts.JSON {
		merged := map[string]interface{}{
			"data": allData,
		}
		if lastRaw != nil {
			if meta, ok := lastRaw["metadata"]; ok {
				merged["metadata"] = meta
			}
		}
		return output.RenderJSON(merged, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No SLOs found.")
		return nil
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{r.ID, r.Name, r.Type, r.Target, r.Status})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 36},
		{Name: "Name", Width: 40},
		{Name: "Type", Width: 12},
		{Name: "Target", Width: 10},
		{Name: "Status", Width: 12},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- slos get ----

var slosGetCmd = &cobra.Command{
	Use:   "get <slo_id>",
	Short: "Get SLO details by ID",
	Long: `Get detailed information about a specific SLO.

Uses GET /api/v1/slo/{id}.`,
	Example: `  # Get details for a specific SLO
  datadog-cli slos get abc123def456

  # Get SLO details in JSON format
  datadog-cli slos get abc123def456 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runSLOsGet,
}

func runSLOsGet(cmd *cobra.Command, args []string) error {
	sloID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/slo/"+sloID, nil)
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

	slo, _ := raw["data"].(map[string]interface{})
	if slo == nil {
		slo = raw
	}

	name := slosStringField(slo, "name")
	_, _ = fmt.Fprintf(os.Stdout, "SLO: %s\n\n", name)

	// Extract thresholds
	target := slosExtractTarget(slo)
	timeframe := ""
	if thresholds, ok := slo["thresholds"].([]interface{}); ok && len(thresholds) > 0 {
		if t, ok := thresholds[0].(map[string]interface{}); ok {
			timeframe = slosStringField(t, "timeframe")
		}
	}

	// Extract current SLI
	sli := ""
	if overallStatus, ok := slo["overall_status"].([]interface{}); ok && len(overallStatus) > 0 {
		if s, ok := overallStatus[0].(map[string]interface{}); ok {
			if v, ok := s["sli"].(float64); ok {
				sli = fmt.Sprintf("%.4f%%", v)
			}
		}
	}

	tags := ""
	if tagsRaw, ok := slo["tags"].([]interface{}); ok {
		tagStrs := make([]string, 0, len(tagsRaw))
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				tagStrs = append(tagStrs, s)
			}
		}
		tags = strings.Join(tagStrs, ", ")
	}

	creator := ""
	if creatorMap, ok := slo["creator"].(map[string]interface{}); ok {
		creator = slosStringField(creatorMap, "email")
	}

	details := []struct{ k, v string }{
		{"ID", slosStringField(slo, "id")},
		{"Name", name},
		{"Type", slosStringField(slo, "type")},
		{"Target", target},
		{"Timeframe", timeframe},
		{"Current SLI", sli},
		{"Description", output.TruncateString(slosStringField(slo, "description"), 80)},
		{"Tags", tags},
		{"Creator", creator},
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
		{Name: "Field", Width: 20},
		{Name: "Value", Width: 80},
	}

	return output.RenderTable(cols, tableRows, detailRows, opts)
}

// ---- slos history ----

var (
	slosHistoryFrom string
	slosHistoryTo   string
)

var slosHistoryCmd = &cobra.Command{
	Use:   "history <slo_id>",
	Short: "Get SLO history",
	Long: `Get historical SLI values for an SLO over a time range.

Uses GET /api/v1/slo/{id}/history with from_ts and to_ts in seconds.`,
	Example: `  # Get default (7 day) SLO history
  datadog-cli slos history abc123def456

  # Get 30 days of SLO history
  datadog-cli slos history abc123def456 --from 30d

  # Get SLO history in JSON format
  datadog-cli slos history abc123def456 --from 7d --json`,
	Args: cobra.ExactArgs(1),
	RunE: runSLOsHistory,
}

func runSLOsHistory(cmd *cobra.Command, args []string) error {
	sloID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	now := time.Now().Unix()

	fromSecs, err := slosParseTimeSecs(slosHistoryFrom, now-(7*86400), now)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toSecs, err := slosParseTimeSecs(slosHistoryTo, now, now)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}

	params := url.Values{}
	params.Set("from_ts", fmt.Sprintf("%d", fromSecs))
	params.Set("to_ts", fmt.Sprintf("%d", toSecs))

	respBytes, err := client.Get("/api/v1/slo/"+sloID+"/history", params)
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

	historyData, _ := raw["data"].(map[string]interface{})
	if historyData == nil {
		_, _ = fmt.Fprintln(os.Stdout, "No history data found.")
		return nil
	}

	// Extract overall.sli_history.history list
	var historyList []interface{}
	if overall, ok := historyData["overall"].(map[string]interface{}); ok {
		if sliHistory, ok := overall["sli_history"].(map[string]interface{}); ok {
			historyList, _ = sliHistory["history"].([]interface{})
		}
	}

	if len(historyList) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No history entries found for the specified time range.")
		return nil
	}

	type histRow struct {
		Timestamp string
		SLI       string
		Uptime    string
	}

	rows := make([]histRow, 0, len(historyList))
	tableRows := make([][]string, 0, len(historyList))

	for _, entry := range historyList {
		e, _ := entry.([]interface{})
		if len(e) < 2 {
			continue
		}
		ts := ""
		if tsNum, ok := e[0].(float64); ok {
			t := time.Unix(int64(tsNum), 0).UTC()
			ts = t.Format("2006-01-02 15:04")
		}
		sliVal := ""
		if v, ok := e[1].(float64); ok {
			sliVal = fmt.Sprintf("%.4f%%", v)
		}

		rows = append(rows, histRow{Timestamp: ts, SLI: sliVal})
		tableRows = append(tableRows, []string{ts, sliVal})
	}

	cols := []output.ColumnConfig{
		{Name: "Timestamp", Width: 18},
		{Name: "SLI", Width: 12},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

func slosStringField(m map[string]interface{}, key string) string {
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

func slosExtractTarget(slo map[string]interface{}) string {
	thresholds, ok := slo["thresholds"].([]interface{})
	if !ok || len(thresholds) == 0 {
		return ""
	}
	t, ok := thresholds[0].(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := t["target"].(float64); ok {
		return fmt.Sprintf("%.2f%%", v)
	}
	return ""
}

func slosExtractStatus(slo map[string]interface{}) string {
	overallStatus, ok := slo["overall_status"].([]interface{})
	if !ok || len(overallStatus) == 0 {
		return ""
	}
	s, ok := overallStatus[0].(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := s["sli"].(float64); ok {
		return fmt.Sprintf("%.2f%%", v)
	}
	return ""
}

// slosParseTimeSecs parses a time string to Unix seconds.
// Supports: "now", relative "7d"/"30d", or plain integer seconds.
func slosParseTimeSecs(s string, defaultVal, now int64) (int64, error) {
	if s == "" {
		return defaultVal, nil
	}
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "now" {
		return now, nil
	}
	// Relative: Nd
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err == nil {
			return now - n*86400, nil
		}
	}
	// Relative: Nh
	if strings.HasSuffix(s, "h") {
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err == nil {
			return now - n*3600, nil
		}
	}
	// Plain integer (seconds)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("invalid time %q: use 'now', relative like '7d', or Unix seconds", s)
}

// ---- init ----

func init() {
	slosListCmd.Flags().StringVar(&slosListIDs, "ids", "", "Comma-separated list of SLO IDs to filter")
	slosListCmd.Flags().StringVar(&slosListTagQuery, "tags-query", "", "Filter by tags (e.g. 'env:production')")
	slosListCmd.Flags().BoolVar(&slosListAll, "all", false, "Fetch all pages until no more results (overrides --limit)")

	slosHistoryCmd.Flags().StringVar(&slosHistoryFrom, "from", "7d", "Start of history window (e.g. '7d', '30d', or Unix seconds)")
	slosHistoryCmd.Flags().StringVar(&slosHistoryTo, "to", "now", "End of history window ('now' or Unix seconds)")

	slosCmd.AddCommand(slosListCmd)
	slosCmd.AddCommand(slosGetCmd)
	slosCmd.AddCommand(slosHistoryCmd)

	rootCmd.AddCommand(slosCmd)
}
