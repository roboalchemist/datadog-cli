package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// maxContainersPageSize is the maximum page[size] accepted by the Datadog containers API.
const maxContainersPageSize = 10000

// ---- containers command group ----

var containersCmd = &cobra.Command{
	Use:   "containers",
	Short: "Query container information from Datadog",
	Long: `Query container information from Datadog.

Uses the Datadog v2 Containers API to list running containers
across your infrastructure.`,
	Example: `  # List all running containers
  datadog-cli containers list

  # Filter containers by Kubernetes namespace
  datadog-cli containers list --filter "kube_namespace:production"`,
}

// ---- containers list ----

var (
	containersListFilter string
	containersListAll    bool
)

var containersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List containers",
	Long: `List containers from Datadog using the v2 containers API.

Uses GET /api/v2/containers. Supports cursor-based pagination via --all to
fetch every page until no next_cursor is returned.`,
	Example: `  # List up to 1000 running containers (default)
  datadog-cli containers list

  # Fetch all containers across all pages
  datadog-cli containers list --all

  # Filter containers by Kubernetes namespace
  datadog-cli containers list --filter "kube_namespace:production"

  # Filter by image name and output as JSON
  datadog-cli containers list --filter "image_name:nginx" --json

  # Fetch up to 500 containers
  datadog-cli containers list --limit 500`,
	RunE: runContainersList,
}

func runContainersList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	// Determine effective limit: --all means no cap.
	effectiveLimit := flagLimit
	if containersListAll {
		effectiveLimit = 0 // 0 = unlimited
	}

	// First page size: min(effectiveLimit, maxContainersPageSize).
	// If unlimited (--all), use max page size.
	firstPageSize := effectiveLimit
	if firstPageSize == 0 || firstPageSize > maxContainersPageSize {
		firstPageSize = maxContainersPageSize
	}

	type containerRow struct {
		ContainerID string
		Name        string
		Image       string
		State       string
		Host        string
	}

	var rows []containerRow
	// allData accumulates raw container items for JSON output across pages.
	var allData []interface{}
	// lastRaw holds the most recent raw response for metadata.
	var lastRaw map[string]interface{}
	cursor := ""
	pageNum := 0

	needsMorePages := func() bool {
		if containersListAll {
			return true // stop only when cursor is gone
		}
		return len(rows) < effectiveLimit
	}

	for needsMorePages() {
		pageNum++

		params := url.Values{}

		// Compute page size for this request.
		pageSize := firstPageSize
		if !containersListAll && pageNum > 1 {
			remaining := effectiveLimit - len(rows)
			if remaining < pageSize {
				pageSize = remaining
			}
		}
		params.Set("page[size]", fmt.Sprintf("%d", pageSize))

		if containersListFilter != "" {
			params.Set("filter[tags]", containersListFilter)
		}

		if cursor != "" {
			params.Set("page[cursor]", cursor)
			// Print progress to stderr (unless --quiet).
			if !flagQuiet {
				if containersListAll {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d containers so far)...\n", pageNum, len(rows))
				} else {
					_, _ = fmt.Fprintf(os.Stderr, "Fetching page %d (%d/%d)...\n", pageNum, len(rows), effectiveLimit)
				}
			}
		}

		respBytes, err := client.Get("/api/v2/containers", params)
		if err != nil {
			return err
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(respBytes, &raw); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
		lastRaw = raw

		containersList, _ := raw["data"].([]interface{})

		for _, item := range containersList {
			if !containersListAll && len(rows) >= effectiveLimit {
				break
			}
			allData = append(allData, item)

			container, _ := item.(map[string]interface{})
			attrs, _ := container["attributes"].(map[string]interface{})
			if attrs == nil {
				attrs = container
			}

			// Container ID: prefer top-level id, fall back to attributes.
			containerID := ""
			if id, ok := container["id"].(string); ok && id != "" {
				containerID = output.TruncateString(id, 20)
			} else {
				containerID = output.TruncateString(stringFieldFromMap(attrs, "container_id"), 20)
			}

			// Name
			name := stringFieldFromMap(attrs, "name")
			if name == "" {
				name = stringFieldFromMap(attrs, "container_name")
			}

			// Image
			image := stringFieldFromMap(attrs, "image_name")

			// State
			state := stringFieldFromMap(attrs, "state")
			if state == "" {
				state = stringFieldFromMap(attrs, "status")
			}

			// Host
			host := stringFieldFromMap(attrs, "host")

			rows = append(rows, containerRow{
				ContainerID: containerID,
				Name:        output.TruncateString(name, 40),
				Image:       output.TruncateString(image, 35),
				State:       state,
				Host:        output.TruncateString(host, 30),
			})
		}

		// Check for next cursor at meta.pagination.next_cursor.
		meta, _ := raw["meta"].(map[string]interface{})
		pagination, _ := meta["pagination"].(map[string]interface{})
		nextCursor, _ := pagination["next_cursor"].(string)
		if nextCursor == "" || len(containersList) == 0 {
			break
		}
		cursor = nextCursor
	}

	if opts.JSON {
		// Build a merged response: combine all pages' data arrays, keep metadata
		// from the last response so meta reflects final state.
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
		_, _ = fmt.Fprintln(os.Stdout, "No containers found.")
		return nil
	}

	cols := []output.ColumnConfig{
		{Name: "Container ID", Width: 20},
		{Name: "Name", Width: 40},
		{Name: "Image", Width: 35},
		{Name: "State"},
		{Name: "Host", Width: 30},
	}

	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{r.ContainerID, r.Name, r.Image, r.State, r.Host}
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- init ----

func init() {
	// containers list flags
	containersListCmd.Flags().StringVar(&containersListFilter, "filter", "", "Filter containers by tag (e.g. 'kube_namespace:production')")
	containersListCmd.Flags().BoolVar(&containersListAll, "all", false, "Fetch all pages until no cursor remains (overrides --limit)")

	// Add subcommands to containers
	containersCmd.AddCommand(containersListCmd)

	// Add containers to root
	rootCmd.AddCommand(containersCmd)
}
