package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// maxProcessesPageSize is the maximum page[limit] accepted by the Datadog v2 processes API.
const maxProcessesPageSize = 1000

// ---- processes command group ----

var processesCmd = &cobra.Command{
	Use:   "processes",
	Short: "Query live process information from Datadog",
	Long: `Query live process information from Datadog.

Uses the Datadog v2 Processes API to list running processes
across your infrastructure.`,
	Example: `  # List all running processes
  datadog-cli processes list

  # Search for Python processes on a specific host
  datadog-cli processes list --search "python" --host "web-server-01"`,
}

// ---- processes list ----

var (
	processesListSearch string
	processesListHost   string
	processesListAll    bool
)

var processesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List processes",
	Long: `List live processes from Datadog using the v2 processes API.

Uses GET /api/v2/processes.`,
	Example: `  # List all running processes
  datadog-cli processes list

  # Search for Python processes
  datadog-cli processes list --search "python"

  # Fetch all Python processes across all pages
  datadog-cli processes list --search "python" --all

  # Search for nginx processes on a specific host and output as JSON
  datadog-cli processes list --search "nginx" --host "web-server-01" --json`,
	RunE: runProcessesList,
}

func runProcessesList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if processesListAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// First page size: min(effectiveLimit, maxProcessesPageSize).
	firstPageSize := effectiveLimit
	if firstPageSize == 0 || firstPageSize > maxProcessesPageSize {
		firstPageSize = maxProcessesPageSize
	}

	type processRow struct {
		PID    string
		Name   string
		User   string
		CPU    string
		Memory string
		Host   string
	}

	var rows []processRow
	var allData []interface{}
	var lastRaw map[string]interface{}
	cursor := ""
	pageNum := 0

	needsMorePages := func() bool {
		if processesListAll {
			return true // stop only when cursor is gone
		}
		return len(rows) < effectiveLimit
	}

	for needsMorePages() {
		pageNum++

		pageSize := firstPageSize
		if cursor != "" && !processesListAll {
			remaining := effectiveLimit - len(rows)
			if remaining < pageSize {
				pageSize = remaining
			}
		}

		params := url.Values{}
		params.Set("page[limit]", strconv.Itoa(pageSize))
		if cursor != "" {
			params.Set("page[cursor]", cursor)
			// Print progress to stderr (unless --quiet).
			if !flagQuiet {
				if processesListAll {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d processes so far)...\n", pageNum, len(rows))
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
				}
			}
		}

		if processesListSearch != "" {
			params.Set("search", processesListSearch)
		}
		if processesListHost != "" {
			params.Set("tags", "host:"+processesListHost)
		}

		respBytes, err := client.Get("/api/v2/processes", params)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		processesList, _ := raw["data"].([]interface{})
		for _, item := range processesList {
			if !processesListAll && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)

			process, _ := item.(map[string]interface{})
			attrs, _ := process["attributes"].(map[string]interface{})
			if attrs == nil {
				attrs = process
			}

			// PID
			pid := ""
			if pidVal, ok := attrs["pid"]; ok && pidVal != nil {
				pid = fmt.Sprintf("%v", pidVal)
			}

			// Name — extract from cmdline if no dedicated name field
			name := stringFieldFromMap(attrs, "name")
			if name == "" {
				cmdline := stringFieldFromMap(attrs, "cmdline")
				name = output.TruncateString(cmdline, 30)
			}

			// User
			user := stringFieldFromMap(attrs, "user")

			// CPU percentage
			cpu := ""
			if cpuVal, ok := attrs["pctCpu"]; ok && cpuVal != nil {
				cpu = fmt.Sprintf("%.1f", toFloat64(cpuVal))
			}

			// Memory percentage
			mem := ""
			if memVal, ok := attrs["pctMem"]; ok && memVal != nil {
				mem = fmt.Sprintf("%.1f", toFloat64(memVal))
			}

			// Host
			host := stringFieldFromMap(attrs, "host")

			rows = append(rows, processRow{
				PID:    pid,
				Name:   output.TruncateString(name, 30),
				User:   output.TruncateString(user, 15),
				CPU:    cpu,
				Memory: mem,
				Host:   output.TruncateString(host, 30),
			})
		}

		// Check for next page cursor
		meta, _ := raw["meta"].(map[string]interface{})
		page, _ := meta["page"].(map[string]interface{})
		nextCursor, _ := page["after"].(string)
		if nextCursor == "" || len(processesList) == 0 {
			break
		}
		cursor = nextCursor
	}

	if opts.JSON {
		merged := map[string]interface{}{
			"data": allData,
		}
		if lastRaw != nil {
			if metaVal, ok := lastRaw["meta"]; ok {
				merged["meta"] = metaVal
			}
		}
		return output.RenderJSON(merged, opts)
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No processes found.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "PID"},
		{Name: "Name", Width: 30},
		{Name: "User", Width: 15},
		{Name: "CPU%"},
		{Name: "Memory"},
		{Name: "Host", Width: 30},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.PID, r.Name, r.User, r.CPU, r.Memory, r.Host}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		return 0
	}
}

// ---- init ----

func init() {
	// processes list flags
	processesListCmd.Flags().StringVar(&processesListSearch, "search", "", "Search by process name or command line")
	processesListCmd.Flags().StringVar(&processesListHost, "host", "", "Filter by host name")
	processesListCmd.Flags().BoolVar(&processesListAll, "all", false, "Fetch all pages until no cursor remains (overrides --limit)")

	// Add subcommands to processes
	processesCmd.AddCommand(processesListCmd)

	// Add processes to root
	rootCmd.AddCommand(processesCmd)
}
