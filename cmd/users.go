package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- users command group ----

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Query users from Datadog",
	Long:  `Query users from Datadog.`,
	Example: `  # List all users in your organization
  datadog-cli users list

  # Get details for a specific user
  datadog-cli users get abc12345-1234-5678-abcd-1234567890ab`,
}

// ---- users list ----

var usersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	Long: `List users in your Datadog organization.

Uses GET /api/v2/users.`,
	Example: `  # List all users
  datadog-cli users list

  # List users in JSON format
  datadog-cli users list --json`,
	RunE: runUsersList,
}

func runUsersList(cmd *cobra.Command, args []string) error {
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/users", nil)
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
		_, _ = fmt.Fprintln(os.Stdout, "No users found.")
		return nil
	}

	if flagLimit > 0 && len(dataArr) > flagLimit {
		dataArr = dataArr[:flagLimit]
	}

	type userRow struct {
		ID     string
		Name   string
		Email  string
		Status string
		Role   string
	}

	rows := make([]userRow, 0, len(dataArr))
	tableRows := make([][]string, 0, len(dataArr))

	for _, item := range dataArr {
		u, _ := item.(map[string]interface{})
		id := usersStringField(u, "id")
		if len(id) > 20 {
			id = id[:20] + "..."
		}
		attrs, _ := u["attributes"].(map[string]interface{})
		name := output.TruncateString(usersStringField(attrs, "name"), 25)
		email := output.TruncateString(usersStringField(attrs, "email"), 35)
		status := usersStringField(attrs, "status")
		role := usersExtractRole(u)

		rows = append(rows, userRow{ID: id, Name: name, Email: email, Status: status, Role: role})
		tableRows = append(tableRows, []string{id, name, email, output.ColorStatus(status), role})
	}

	cols := []output.ColumnConfig{
		{Name: "ID", Width: 20},
		{Name: "Name", Width: 25},
		{Name: "Email", Width: 35},
		{Name: "Status", Width: 10},
		{Name: "Role", Width: 20},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- users get ----

var usersGetCmd = &cobra.Command{
	Use:   "get <user_id>",
	Short: "Get user details by ID",
	Long: `Get detailed information about a specific user.

Uses GET /api/v2/users/{id}.`,
	Example: `  # Get details for a specific user
  datadog-cli users get abc12345-1234-5678-abcd-1234567890ab

  # Get user details in JSON format
  datadog-cli users get abc12345-1234-5678-abcd-1234567890ab --json`,
	Args: cobra.ExactArgs(1),
	RunE: runUsersGet,
}

func runUsersGet(cmd *cobra.Command, args []string) error {
	userID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	respBytes, err := client.Get("/api/v2/users/"+userID, nil)
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

	user, _ := raw["data"].(map[string]interface{})
	if user == nil {
		user = raw
	}

	attrs, _ := user["attributes"].(map[string]interface{})
	if attrs == nil {
		attrs = map[string]interface{}{}
	}

	email := usersStringField(attrs, "email")
	_, _ = fmt.Fprintf(os.Stdout, "User: %s\n\n", email)

	created := usersFormatTimestamp(attrs["created_at"])
	modified := usersFormatTimestamp(attrs["modified_at"])

	details := []struct{ k, v string }{
		{"ID", usersStringField(user, "id")},
		{"Name", usersStringField(attrs, "name")},
		{"Email", email},
		{"Handle", usersStringField(attrs, "handle")},
		{"Title", usersStringField(attrs, "title")},
		{"Status", output.ColorStatus(usersStringField(attrs, "status"))},
		{"Verified", usersBoolField(attrs, "verified")},
		{"Service Account", usersBoolField(attrs, "service_account")},
		{"Created", created},
		{"Modified", modified},
	}

	type detailRow struct {
		Field string
		Value string
	}

	detailRows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" {
			continue
		}
		detailRows = append(detailRows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 20},
		{Name: "Value", Width: 60},
	}

	return output.RenderTable(cols, tableRows, detailRows, opts)
}

// ---- helpers ----

func usersStringField(m map[string]interface{}, key string) string {
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

func usersBoolField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	if b {
		return "Yes"
	}
	return "No"
}

func usersExtractRole(user map[string]interface{}) string {
	// v2 users may have relationships.roles
	rels, ok := user["relationships"].(map[string]interface{})
	if !ok {
		return ""
	}
	rolesRel, ok := rels["roles"].(map[string]interface{})
	if !ok {
		return ""
	}
	rolesData, ok := rolesRel["data"].([]interface{})
	if !ok || len(rolesData) == 0 {
		return ""
	}
	names := make([]string, 0, len(rolesData))
	for _, r := range rolesData {
		if rm, ok := r.(map[string]interface{}); ok {
			if name, ok := rm["id"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return strings.Join(names, ", ")
}

func usersFormatTimestamp(ts interface{}) string {
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
	usersCmd.AddCommand(usersListCmd)
	usersCmd.AddCommand(usersGetCmd)

	rootCmd.AddCommand(usersCmd)
}
