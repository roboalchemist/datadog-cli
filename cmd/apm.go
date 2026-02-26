package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- apm command group ----

var apmCmd = &cobra.Command{
	Use:   "apm",
	Short: "Query APM services, definitions, and dependencies",
	Long:  `Query APM services, service catalog definitions, and service dependencies.`,
	Example: `  # List all APM services
  datadog-cli apm services

  # Show service dependency map for production
  datadog-cli apm dependencies --env production

  # List service catalog definitions as JSON
  datadog-cli apm definitions --json`,
}

// ---- apm services ----

var apmServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "List APM services",
	Long: `List APM services from the service catalog.

Uses GET /api/v2/services/definitions.`,
	Example: `  # List all APM services
  datadog-cli apm services

  # List services in JSON format
  datadog-cli apm services --json`,
	RunE: runAPMServices,
}

func runAPMServices(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{
		"page[size]": {"100"},
	}

	respBytes, err := client.Get("/api/v2/services/definitions", params)
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

	data, _ := raw["data"].([]interface{})
	if len(data) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No APM services found.")
		return nil
	}

	// Apply --limit
	if flagLimit > 0 && len(data) > flagLimit {
		data = data[:flagLimit]
	}

	type serviceRow struct {
		Name string
		Type string
		Team string
		Tags string
	}

	var rows []serviceRow
	for _, item := range data {
		entry, _ := item.(map[string]interface{})
		attrs, _ := entry["attributes"].(map[string]interface{})
		schema, _ := attrs["schema"].(map[string]interface{})

		name := apmExtractServiceName(schema)
		team := apmStringField(schema, "team")
		tags := apmFormatList(apmStringSlice(schema, "tags"))
		// schema type/kind
		svcType := apmStringField(schema, "type")
		if svcType == "" {
			svcType = apmStringField(schema, "kind")
		}

		rows = append(rows, serviceRow{
			Name: name,
			Type: svcType,
			Team: team,
			Tags: tags,
		})
	}

	cols := []output.ColumnConfig{
		{Name: "Name", Width: 40},
		{Name: "Type", Width: 20},
		{Name: "Team", Width: 25},
		{Name: "Tags", Width: 40},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Name, r.Type, r.Team, r.Tags}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- apm definitions ----

var apmDefinitionsCmd = &cobra.Command{
	Use:   "definitions",
	Short: "List service catalog definitions",
	Long: `List service catalog definitions with full detail.

Uses GET /api/v2/services/definitions.`,
	Example: `  # List all service catalog definitions
  datadog-cli apm definitions

  # List definitions in JSON format
  datadog-cli apm definitions --json`,
	RunE: runAPMDefinitions,
}

func runAPMDefinitions(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	pageSize := flagLimit
	if pageSize > 100 {
		pageSize = 100
	}

	params := url.Values{
		"page[size]": {fmt.Sprintf("%d", pageSize)},
	}

	respBytes, err := client.Get("/api/v2/services/definitions", params)
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

	data, _ := raw["data"].([]interface{})
	if len(data) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No service definitions found.")
		return nil
	}

	type defRow struct {
		Name     string
		Schema   string
		Team     string
		Contacts string
		Tags     string
	}

	var rows []defRow
	for _, item := range data {
		entry, _ := item.(map[string]interface{})
		attrs, _ := entry["attributes"].(map[string]interface{})
		schema, _ := attrs["schema"].(map[string]interface{})
		meta, _ := attrs["meta"].(map[string]interface{})

		name := apmExtractServiceName(schema)
		schemaVersion := apmStringField(meta, "schema-version")
		team := apmStringField(schema, "team")
		tags := apmFormatList(apmStringSlice(schema, "tags"))
		contacts := apmExtractContacts(schema)

		rows = append(rows, defRow{
			Name:     name,
			Schema:   schemaVersion,
			Team:     team,
			Contacts: contacts,
			Tags:     tags,
		})
	}

	cols := []output.ColumnConfig{
		{Name: "Name", Width: 40},
		{Name: "Schema", Width: 8},
		{Name: "Team", Width: 25},
		{Name: "Contacts", Width: 35},
		{Name: "Tags", Width: 35},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Name, r.Schema, r.Team, r.Contacts, r.Tags}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- apm dependencies ----

var (
	apmDepsEnv     string
	apmDepsService string
)

