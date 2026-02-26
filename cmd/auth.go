package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- auth command group ----

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication utilities and information",
	Long:  `Authentication utilities and information.`,
	Example: `  # Show all required API/App key scopes
  datadog-cli auth scopes

  # Show required scopes in JSON format
  datadog-cli auth scopes --json`,
}

// commandScopeEntry describes the scopes needed by one command group.
type commandScopeEntry struct {
	Scopes      []string
	Description string
	Subcommands []string
}

// commandScopes maps each datadog-cli command group to its required Datadog scopes.
var commandScopes = map[string]commandScopeEntry{
	"api-keys": {
		Scopes:      []string{"api_keys_read"},
		Description: "Read API key metadata (no secrets exposed)",
		Subcommands: []string{"list"},
	},
	"apm": {
		Scopes:      []string{"apm_read", "apm_service_catalog_read"},
		Description: "Read APM services, traces, and service catalog",
		Subcommands: []string{"services", "definitions", "dependencies"},
	},
	"audit": {
		Scopes:      []string{"audit_logs_read"},
		Description: "Read audit trail events",
		Subcommands: []string{"search"},
	},
	"containers": {
		Scopes:      []string{"containers_read"},
		Description: "Read container data",
		Subcommands: []string{"list"},
	},
	"dashboards": {
		Scopes:      []string{"dashboards_read"},
		Description: "Read dashboard definitions",
		Subcommands: []string{"list", "get", "search"},
	},
	"downtimes": {
		Scopes:      []string{"monitors_downtime"},
		Description: "Read downtime schedules",
		Subcommands: []string{"list", "get"},
	},
	"events": {
		Scopes:      []string{"events_read"},
		Description: "Read event stream data",
		Subcommands: []string{"list", "get"},
	},
	"hosts": {
		Scopes:      []string{"hosts_read"},
		Description: "Read infrastructure host data",
		Subcommands: []string{"list", "totals"},
	},
	"incidents": {
		Scopes:      []string{"incident_read"},
		Description: "Read incident data and timeline",
		Subcommands: []string{"list", "get"},
	},
	"logs": {
		Scopes:      []string{"logs_read_data", "logs_read_index_data"},
		Description: "Read log data and index configuration",
		Subcommands: []string{"search", "aggregate", "indexes"},
	},
	"metrics": {
		Scopes:      []string{"metrics_read", "timeseries_query"},
		Description: "Read metrics and query timeseries data",
		Subcommands: []string{"list", "query"},
	},
	"monitors": {
		Scopes:      []string{"monitors_read"},
		Description: "Read monitor configurations and status",
		Subcommands: []string{"list", "get", "search"},
	},
	"notebooks": {
		Scopes:      []string{"notebooks_read"},
		Description: "Read notebook contents and metadata",
		Subcommands: []string{"list", "get"},
	},
	"pipelines": {
		Scopes:      []string{"logs_read_config"},
		Description: "Read log pipeline configurations",
		Subcommands: []string{"list", "get"},
	},
	"processes": {
		Scopes:      []string{"processes_read"},
		Description: "Read process data",
		Subcommands: []string{"list"},
	},
	"rum": {
		Scopes:      []string{"rum_read"},
		Description: "Read RUM events for session replays and error debugging",
		Subcommands: []string{"search", "aggregate"},
	},
	"slos": {
		Scopes:      []string{"slos_read"},
		Description: "Read SLO configurations and history",
		Subcommands: []string{"list", "get", "history"},
	},
	"traces": {
		Scopes:      []string{"apm_read"},
		Description: "Read APM traces and spans",
		Subcommands: []string{"search", "aggregate"},
	},
	"usage": {
		Scopes:      []string{"usage_read"},
		Description: "Read usage metering data",
		Subcommands: []string{"summary", "top-metrics"},
	},
	"users": {
		Scopes:      []string{"user_access_read"},
		Description: "Read user data and organization membership",
		Subcommands: []string{"list", "get"},
	},
}

