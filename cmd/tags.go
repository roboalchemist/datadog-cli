package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- tags command group ----

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "Query host tags from Datadog",
	Long: `Query host tags from Datadog.

Subcommands:
  list   List all host tags
  get    Get tags for a specific host`,
}

// ---- tags list ----

var tagsListSource string

var tagsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all host tags",
	Long: `List all host tags from Datadog.

Returns a mapping of tags to the hosts that have each tag.

Uses GET /api/v1/tags/hosts.

Examples:
  datadog-cli tags list
  datadog-cli tags list --source users
  datadog-cli tags list --json`,
	RunE: runTagsList,
}

func runTagsList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	if tagsListSource != "" {
		params.Set("source", tagsListSource)
	}

	respBytes, err := client.Get("/api/v1/tags/hosts", params)
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

	// API returns {"tags": {"hostname": ["tag1", "tag2"], ...}}
	tagsMap, _ := raw["tags"].(map[string]interface{})
	if len(tagsMap) == 0 {
		fmt.Fprintln(os.Stdout, "No host tags found.")
		return nil
	}

	// Build a reverse map: tag → []hostname
	tagToHosts := make(map[string][]string)
	for hostname, tagsRaw := range tagsMap {
		tagsList, _ := tagsRaw.([]interface{})
		for _, t := range tagsList {
			if tag, ok := t.(string); ok {
				tagToHosts[tag] = append(tagToHosts[tag], hostname)
			}
		}
	}

	// Sort tags for deterministic output
	sortedTags := make([]string, 0, len(tagToHosts))
	for tag := range tagToHosts {
		sortedTags = append(sortedTags, tag)
	}
	sort.Strings(sortedTags)

	// Apply limit
	limit := flagLimit
	if limit > 0 && len(sortedTags) > limit {
		sortedTags = sortedTags[:limit]
	}

	type tagRow struct {
		Tag       string
		HostCount string
	}

	rows := make([]tagRow, 0, len(sortedTags))
	tableRows := make([][]string, 0, len(sortedTags))
	for _, tag := range sortedTags {
		hosts := tagToHosts[tag]
		sort.Strings(hosts)
		count := fmt.Sprintf("%d", len(hosts))
		rows = append(rows, tagRow{Tag: tag, HostCount: count})
		tableRows = append(tableRows, []string{tag, count})
	}

	cols := []output.ColumnConfig{
		{Name: "Tag", Width: 60},
		{Name: "Host Count"},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- tags get ----

var tagsGetSource string

var tagsGetCmd = &cobra.Command{
	Use:   "get <host_name>",
	Short: "Get tags for a specific host",
	Long: `Get all tags for a specific host.

Uses GET /api/v1/tags/hosts/{host_name}.

Examples:
  datadog-cli tags get myhost.example.com
  datadog-cli tags get myhost.example.com --source users
  datadog-cli tags get myhost.example.com --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTagsGet,
}

func runTagsGet(cmd *cobra.Command, args []string) error {
	hostname := args[0]
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	if tagsGetSource != "" {
		params.Set("source", tagsGetSource)
	}

	respBytes, err := client.Get("/api/v1/tags/hosts/"+hostname, params)
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

	// API returns {"tags": ["tag1", "tag2", ...]}
	tagsRaw, _ := raw["tags"].([]interface{})
	if len(tagsRaw) == 0 {
		fmt.Fprintf(os.Stdout, "No tags found for host %q.\n", hostname)
		return nil
	}

	tags := make([]string, 0, len(tagsRaw))
	for _, t := range tagsRaw {
		if s, ok := t.(string); ok {
			tags = append(tags, s)
		}
	}
	sort.Strings(tags)

	// Apply limit
	limit := flagLimit
	if limit > 0 && len(tags) > limit {
		tags = tags[:limit]
	}

	type tagRow struct {
		Tag string
	}

	rows := make([]tagRow, 0, len(tags))
	tableRows := make([][]string, 0, len(tags))
	for _, tag := range tags {
		// Split key:value for display
		key := tag
		value := ""
		if idx := strings.Index(tag, ":"); idx >= 0 {
			key = tag[:idx]
			value = tag[idx+1:]
		}
		rows = append(rows, tagRow{Tag: tag})
		tableRows = append(tableRows, []string{key, value})
	}

	cols := []output.ColumnConfig{
		{Name: "Key", Width: 40},
		{Name: "Value", Width: 60},
	}

	fmt.Fprintf(os.Stdout, "Host: %s  (%d tags)\n\n", hostname, len(tags))
	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- init ----

func init() {
	// tags list flags
	tagsListCmd.Flags().StringVar(&tagsListSource, "source", "", "Filter tags by source (e.g. 'users', 'datadog')")

	// tags get flags
	tagsGetCmd.Flags().StringVar(&tagsGetSource, "source", "", "Filter tags by source (e.g. 'users', 'datadog')")

	// Wire up subcommands
	tagsCmd.AddCommand(tagsListCmd)
	tagsCmd.AddCommand(tagsGetCmd)

	// Register with root
	rootCmd.AddCommand(tagsCmd)
}
