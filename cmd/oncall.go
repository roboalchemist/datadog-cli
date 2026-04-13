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

// ---- oncall command group ----

var oncallCmd = &cobra.Command{
	Use:   "oncall",
	Short: "Query Datadog On-Call schedules and escalation policies",
	Long:  `Query Datadog On-Call schedules, escalation policies, and current on-call users.`,
	Example: `  # Get an on-call schedule by ID
  datadog-cli oncall schedules get 3653d3c6-0c75-11ea-ad28-fb5701eabc7d

  # Show who is currently on-call for a schedule
  datadog-cli oncall schedules who 3653d3c6-0c75-11ea-ad28-fb5701eabc7d

  # Get an escalation policy by ID
  datadog-cli oncall escalations get a3000000-0000-0000-0000-000000000000

  # Show who is on-call for a team
  datadog-cli oncall teams who 27590dae-47be-4a7d-9abf-8f4e45124020

  # Get routing rules for a team
  datadog-cli oncall teams routing-rules 27590dae-47be-4a7d-9abf-8f4e45124020`,
}

// ---- oncall schedules sub-group ----

var oncallSchedulesCmd = &cobra.Command{
	Use:   "schedules",
	Short: "Query On-Call schedules",
	Long:  `Query Datadog On-Call schedules and who is on-call.`,
	Example: `  # Get a schedule by ID
  datadog-cli oncall schedules get 3653d3c6-0c75-11ea-ad28-fb5701eabc7d

  # Show who is currently on-call for a schedule
  datadog-cli oncall schedules who 3653d3c6-0c75-11ea-ad28-fb5701eabc7d`,
}

// ---- oncall schedules get ----

var oncallSchedulesGetInclude string

var oncallSchedulesGetCmd = &cobra.Command{
	Use:   "get <schedule_id>",
	Short: "Get On-Call schedule details by ID",
	Long: `Get detailed information about an On-Call schedule.

Uses GET /api/v2/on-call/schedules/{schedule_id}.`,
	Example: `  # Get schedule details
  datadog-cli oncall schedules get 3653d3c6-0c75-11ea-ad28-fb5701eabc7d

  # Include related teams and layers
  datadog-cli oncall schedules get 3653d3c6-0c75-11ea-ad28-fb5701eabc7d --include "teams,layers"

  # Get schedule in JSON format
  datadog-cli oncall schedules get 3653d3c6-0c75-11ea-ad28-fb5701eabc7d --json`,
	Args: cobra.ExactArgs(1),
	RunE: runOncallSchedulesGet,
}

func runOncallSchedulesGet(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	if oncallSchedulesGetInclude != "" {
		params.Set("include", oncallSchedulesGetInclude)
	}

	respBytes, err := client.Get("/api/v2/on-call/schedules/"+scheduleID, params)
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

	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		data = raw
	}
	attrs, _ := data["attributes"].(map[string]interface{})
	if attrs == nil {
		attrs = map[string]interface{}{}
	}

	id := oncallStringField(data, "id")
	name := oncallStringField(attrs, "name")
	timezone := oncallStringField(attrs, "time_zone")

	// Extract team names from included
	teams := oncallExtractIncludedNames(raw, "teams")

	_, _ = fmt.Fprintf(os.Stdout, "Schedule: %s\n\n", name)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Name", name},
		{"Timezone", timezone},
		{"Teams", teams},
	}

	rows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" {
			continue
		}
		rows = append(rows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 20},
		{Name: "Value", Width: 60},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- oncall schedules who ----

var oncallSchedulesWhoAt string

