package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- hosts command group ----

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Query infrastructure hosts from Datadog",
	Long:  `Query infrastructure hosts from Datadog.`,
	Example: `  # List all hosts
  datadog-cli hosts list

  # Filter hosts by environment tag
  datadog-cli hosts list --filter "env:production"

  # Show total host counts
  datadog-cli hosts totals`,
}

// maxHostsPageSize is the maximum count accepted by the Datadog hosts list API.
const maxHostsPageSize = 1000

// ---- hosts list ----

var (
	hostsListFilter    string
	hostsListSortField string
	hostsListSortDir   string
	hostsListCount     int
	hostsListStart     int
	hostsListAll       bool
)

var hostsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List infrastructure hosts",
	Long: `List infrastructure hosts from Datadog.

Uses GET /api/v1/hosts with automatic offset-based pagination.
The API count cap is 1000 per page; use --all to fetch every host.`,
	Example: `  # List all hosts
  datadog-cli hosts list

  # Fetch every host regardless of count
  datadog-cli hosts list --all

  # Filter hosts by environment tag
  datadog-cli hosts list --filter "env:production"

  # Sort by CPU usage descending and output as JSON
  datadog-cli hosts list --sort-field cpu --sort-dir desc --json`,
	RunE: runHostsList,
}

func runHostsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	if hostsListSortDir != "" && hostsListSortDir != "asc" && hostsListSortDir != "desc" {
		return fmt.Errorf("--sort-dir must be 'asc' or 'desc', got %q", hostsListSortDir)
	}

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if hostsListAll {
		effectiveLimit = 0 // 0 = unlimited
	}
	// --count overrides --limit for page size (legacy flag kept for compatibility).
	if hostsListCount > 0 {
		if !hostsListAll {
			effectiveLimit = hostsListCount
		}
	}

	// Page size is min(effectiveLimit, maxHostsPageSize); if unlimited use max.
	pageSize := effectiveLimit
	if pageSize == 0 || pageSize > maxHostsPageSize {
		pageSize = maxHostsPageSize
	}

	type hostRow struct {
		HostName     string
		OS           string
		Platform     string
		CPUCores     string
		Up           string
		LastReported string
	}

	var rows []hostRow
	var allHostList []interface{}
	// Start offset: honour --start for the first page (manual offset override).
	start := hostsListStart
	pageNum := 0

	for {
		params := url.Values{}
		params.Set("count", fmt.Sprintf("%d", pageSize))
		params.Set("start", fmt.Sprintf("%d", start))
		params.Set("include_hosts_metadata", "true")

		if hostsListFilter != "" {
			params.Set("filter", hostsListFilter)
		}
		if hostsListSortField != "" {
			params.Set("sort_field", hostsListSortField)
		}
		if hostsListSortDir != "" {
			params.Set("sort_dir", hostsListSortDir)
		}

		if pageNum > 0 && !flagQuiet {
			if hostsListAll {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (start=%d, %d hosts so far)...\n", pageNum+1, start, len(rows))
			} else {
				_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (start=%d, %d/%d)...\n", pageNum+1, start, len(rows), effectiveLimit)
			}
		}

		respBytes, err := client.Get("/api/v1/hosts", params)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		hostList, _ := raw["host_list"].([]interface{})

		for _, item := range hostList {
			if !hostsListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit {
				break
			}
			allHostList = append(allHostList, item)
			host, _ := item.(map[string]interface{})

			name := stringField(host, "name")
			if name == "" {
				name = stringField(host, "host_name")
			}

			upStr := "unknown"
			if isUp, ok := host["up"].(bool); ok {
				if isUp {
					upStr = "UP"
				} else {
					upStr = "DOWN"
				}
			}

			lastReported := ""
			if lr, ok := host["last_reported_time"]; ok {
				lastReported = hostsFormatTimestamp(lr)
			}

			osName := ""
			platform := ""
			cpuCores := ""
			if meta, ok := host["meta"].(map[string]interface{}); ok {
				if gohaiStr := stringField(meta, "gohai"); gohaiStr != "" {
					var gohai map[string]interface{}
					if err := json.Unmarshal([]byte(gohaiStr), &gohai); err == nil {
						if platformMap, ok := gohai["platform"].(map[string]interface{}); ok {
							osName = stringField(platformMap, "os")
						}
					}
				}
				if osName == "" {
					osName = stringField(meta, "platform")
				}
				platform = stringField(meta, "platform")
				if v, ok := meta["cpuCores"]; ok {
					cpuCores = fmt.Sprintf("%v", v)
				}
			}
			if osName == "" {
				osName = stringField(host, "os")
			}

			rows = append(rows, hostRow{
				HostName:     name,
				OS:           osName,
				Platform:     platform,
				CPUCores:     cpuCores,
				Up:           upStr,
				LastReported: lastReported,
			})
		}

		// Stop if we have enough, the page was short (last page), or empty.
		if (!hostsListAll && effectiveLimit > 0 && len(rows) >= effectiveLimit) ||
			len(hostList) < pageSize || len(hostList) == 0 {
			break
		}

		start += len(hostList)
		pageNum++
	}

	if opts.JSON {
		return output.RenderJSON(allHostList, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No hosts found.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "Host Name", Width: 50},
		{Name: "OS", Width: 15},
		{Name: "Platform", Width: 15},
		{Name: "CPU Cores"},
		{Name: "Up"},
		{Name: "Last Reported"},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.HostName, r.OS, r.Platform, r.CPUCores, r.Up, r.LastReported}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// hostsFormatTimestamp formats a unix timestamp from various numeric types.
