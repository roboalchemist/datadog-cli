package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- audit command group ----

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Search Datadog Audit Logs for organization activity",
	Long: `Search Datadog Audit Logs for organization activity.

Audit logs record actions taken within your Datadog organization such as:
  - User logins and authentication events
  - Dashboard, monitor, and notebook changes
  - API key management
  - Permission and role changes
  - Integration configuration

Required scope: audit_logs_read`,
	Example: `  # Search all audit events from the last hour
  datadog-cli audit search -q "*" --from 1h

  # Search for actions by a specific user
  datadog-cli audit search -q "@usr.email:admin@example.com"`,
}

// maxAuditPageSize is the maximum page limit accepted by the Datadog audit events search API.
const maxAuditPageSize = 1000

// ---- audit search ----

var (
	auditSearchQuery string
	auditSearchFrom  string
	auditSearchTo    string
	auditSearchSort  string
	auditSearchAll   bool
)

var auditSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search Datadog Audit Logs",
	Long: `Search Datadog Audit Logs using query syntax.

Uses POST /api/v2/audit/events/search. Timestamps are in milliseconds.`,
	Example: `  # Search all audit events from the last hour
  datadog-cli audit search --query "*" --from 1h

  # Search for actions by a specific user
  datadog-cli audit search -q "@usr.email:admin@example.com"

  # Fetch all dashboard audit events across all pages
  datadog-cli audit search -q "dashboard" --from 1d --all

  # Search for dashboard-related events as JSON
  datadog-cli audit search -q "dashboard" --from 1d --json`,
	RunE: runAuditSearch,
}

func runAuditSearch(cmd *cobra.Command, args []string) error {
	fromMs, err := parseTime(auditSearchFrom)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toMs, err := parseTime(auditSearchTo)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if fromMs >= toMs {
		return fmt.Errorf("--from time must be before --to time")
	}

	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if auditSearchAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// First page size: min(effectiveLimit, maxAuditPageSize).
	firstPageSize := effectiveLimit
	if firstPageSize == 0 || firstPageSize > maxAuditPageSize {
		firstPageSize = maxAuditPageSize
	}

	reqBody := map[string]interface{}{
		"filter": map[string]interface{}{
			"query": auditSearchQuery,
			"from":  strconv.FormatInt(fromMs, 10),
			"to":    strconv.FormatInt(toMs, 10),
		},
		"sort": auditSearchSort,
		"page": map[string]interface{}{
			"limit": firstPageSize,
		},
	}

	type auditRow struct {
		Timestamp string
		Type      string
		Action    string
		User      string
		Service   string
	}

	var rows []auditRow
	var allData []interface{}
	var lastRaw map[string]interface{}
	cursor := ""
	pageNum := 0

	needsMorePages := func() bool {
		if auditSearchAll {
			return true // stop only when cursor is gone
		}
		return len(rows) < effectiveLimit
	}

	for needsMorePages() {
		pageNum++

		if cursor != "" {
			if pageMap, ok := reqBody["page"].(map[string]interface{}); ok {
				pageMap["cursor"] = cursor
				// Adjust page size for the last page when limit is set.
				if !auditSearchAll {
					remaining := effectiveLimit - len(rows)
					ps := remaining
					if ps > maxAuditPageSize {
						ps = maxAuditPageSize
					}
					pageMap["limit"] = ps
				}
			}
			// Print progress to stderr (unless --quiet).
			if !flagQuiet {
				if auditSearchAll {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d events so far)...\n", pageNum, len(rows))
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
				}
			}
		}

		respBytes, err := client.Post("/api/v2/audit/events/search", reqBody, nil)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		data, _ := raw["data"].([]interface{})
		for _, item := range data {
			if !auditSearchAll && len(rows) >= effectiveLimit {
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

			// Extract nested attributes (evt, usr)
			innerAttrs, _ := attrs["attributes"].(map[string]interface{})

			// Extract event type/action
			evtType := ""
			action := ""
			if evt, ok := innerAttrs["evt"].(map[string]interface{}); ok {
				evtType = stringFieldFromMap(evt, "category")
				action = stringFieldFromMap(evt, "name")
			}

			// Extract user email
			user := ""
			if usr, ok := innerAttrs["usr"].(map[string]interface{}); ok {
				user = output.TruncateString(stringFieldFromMap(usr, "email"), 30)
			}

			rows = append(rows, auditRow{
				Timestamp: ts,
				Type:      evtType,
				Action:    output.TruncateString(action, 25),
				User:      user,
				Service:   stringFieldFromMap(attrs, "service"),
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
		_, _ = fmt.Fprintln(os.Stdout, "No audit events found matching your query.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "Timestamp"},
		{Name: "Type", Width: 20},
		{Name: "Action", Width: 25},
		{Name: "User", Width: 30},
		{Name: "Service", Width: 20},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Timestamp, r.Type, r.Action, r.User, r.Service}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- init ----

func init() {
	// audit search flags
	auditSearchCmd.Flags().StringVarP(&auditSearchQuery, "query", "q", "", "Audit log search query (required)")
	auditSearchCmd.Flags().StringVar(&auditSearchFrom, "from", "15m", "Start time (e.g. 15m, 1h, 2d, 1w, now, ISO-8601, unix)")
	auditSearchCmd.Flags().StringVar(&auditSearchTo, "to", "now", "End time (e.g. now, ISO-8601, unix)")
	auditSearchCmd.Flags().StringVar(&auditSearchSort, "sort", "-timestamp", "Sort order: 'timestamp' (asc) or '-timestamp' (desc)")
	auditSearchCmd.Flags().BoolVar(&auditSearchAll, "all", false, "Fetch all pages until no cursor remains (overrides --limit)")
	_ = auditSearchCmd.MarkFlagRequired("query")

	// Add subcommands to audit
	auditCmd.AddCommand(auditSearchCmd)

	// Add audit to root
	rootCmd.AddCommand(auditCmd)
}
