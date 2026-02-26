package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- hosts command group ----

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Query infrastructure hosts from Datadog",
	Long: `Query infrastructure hosts from Datadog.

Subcommands:
  list    List infrastructure hosts
  totals  Show total and active host counts`,
}

// ---- hosts list ----

var (
	hostsListFilter    string
	hostsListSortField string
	hostsListSortDir   string
	hostsListCount     int
	hostsListStart     int
)

var hostsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List infrastructure hosts",
	Long: `List infrastructure hosts from Datadog.

Uses GET /api/v1/hosts.

Examples:
  datadog-cli hosts list
  datadog-cli hosts list --filter "env:production"
  datadog-cli hosts list --sort-field cpu --sort-dir desc
  datadog-cli hosts list --count 50 --start 0
  datadog-cli hosts list --json`,
	RunE: runHostsList,
}

func runHostsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}

	if hostsListFilter != "" {
		params.Set("filter", hostsListFilter)
	}
	if hostsListSortField != "" {
		params.Set("sort_field", hostsListSortField)
	}
	if hostsListSortDir != "" {
		if hostsListSortDir != "asc" && hostsListSortDir != "desc" {
			return fmt.Errorf("--sort-dir must be 'asc' or 'desc', got %q", hostsListSortDir)
		}
		params.Set("sort_dir", hostsListSortDir)
	}
	if hostsListCount > 0 {
		params.Set("count", fmt.Sprintf("%d", hostsListCount))
	} else {
		params.Set("count", fmt.Sprintf("%d", flagLimit))
	}
	if hostsListStart > 0 {
		params.Set("start", fmt.Sprintf("%d", hostsListStart))
	}

	// Include metadata so we get platform/CPU cores
	params.Set("include_hosts_metadata", "true")

	respBytes, err := client.Get("/api/v1/hosts", params)
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

	hostList, _ := raw["host_list"].([]interface{})
	if len(hostList) == 0 {
		fmt.Fprintln(os.Stdout, "No hosts found.")
		return nil
	}

	type hostRow struct {
		HostName     string
		OS           string
		Platform     string
		CPUCores     string
		Up           string
		LastReported string
	}

	rows := make([]hostRow, 0, len(hostList))
	for _, item := range hostList {
		host, _ := item.(map[string]interface{})

		name := stringField(host, "name")
		if name == "" {
			name = stringField(host, "host_name")
		}

		// Up status
		upStr := "unknown"
		if isUp, ok := host["is_up"].(bool); ok {
			if isUp {
				upStr = "UP"
			} else {
				upStr = "DOWN"
			}
		}

		// Last reported time
		lastReported := ""
		if lr, ok := host["last_reported_time"]; ok {
			lastReported = hostsFormatTimestamp(lr)
		}

		// OS and platform from meta
		osName := ""
		platform := ""
		cpuCores := ""
		if meta, ok := host["meta"].(map[string]interface{}); ok {
			osName = stringField(meta, "gohai")
			if osName == "" {
				osName = stringField(meta, "os")
			}
			platform = stringField(meta, "platform")
			if v, ok := meta["cpuCores"]; ok {
				cpuCores = fmt.Sprintf("%v", v)
			}
		}

		// Fall back to top-level os field
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

Uses GET /api/v1/hosts/totals.

Examples:
  datadog-cli hosts totals
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
	hostsListCmd.Flags().IntVar(&hostsListCount, "count", 0, "Number of hosts to return (default: --limit value)")
	hostsListCmd.Flags().IntVar(&hostsListStart, "start", 0, "Starting offset for pagination")

	// Wire up subcommands
	hostsCmd.AddCommand(hostsListCmd)
	hostsCmd.AddCommand(hostsTotalsCmd)

	// Register with root
	rootCmd.AddCommand(hostsCmd)
}