// scopeDescriptions maps each Datadog scope name to a human-readable description.
var scopeDescriptions = map[string]string{
	"api_keys_read":            "Read API key metadata (no secrets exposed)",
	"apm_read":                 "Read APM services, traces, and dependencies",
	"apm_service_catalog_read": "Read service definitions from Service Catalog",
	"audit_logs_read":          "Read audit trail events",
	"containers_read":          "Read container data",
	"dashboards_read":          "Read dashboard definitions",
	"events_read":              "Read events from event stream",
	"hosts_read":               "Read host data from Infrastructure",
	"incident_read":            "Read incident data and timeline",
	"logs_read_config":         "Read log pipeline and index configuration",
	"logs_read_data":           "Read log data from Log Explorer",
	"logs_read_index_data":     "Read log index configuration",
	"metrics_read":             "List active metrics",
	"monitors_downtime":        "Read downtime schedules",
	"monitors_read":            "Read monitor configurations and status",
	"notebooks_read":           "Read notebook contents and metadata",
	"processes_read":           "Read process data",
	"rum_read":                 "Read RUM events for session replays and error debugging",
	"slos_read":                "Read SLO configurations and history",
	"timeseries_query":         "Query metric timeseries data",
	"usage_read":               "Read usage metering data and billing information",
	"user_access_read":         "Read user data and organization membership",
}

// ---- auth scopes ----

var authScopesCmd = &cobra.Command{
	Use:   "scopes",
	Short: "Display required Datadog API/App key scopes",
	Long: `Display the Datadog Application Key scopes required by datadog-cli.

No API call is made — this displays a static list of required permissions.
Use this to create a minimally-scoped Application Key for datadog-cli.`,
	Example: `  # Show all required scopes in table format
  datadog-cli auth scopes

  # Show required scopes in JSON format
  datadog-cli auth scopes --json`,
	RunE: runAuthScopes,
}

func runAuthScopes(cmd *cobra.Command, args []string) error {
	opts := GetOutputOptions()

	// Collect all unique scopes and which commands use them
	allScopes := map[string][]string{} // scope → commands that use it
	for cmdName, entry := range commandScopes {
		for _, scope := range entry.Scopes {
			allScopes[scope] = append(allScopes[scope], cmdName)
		}
	}

	// Sort scope names
	scopeNames := make([]string, 0, len(allScopes))
	for s := range allScopes {
		scopeNames = append(scopeNames, s)
	}
	sort.Strings(scopeNames)

	if opts.JSON {
		type scopeJSON struct {
			Scope       string   `json:"scope"`
			Description string   `json:"description"`
			UsedBy      []string `json:"used_by"`
		}
		result := make([]scopeJSON, 0, len(scopeNames))
		for _, s := range scopeNames {
			cmds := allScopes[s]
			sort.Strings(cmds)
			result = append(result, scopeJSON{
				Scope:       s,
				Description: scopeDescriptions[s],
				UsedBy:      cmds,
			})
		}
		return output.RenderJSON(result, opts)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Datadog Application Key Scopes Required by datadog-cli\n\n")
	_, _ = fmt.Fprintf(os.Stdout, "Create a scoped Application Key with these permissions.\n\n")

	type scopeRow struct {
		Scope       string
		Description string
		UsedBy      string
	}

	rows := make([]scopeRow, 0, len(scopeNames))
	tableRows := make([][]string, 0, len(scopeNames))

	for _, s := range scopeNames {
		cmds := allScopes[s]
		sort.Strings(cmds)
		desc := scopeDescriptions[s]
		usedBy := ""
		if len(cmds) > 0 {
			usedBy = joinStrings(cmds, ", ")
		}
		rows = append(rows, scopeRow{Scope: s, Description: desc, UsedBy: usedBy})
		tableRows = append(tableRows, []string{s, desc, usedBy})
	}

	cols := []output.ColumnConfig{
		{Name: "Scope", Width: 30},
		{Name: "Description", Width: 45},
		{Name: "Used By", Width: 30},
	}

	if err := output.RenderTable(cols, tableRows, rows, opts); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "\nTotal: %d scopes required for full datadog-cli functionality.\n", len(scopeNames))
	return nil
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// ---- init ----

func init() {
	authCmd.AddCommand(authScopesCmd)

	rootCmd.AddCommand(authCmd)
}