var oncallSchedulesWhoCmd = &cobra.Command{
	Use:   "who <schedule_id>",
	Short: "Show who is on-call for a schedule",
	Long: `Show who is currently on-call for an On-Call schedule.

Uses GET /api/v2/on-call/schedules/{schedule_id}/on-call.
Defaults to the current time; use --at to query a specific time.`,
	Example: `  # Show who is currently on-call
  datadog-cli oncall schedules who 3653d3c6-0c75-11ea-ad28-fb5701eabc7d

  # Show who was on-call at a specific time
  datadog-cli oncall schedules who 3653d3c6-0c75-11ea-ad28-fb5701eabc7d --at "2025-05-07T02:53:01Z"

  # Output as JSON
  datadog-cli oncall schedules who 3653d3c6-0c75-11ea-ad28-fb5701eabc7d --json`,
	Args: cobra.ExactArgs(1),
	RunE: runOncallSchedulesWho,
}

func runOncallSchedulesWho(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	params.Set("include", "user")
	if oncallSchedulesWhoAt != "" {
		params.Set("filter[at_ts]", oncallSchedulesWhoAt)
	}

	respBytes, err := client.Get("/api/v2/on-call/schedules/"+scheduleID+"/on-call", params)
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

	// Response shape: Shift resource with included user
	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		data = raw
	}

	// Extract user details from included
	userEmail, userName, userHandle := oncallExtractShiftUser(raw, data)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"Name", userName},
		{"Email", userEmail},
		{"Handle", userHandle},
	}

	rows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" {
			continue
		}
		rows = append(rows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No on-call user found for this schedule.")
		return nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "On-call for schedule %s:\n\n", scheduleID)

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 10},
		{Name: "Value", Width: 50},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- oncall escalations sub-group ----

var oncallEscalationsCmd = &cobra.Command{
	Use:   "escalations",
	Short: "Query On-Call escalation policies",
	Long:  `Query Datadog On-Call escalation policies.`,
	Example: `  # Get an escalation policy by ID
  datadog-cli oncall escalations get a3000000-0000-0000-0000-000000000000`,
}

// ---- oncall escalations get ----

var oncallEscalationsGetInclude string

var oncallEscalationsGetCmd = &cobra.Command{
	Use:   "get <policy_id>",
	Short: "Get On-Call escalation policy by ID",
	Long: `Get detailed information about an On-Call escalation policy.

Uses GET /api/v2/on-call/escalation-policies/{policy_id}.`,
	Example: `  # Get escalation policy details
  datadog-cli oncall escalations get a3000000-0000-0000-0000-000000000000

  # Include related teams and steps
  datadog-cli oncall escalations get a3000000-0000-0000-0000-000000000000 --include "teams,steps"

  # Output as JSON
  datadog-cli oncall escalations get a3000000-0000-0000-0000-000000000000 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runOncallEscalationsGet,
}

func runOncallEscalationsGet(cmd *cobra.Command, args []string) error {
	policyID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	if oncallEscalationsGetInclude != "" {
		params.Set("include", oncallEscalationsGetInclude)
	}

	respBytes, err := client.Get("/api/v2/on-call/escalation-policies/"+policyID, params)
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

	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		data = raw
	}
	attrs, _ := data["attributes"].(map[string]interface{})
	if attrs == nil {
		attrs = map[string]interface{}{}
	}

	id := oncallStringField(data, "id")
	name := oncallStringField(attrs, "name")
	resolvePageOnPolicyEnd := ""
	if v, ok := attrs["resolve_page_on_policy_end"].(bool); ok {
		if v {
			resolvePageOnPolicyEnd = "Yes"
		} else {
			resolvePageOnPolicyEnd = "No"
		}
	}
	retries := ""
	if v, ok := attrs["retries"].(float64); ok {
		retries = fmt.Sprintf("%.0f", v)
	}

	teams := oncallExtractIncludedNames(raw, "teams")

	// Count steps
	steps := ""
	if rels, ok := data["relationships"].(map[string]interface{}); ok {
		if stepsRel, ok := rels["steps"].(map[string]interface{}); ok {
			if stepsData, ok := stepsRel["data"].([]interface{}); ok {
				steps = fmt.Sprintf("%d", len(stepsData))
			}
		}
	}

	_, _ = fmt.Fprintf(os.Stdout, "Escalation Policy: %s\n\n", name)

	type detailRow struct {
		Field string
		Value string
	}

	details := []struct{ k, v string }{
		{"ID", id},
		{"Name", name},
		{"Retries", retries},
		{"Resolve on Policy End", resolvePageOnPolicyEnd},
		{"Steps", steps},
		{"Teams", teams},
	}

	rows := make([]detailRow, 0, len(details))
	tableRows := make([][]string, 0, len(details))
	for _, d := range details {
		if d.v == "" {
			continue
		}
		rows = append(rows, detailRow{Field: d.k, Value: d.v})
		tableRows = append(tableRows, []string{d.k, d.v})
	}

	cols := []output.ColumnConfig{
		{Name: "Field", Width: 25},
		{Name: "Value", Width: 60},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- oncall teams sub-group ----

var oncallTeamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "Query On-Call team information",
	Long:  `Query Datadog On-Call team on-call users and routing rules.`,
	Example: `  # Show who is on-call for a team
  datadog-cli oncall teams who 27590dae-47be-4a7d-9abf-8f4e45124020

  # Get routing rules for a team
  datadog-cli oncall teams routing-rules 27590dae-47be-4a7d-9abf-8f4e45124020`,
}

// ---- oncall teams who ----

var oncallTeamsWhoInclude string

var oncallTeamsWhoCmd = &cobra.Command{
	Use:   "who <team_id>",
	Short: "Show who is on-call for a team",
	Long: `Show who is currently on-call for a team.

Uses GET /api/v2/on-call/teams/{team_id}/on-call.`,
	Example: `  # Show who is on-call for a team
  datadog-cli oncall teams who 27590dae-47be-4a7d-9abf-8f4e45124020

  # Include responders and escalation details
  datadog-cli oncall teams who 27590dae-47be-4a7d-9abf-8f4e45124020 --include "responders,escalations"

  # Output as JSON
  datadog-cli oncall teams who 27590dae-47be-4a7d-9abf-8f4e45124020 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runOncallTeamsWho,
}