func hostsFormatTimestamp(ts interface{}) string {
	var secs int64
	switch v := ts.(type) {
	case float64:
		secs = int64(v)
	case int64:
		secs = v
	case int:
		secs = int64(v)
	default:
		return fmt.Sprintf("%v", ts)
	}
	if secs == 0 {
		return ""
	}
	return time.Unix(secs, 0).UTC().Format("2006-01-02 15:04:05")
}

// ---- hosts totals ----

var hostsTotalsCmd = &cobra.Command{
	Use:   "totals",
	Short: "Show total and active host counts",
	Long: `Show total and active host counts from Datadog.

Uses GET /api/v1/hosts/totals.`,
	Example: `  # Show host totals
  datadog-cli hosts totals

  # Show host totals in JSON format
  datadog-cli hosts totals --json`,
	RunE: runHostsTotals,
}

func runHostsTotals(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v1/hosts/totals", nil)
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

	totalActive := ""
	totalUp := ""
	if v, ok := raw["total_active"]; ok {
		totalActive = fmt.Sprintf("%v", v)
	}
	if v, ok := raw["total_up"]; ok {
		totalUp = fmt.Sprintf("%v", v)
	}

	cols := []output.ColumnConfig{
		{Name: "Total Active"},
		{Name: "Total Up"},
	}

	tableRows := [][]string{
		{totalActive, totalUp},
	}

	type totalsRow struct {
		TotalActive string
		TotalUp     string
	}
	data := []totalsRow{{TotalActive: totalActive, TotalUp: totalUp}}

	return output.RenderTable(cols, tableRows, data, opts)
}

// ---- init ----

func init() {
	// hosts list flags
	hostsListCmd.Flags().StringVar(&hostsListFilter, "filter", "", "Filter hosts by name, alias, or tag (e.g. 'env:production')")
	hostsListCmd.Flags().StringVar(&hostsListSortField, "sort-field", "", "Field to sort by (e.g. 'cpu', 'name')")
	hostsListCmd.Flags().StringVar(&hostsListSortDir, "sort-dir", "", "Sort direction: 'asc' or 'desc'")
	hostsListCmd.Flags().IntVar(&hostsListCount, "count", 0, "Number of hosts to return per page (default: --limit value, max 1000)")
	hostsListCmd.Flags().IntVar(&hostsListStart, "start", 0, "Starting offset for first page")
	hostsListCmd.Flags().BoolVar(&hostsListAll, "all", false, "Fetch all pages until no more hosts remain (overrides --limit)")

	// Wire up subcommands
	hostsCmd.AddCommand(hostsListCmd)
	hostsCmd.AddCommand(hostsTotalsCmd)

	// Register with root
	rootCmd.AddCommand(hostsCmd)
}
