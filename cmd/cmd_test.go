package cmd

import (
	"testing"
)

// commandIndex builds a map from command name to its registered subcommand names.
// This allows tests to verify both top-level groups and their children.
func commandIndex() map[string]map[string]bool {
	index := map[string]map[string]bool{}
	for _, c := range rootCmd.Commands() {
		// Use the first word of Use as the key (strips argument placeholders).
		name := firstWord(c.Use)
		subs := map[string]bool{}
		for _, sub := range c.Commands() {
			subs[firstWord(sub.Use)] = true
		}
		index[name] = subs
	}
	return index
}

func firstWord(s string) string {
	for i, ch := range s {
		if ch == ' ' {
			return s[:i]
		}
	}
	return s
}

// assertSubcmd checks that a top-level group and each of its expected subcommands
// are registered.
func assertSubcmd(t *testing.T, index map[string]map[string]bool, group string, subs ...string) {
	t.Helper()
	groupSubs, ok := index[group]
	if !ok {
		t.Errorf("command group %q not registered on rootCmd", group)
		return
	}
	for _, sub := range subs {
		if !groupSubs[sub] {
			t.Errorf("command %q %q: subcommand %q not registered", group, sub, sub)
		}
	}
}

// TestCommandGroupsAndSubcommands verifies every command group and its children.
func TestCommandGroupsAndSubcommands(t *testing.T) {
	idx := commandIndex()

	assertSubcmd(t, idx, "logs", "search", "aggregate", "indexes")
	assertSubcmd(t, idx, "traces", "search", "aggregate", "get")
	assertSubcmd(t, idx, "apm", "services", "definitions", "dependencies")
	assertSubcmd(t, idx, "hosts", "list", "totals")
	assertSubcmd(t, idx, "tags", "list", "get")
	assertSubcmd(t, idx, "metrics", "list", "query", "search")
	assertSubcmd(t, idx, "monitors", "list", "get", "search")
	assertSubcmd(t, idx, "dashboards", "list", "get", "search")
	assertSubcmd(t, idx, "events", "list", "get")
	assertSubcmd(t, idx, "incidents", "list", "get")
	assertSubcmd(t, idx, "downtimes", "list", "get")
	assertSubcmd(t, idx, "notebooks", "list", "get")
	assertSubcmd(t, idx, "rum", "search", "aggregate")
	assertSubcmd(t, idx, "audit", "search")
	assertSubcmd(t, idx, "containers", "list")
	assertSubcmd(t, idx, "processes", "list")
	assertSubcmd(t, idx, "slos", "list", "get", "history")
	assertSubcmd(t, idx, "usage", "summary", "top-metrics")
	assertSubcmd(t, idx, "users", "list", "get")
	assertSubcmd(t, idx, "pipelines", "list", "get")
	assertSubcmd(t, idx, "api-keys", "list")
	assertSubcmd(t, idx, "auth", "scopes")
	assertSubcmd(t, idx, "skill", "print", "add")
}

// TestRootCmdFindResolvesSubcommands verifies cobra can resolve nested commands.
func TestRootCmdFindResolvesSubcommands(t *testing.T) {
	cases := []struct {
		args []string
	}{
		{[]string{"logs", "search"}},
		{[]string{"logs", "aggregate"}},
		{[]string{"logs", "indexes"}},
		{[]string{"traces", "search"}},
		{[]string{"apm", "services"}},
		{[]string{"hosts", "list"}},
		{[]string{"metrics", "query"}},
		{[]string{"monitors", "list"}},
		{[]string{"dashboards", "list"}},
		{[]string{"events", "list"}},
		{[]string{"slos", "list"}},
		{[]string{"users", "list"}},
		{[]string{"auth", "scopes"}},
	}

	for _, tc := range cases {
		found, _, err := rootCmd.Find(tc.args)
		if err != nil {
			t.Errorf("rootCmd.Find(%v) error: %v", tc.args, err)
			continue
		}
		if found == nil {
			t.Errorf("rootCmd.Find(%v) returned nil command", tc.args)
			continue
		}
		// The resolved command should match the last element of the path.
		wantName := tc.args[len(tc.args)-1]
		gotName := firstWord(found.Use)
		if gotName != wantName {
			t.Errorf("rootCmd.Find(%v): resolved to %q, want %q", tc.args, gotName, wantName)
		}
	}
}

// TestAllTopLevelCommandsHaveShortDesc ensures every registered command has a non-empty Short description.
func TestAllTopLevelCommandsHaveShortDesc(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Short == "" {
			t.Errorf("command %q has empty Short description", firstWord(c.Use))
		}
	}
}

// TestAllSubcommandsHaveShortDesc ensures every subcommand also has a Short description.
func TestAllSubcommandsHaveShortDesc(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		for _, sub := range c.Commands() {
			if sub.Short == "" {
				t.Errorf("command %q %q has empty Short description",
					firstWord(c.Use), firstWord(sub.Use))
			}
		}
	}
}

// TestTagsSubcommands checks the tags command specifically since it has list/get.
func TestTagsSubcommands(t *testing.T) {
	idx := commandIndex()
	assertSubcmd(t, idx, "tags", "list", "get")
}

// TestAuthSubcommands checks auth has the scopes subcommand.
func TestAuthSubcommands(t *testing.T) {
	idx := commandIndex()
	assertSubcmd(t, idx, "auth", "scopes")
}

// TestCompletionCommandRegistered verifies the completion command is present.
func TestCompletionCommandRegistered(t *testing.T) {
	idx := commandIndex()
	if _, ok := idx["completion"]; !ok {
		t.Error("completion command not registered")
	}
}

// TestDocsCommandRegistered verifies the docs command is present.
func TestDocsCommandRegistered(t *testing.T) {
	idx := commandIndex()
	if _, ok := idx["docs"]; !ok {
		t.Error("docs command not registered")
	}
}

// TestSkillCommandRegistered verifies the skill command has print and add subcommands.
func TestSkillCommandRegistered(t *testing.T) {
	idx := commandIndex()
	assertSubcmd(t, idx, "skill", "print", "add")
}