func runOncallTeamsWho(cmd *cobra.Command, args []string) error {
	teamID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	include := oncallTeamsWhoInclude
	if include == "" {
		include = "responders"
	}
	params.Set("include", include)

	respBytes, err := client.Get("/api/v2/on-call/teams/"+teamID+"/on-call", params)
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

	// Response shape: TeamOnCallResponders — data.relationships.responders with included users
	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		data = raw
	}

	type responderRow struct {
		Name   string
		Email  string
		Handle string
	}

	var rows []responderRow
	var tableRows [][]string

	// Extract responder IDs from relationships
	responderIDs := oncallExtractRelationshipIDs(data, "responders")

	// Build a map from included user ID to user attributes
	includedMap := oncallBuildIncludedMap(raw)

	for _, rid := range responderIDs {
		userAttrs, ok := includedMap[rid]
		if !ok {
			// Just show the ID if we can't find attributes
			rows = append(rows, responderRow{Name: rid})
			tableRows = append(tableRows, []string{rid, "", ""})
			continue
		}
		name := oncallStringField(userAttrs, "name")
		email := oncallStringField(userAttrs, "email")
		handle := oncallStringField(userAttrs, "handle")
		rows = append(rows, responderRow{Name: name, Email: email, Handle: handle})
		tableRows = append(tableRows, []string{name, email, handle})
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No on-call users found for this team.")
		return nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "On-call for team %s:\n\n", teamID)

	cols := []output.ColumnConfig{
		{Name: "Name", Width: 30},
		{Name: "Email", Width: 40},
		{Name: "Handle", Width: 25},
	}

	return output.RenderTable(cols, tableRows, rows, opts)
}

// ---- oncall teams routing-rules ----

var oncallTeamsRoutingRulesInclude string

