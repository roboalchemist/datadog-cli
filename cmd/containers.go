package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

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

var containersListFilter string

var containersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List containers",
	Long: `List containers from Datadog using the v2 containers API.

Uses GET /api/v2/containers.`,
	Example: `  # List all running containers
  datadog-cli containers list

  # Filter containers by Kubernetes namespace
  datadog-cli containers list --filter "kube_namespace:production"

  # Filter by image name and output as JSON
  datadog-cli containers list --filter "image_name:nginx" --json`,
	RunE: runContainersList,
}

func runContainersList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("page[size]", fmt.Sprintf("%d", flagLimit))

	if containersListFilter != "" {
		params.Set("filter[tags]", containersListFilter)
	}

	respBytes, err := client.Get("/api/v2/containers", params)
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

	containersList, _ := raw["data"].([]interface{})
	if len(containersList) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No containers found.")
		return nil
	}

	type containerRow struct {
		ContainerID string
		Name        string
		Image       string
		State       string
		Host        string
	}

	rows := make([]containerRow, 0, len(containersList))
	for _, item := range containersList {
		container, _ := item.(map[string]interface{})
		attrs, _ := container["attributes"].(map[string]interface{})
		if attrs == nil {
			attrs = container
		}

		// Container ID: prefer top-level id, fall back to attributes
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

	// Add subcommands to containers
	containersCmd.AddCommand(containersListCmd)

	// Add containers to root
	rootCmd.AddCommand(containersCmd)
}
