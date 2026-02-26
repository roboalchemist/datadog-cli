package cmd

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/api"
	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/auth"
	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// Package-level vars set by main.go via Set* functions.
var (
	version         string
	readmeContents  string
	skillMD         string
	commandsRef     string
	skillFS         fs.FS
)

// SetVersion sets the binary version string (injected via ldflags).
func SetVersion(v string) {
	version = v
}

// SetReadmeContents sets the embedded README content.
func SetReadmeContents(s string) {
	readmeContents = s
}

// SetSkillData sets the embedded skill markdown and filesystem.
func SetSkillData(skillMarkdown, commandsReference string, skillFilesystem fs.FS) {
	skillMD = skillMarkdown
	commandsRef = commandsReference
	skillFS = skillFilesystem
}

// Global flag values
var (
	flagJSON      bool
	flagPlaintext bool
	flagNoColor   bool
	flagDebug     bool
	flagVerbose   bool
	flagLimit     int
	flagProfile   string
	flagSite      string
	flagAPIKey    string
	flagAppKey    string
	flagFields    string
	flagJQ        string
)

var rootCmd = &cobra.Command{
	Use:   "datadog-cli",
	Short: "Read-only CLI for the Datadog API",
	Long: `datadog-cli is a read-only command-line interface for querying the Datadog API.

Query logs, metrics, monitors, dashboards, hosts, APM traces, and more.

Credentials are resolved from (in priority order):
  1. --api-key / --app-key flags
  2. DD_API_KEY / DD_APP_KEY environment variables
  3. ~/.datadog-cli/config.yaml (profile-based)`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute runs the root command.
func Execute() {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		output.PrintError(err)
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()

	pf.BoolVarP(&flagJSON, "json", "j", false, "Output as JSON")
	pf.BoolVarP(&flagPlaintext, "plaintext", "p", false, "Plain text output (no color, no borders)")
	pf.BoolVar(&flagNoColor, "no-color", false, "Disable color output")
	pf.BoolVar(&flagDebug, "debug", false, "Enable debug logging")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	pf.IntVarP(&flagLimit, "limit", "l", 100, "Maximum number of results to return")
	pf.StringVar(&flagProfile, "profile", "", "Config profile name")
	pf.StringVar(&flagSite, "site", "", "Datadog site (default: datadoghq.com)")
	pf.StringVar(&flagAPIKey, "api-key", "", "Datadog API key")
	pf.StringVar(&flagAppKey, "app-key", "", "Datadog Application key")
	pf.StringVar(&flagFields, "fields", "", "Comma-separated list of fields to display")
	pf.StringVar(&flagJQ, "jq", "", "JQ expression to filter JSON output")
}

// GetOutputOptions returns output options based on the current flag values.
func GetOutputOptions() output.Options {
	return output.Options{
		JSON:      flagJSON,
		Plaintext: flagPlaintext,
		NoColor:   flagNoColor,
		Fields:    flagFields,
		JQExpr:    flagJQ,
		Debug:     flagDebug,
	}
}

// newClient resolves credentials from flags/env/config and returns an API client.
// If credentials cannot be resolved, it prints a user-friendly error and calls os.Exit(1).
func newClient() *api.DatadogClient {
	creds, err := auth.ResolveCredentials(flagAPIKey, flagAppKey, flagSite, flagProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return api.NewClient(api.ClientConfig{
		APIKey:  creds.APIKey,
		AppKey:  creds.AppKey,
		Site:    creds.Site,
		Verbose: flagVerbose,
		Debug:   flagDebug,
	})
}

// RootCmd exposes the root cobra command for use by external tools (e.g., gendocs).
var RootCmd = rootCmd