var apmDependenciesCmd = &cobra.Command{
	Use:   "dependencies",
	Short: "Show service dependency map",
	Long: `Show APM service dependencies (service map).

Uses GET /api/v1/service_dependencies.`,
	Example: `  # Show all service dependencies in production
  datadog-cli apm dependencies --env production

  # Show dependencies for a specific service in staging
  datadog-cli apm dependencies --env staging --service my-service

  # Show dependencies as JSON
  datadog-cli apm dependencies --env production --json`,
	RunE: runAPMDependencies,
}

var apmDepsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Alias for dependencies — show service dependency map",
	Long: `Show APM service dependencies (service map). Alias for 'dependencies'.

Uses GET /api/v1/service_dependencies.`,
	Example: `  # Show all service dependencies in production
  datadog-cli apm deps --env production

  # Show dependencies for a specific service
  datadog-cli apm deps --env staging --service my-service`,
	RunE: runAPMDependencies,
}

func runAPMDependencies(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{
		"env": {apmDepsEnv},
	}
	if apmDepsService != "" {
		params["service"] = []string{apmDepsService}
	}

	respBytes, err := client.Get("/api/v1/service_dependencies", params)
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

	if len(raw) == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "No service dependencies found for env=%s.\n", apmDepsEnv)
		return nil
	}

	type depRow struct {
		Service   string
		CallCount string
		Calls     string
	}

	var rows []depRow
	for svcName, depsInfo := range raw {
		depsMap, _ := depsInfo.(map[string]interface{})
		calls, _ := depsMap["calls"].([]interface{})

		callNames := make([]string, 0, len(calls))
		for _, c := range calls {
			if s, ok := c.(string); ok {
				callNames = append(callNames, s)
			}
		}

		rows = append(rows, depRow{
			Service:   svcName,
			CallCount: fmt.Sprintf("%d", len(callNames)),
			Calls:     apmFormatList(callNames),
		})
	}

	// Sort by service name for deterministic output
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[i].Service > rows[j].Service {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}

	cols := []output.ColumnConfig{
		{Name: "Service", Width: 40},
		{Name: "# Calls"},
		{Name: "Downstream Services", Width: 60},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.Service, r.CallCount, r.Calls}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

// apmStringField safely extracts a string value from a map.
func apmStringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// apmStringSlice extracts a []string from a map field that holds []interface{}.
func apmStringSlice(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// apmFormatList renders a []string as a comma-separated string, truncating
// if there are more than 5 items.
func apmFormatList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	const max = 5
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf("... (+%d)", len(items)-max)
}

// apmExtractServiceName handles the different field names across schema versions.
func apmExtractServiceName(schema map[string]interface{}) string {
	if schema == nil {
		return ""
	}
	for _, key := range []string{"dd-service", "service-name", "name"} {
		if v := apmStringField(schema, key); v != "" {
			return v
		}
	}
	return ""
}

// apmExtractContacts builds a compact contacts string from schema contacts/contact fields.
func apmExtractContacts(schema map[string]interface{}) string {
	if schema == nil {
		return ""
	}
	var contacts []string

	if raw, ok := schema["contacts"].([]interface{}); ok {
		for _, c := range raw {
			if m, ok := c.(map[string]interface{}); ok {
				// prefer "contact" field, then "email", then "name"
				for _, key := range []string{"contact", "email", "name"} {
					if v := apmStringField(m, key); v != "" {
						contacts = append(contacts, v)
						break
					}
				}
			}
		}
	} else if v := apmStringField(schema, "contact"); v != "" {
		contacts = append(contacts, v)
	}

	return apmFormatList(contacts)
}

// ---- init ----

func init() {
	// dependencies / deps flags (shared, so we register on both cmds)
	for _, c := range []*cobra.Command{apmDependenciesCmd, apmDepsCmd} {
		c.Flags().StringVarP(&apmDepsEnv, "env", "e", "", "APM environment (required), e.g. production, staging")
		c.Flags().StringVar(&apmDepsService, "service", "", "Filter by service name")
		_ = c.MarkFlagRequired("env")
	}

	// Wire up subcommands
	apmCmd.AddCommand(apmServicesCmd)
	apmCmd.AddCommand(apmDefinitionsCmd)
	apmCmd.AddCommand(apmDependenciesCmd)
	apmCmd.AddCommand(apmDepsCmd)

	// Register with root
	rootCmd.AddCommand(apmCmd)
}