var oncallTeamsRoutingRulesCmd = &cobra.Command{
	Use:   "routing-rules <team_id>",
	Short: "Get On-Call routing rules for a team",
	Long: `Get the On-Call routing rules for a team.

Uses GET /api/v2/on-call/teams/{team_id}/routing-rules.`,
	Example: `  # Get routing rules for a team
  datadog-cli oncall teams routing-rules 27590dae-47be-4a7d-9abf-8f4e45124020

  # Include rules and policies
  datadog-cli oncall teams routing-rules 27590dae-47be-4a7d-9abf-8f4e45124020 --include "rules,rules.policy"

  # Output as JSON
  datadog-cli oncall teams routing-rules 27590dae-47be-4a7d-9abf-8f4e45124020 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runOncallTeamsRoutingRules,
}

func runOncallTeamsRoutingRules(cmd *cobra.Command, args []string) error {
	teamID := args[0]
	client := newClient()
	opts := GetOutputOptions()

	params := url.Values{}
	if oncallTeamsRoutingRulesInclude != "" {
		params.Set("include", oncallTeamsRoutingRulesInclude)
	}

	respBytes, err := client.Get("/api/v2/on-call/teams/"+teamID+"/routing-rules", params)
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

	// Response shape: TeamRoutingRules — data with relationships.rules
	data, _ := raw["data"].(map[string]interface{})
	if data == nil {
		data = raw
	}

	// Count routing rules
	ruleIDs := oncallExtractRelationshipIDs(data, "rules")

	type ruleRow struct {
		RuleID string
		Policy string
	}

	var rows []ruleRow
	var tableRows [][]string

	includedMap := oncallBuildIncludedMap(raw)
	_ = includedMap // used for policy name lookup when included

	for i, rid := range ruleIDs {
		policyName := ""
		// Try to get policy name from included
		if ruleAttrs, ok := includedMap[rid]; ok {
			if policyRel, ok := ruleAttrs["policy"].(map[string]interface{}); ok {
				if policyID, ok := policyRel["id"].(string); ok {
					if policyAttrs, ok := includedMap[policyID]; ok {
						policyName = oncallStringField(policyAttrs, "name")
					}
					if policyName == "" {
						policyName = policyID
					}
				}
			}
		}
		label := fmt.Sprintf("Rule %d", i+1)
		rows = append(rows, ruleRow{RuleID: rid, Policy: policyName})
		tableRows = append(tableRows, []string{label, rid, policyName})
	}

	if len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No routing rules found for this team.")
		return nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "Routing rules for team %s:\n\n", teamID)

	type ruleDisplayRow struct {
		Rule   string
		RuleID string
		Policy string
	}

	displayRows := make([]ruleDisplayRow, 0, len(rows))
	for i, r := range rows {
		displayRows = append(displayRows, ruleDisplayRow{
			Rule:   fmt.Sprintf("Rule %d", i+1),
			RuleID: r.RuleID,
			Policy: r.Policy,
		})
	}

	cols := []output.ColumnConfig{
		{Name: "Rule", Width: 8},
		{Name: "Rule ID", Width: 40},
		{Name: "Policy", Width: 40},
	}

	return output.RenderTable(cols, tableRows, displayRows, opts)
}

// ---- helpers ----

// oncallStringField safely extracts a string value from a map.
func oncallStringField(m map[string]interface{}, key string) string {
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

// oncallExtractIncludedNames extracts names of included resources of a given type.
func oncallExtractIncludedNames(raw map[string]interface{}, resourceType string) string {
	included, _ := raw["included"].([]interface{})
	var names []string
	for _, item := range included {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		t, _ := m["type"].(string)
		if t != resourceType {
			continue
		}
		attrs, _ := m["attributes"].(map[string]interface{})
		name := oncallStringField(attrs, "name")
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// oncallBuildIncludedMap builds a map from resource ID to its attributes/full object
// from the "included" array. Useful for resolving relationships.
func oncallBuildIncludedMap(raw map[string]interface{}) map[string]map[string]interface{} {
	result := map[string]map[string]interface{}{}
	included, _ := raw["included"].([]interface{})
	for _, item := range included {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		attrs, _ := m["attributes"].(map[string]interface{})
		if attrs == nil {
			attrs = map[string]interface{}{}
		}
		result[id] = attrs
	}
	return result
}

// oncallExtractRelationshipIDs extracts IDs from a named relationship on a data object.
func oncallExtractRelationshipIDs(data map[string]interface{}, relName string) []string {
	rels, _ := data["relationships"].(map[string]interface{})
	if rels == nil {
		return nil
	}
	rel, _ := rels[relName].(map[string]interface{})
	if rel == nil {
		return nil
	}
	relData, _ := rel["data"].([]interface{})
	var ids []string
	for _, item := range relData {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		if id, ok := m["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// oncallExtractShiftUser extracts user details from a Shift response.
// It looks in the included array for the user referenced in data.relationships.user.
func oncallExtractShiftUser(raw, data map[string]interface{}) (email, name, handle string) {
	// Get the user ID from relationships
	rels, _ := data["relationships"].(map[string]interface{})
	if rels != nil {
		if userRel, ok := rels["user"].(map[string]interface{}); ok {
			if userData, ok := userRel["data"].(map[string]interface{}); ok {
				userID, _ := userData["id"].(string)
				// Look in included
				includedMap := oncallBuildIncludedMap(raw)
				if attrs, ok := includedMap[userID]; ok {
					email = oncallStringField(attrs, "email")
					name = oncallStringField(attrs, "name")
					handle = oncallStringField(attrs, "handle")
					return
				}
			}
		}
	}

	// Fallback: check data.attributes directly (some APIs embed user info)
	attrs, _ := data["attributes"].(map[string]interface{})
	if attrs != nil {
		email = oncallStringField(attrs, "email")
		name = oncallStringField(attrs, "name")
		handle = oncallStringField(attrs, "handle")
	}
	return
}

// ---- init ----

func init() {
	// oncall schedules get flags
	oncallSchedulesGetCmd.Flags().StringVar(&oncallSchedulesGetInclude, "include", "", "Comma-separated relationships to include (teams,layers,layers.members,layers.members.user)")

	// oncall schedules who flags
	oncallSchedulesWhoCmd.Flags().StringVar(&oncallSchedulesWhoAt, "at", "", "RFC3339 timestamp to query on-call at (e.g. 2025-05-07T02:53:01Z); defaults to now")

	// oncall escalations get flags
	oncallEscalationsGetCmd.Flags().StringVar(&oncallEscalationsGetInclude, "include", "", "Comma-separated relationships to include (teams,steps,steps.targets)")

	// oncall teams who flags
	oncallTeamsWhoCmd.Flags().StringVar(&oncallTeamsWhoInclude, "include", "", "Comma-separated relationships to include (responders,escalations,escalations.responders)")

	// oncall teams routing-rules flags
	oncallTeamsRoutingRulesCmd.Flags().StringVar(&oncallTeamsRoutingRulesInclude, "include", "", "Comma-separated relationships to include (rules,rules.policy)")

	// Wire up subcommands
	oncallSchedulesCmd.AddCommand(oncallSchedulesGetCmd)
	oncallSchedulesCmd.AddCommand(oncallSchedulesWhoCmd)

	oncallEscalationsCmd.AddCommand(oncallEscalationsGetCmd)

	oncallTeamsCmd.AddCommand(oncallTeamsWhoCmd)
	oncallTeamsCmd.AddCommand(oncallTeamsRoutingRulesCmd)

	oncallCmd.AddCommand(oncallSchedulesCmd)
	oncallCmd.AddCommand(oncallEscalationsCmd)
	oncallCmd.AddCommand(oncallTeamsCmd)

	// Register with root
	rootCmd.AddCommand(oncallCmd)
}
