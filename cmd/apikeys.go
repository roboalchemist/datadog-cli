package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- api-keys command group ----

var apiKeysCmd = &cobra.Command{
	Use:   "api-keys",
	Short: "Query API keys from Datadog (read-only, no secrets exposed)",
	Long: `Query API keys from Datadog.

Only key metadata is returned — the actual key values are never exposed.
The last 4 characters are shown for identification.`,
	Example: `  # List all API keys (metadata only, no secrets)
  datadog-cli api-keys list

  # List API keys in JSON format
  datadog-cli api-keys list --json`,
}

// ---- api-keys list ----

var apiKeysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys",
	Long: `List API keys in your Datadog organization.

Uses GET /api/v2/api_keys.
Required scope: api_keys_read

For security, actual key values are not exposed — only the last 4 characters.`,
	Example: `  # List all API keys
  datadog-cli api-keys list

  # List API keys in JSON format
  datadog-cli api-keys list --json`,
	RunE: runAPIKeysList,
}

func runAPIKeysList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/api_keys", nil)
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

	dataArr, _ := raw["data"].([]interface{})

	if len(dataArr) == 0 {
		fmt.Fprintln(os.Stdout, "No API keys found.")
		return nil
	}

	if flagLimit > 0 && len(dataArr) > flagLimit {
		dataArr = dataArr[:flagLimit]
	}

	type apiKeyRow struct {
		ID      string
		Name    string
		Created string
		Last4   string
	}

	rows := make([]apiKeyRow, 0, len(dataArr))
	tableRows := make([][]string, 0, len(dataArr))

	for _, item := range dataArr {
		k, _ := item.(map[string]interface{})
		id := apiKeysStringField(k, "id")
		if len(id) > 20 {
			id = id[:20] + "..."
		}
		attrs, _ := k["attributes"].(map[string]interface{})
		name := output.TruncateString(apiKeysStringField(attrs, "name"), 35)
		created := apiKeysFormatTimestamp(attrs["created_at"])
		last4 := apiKeysStringField(attrs, "last4")

		rows = append(rows, apiKeyRow{ID: id, Name: name, Created: created, Last4: last4})
		tableRows = append(tableRows, []string{id, name, created, last4})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 20},
		{Name: "Name", Width: 35},
		{Name: "Created", Width: 18},
		{Name: "Last4", Width: 6},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- helpers ----

func apiKeysStringField(m map[string]interface{}, key string) string {
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

func apiKeysFormatTimestamp(ts interface{}) string {
	if ts == nil {
		return ""
	}
	s, ok := ts.(string)
	if !ok {
		return fmt.Sprintf("%v", ts)
	}
	if s == "" {
		return ""
	}
	if strings.HasSuffix(s, "Z") {
		s = s[:len(s)-1] + "+00:00"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04")
}

// ---- init ----

func init() {
	apiKeysCmd.AddCommand(apiKeysListCmd)

	rootCmd.AddCommand(apiKeysCmd)
}
