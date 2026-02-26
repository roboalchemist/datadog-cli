package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/roboalchemist/datadog-cli/pkg/api"
	"github.com/roboalchemist/datadog-cli/pkg/auth"
	"github.com/roboalchemist/datadog-cli/pkg/output"
)

// Package-level vars set by main.go via Set* functions.
var (
	version        string
	readmeContents string
	skillMD        string
	skillFS        fs.FS
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
	_ = commandsReference
	skillFS = skillFilesystem
}

// Global flag values
var (
	flagJSON      bool
	flagPlaintext bool
	flagNoColor   bool
	flagDebug     bool
	flagVerbose   bool
	flagQuiet     bool
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
  3. ~/.datadog-cli/config.yaml (profile-based)

Environment Variables:
  DD_API_KEY        Datadog API key (required)
  DD_APP_KEY        Datadog Application key (required)
  DD_SITE           Datadog site (default: datadoghq.com)
  DD_API_URL        Override API base URL
  NO_COLOR          Disable colored output

Files:
  ~/.datadog-cli/config.yaml   Credentials and profile configuration (mode 0600)

Exit Status:
  0   Success
  1   User/authentication error
  2   Usage error (invalid flags or arguments)
  3   System/network/server error

Report bugs to: https://github.com/roboalchemist/datadog-cli/issues
Home page: https://github.com/roboalchemist/datadog-cli`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute runs the root command.
func Execute() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\nCopyright © 2026 roboalchemist\nLicense MIT: <https://opensource.org/licenses/MIT>\n")

	// Handle -V (GNU standard short flag for --version)
	for _, arg := range os.Args[1:] {
		if arg == "-V" {
			fmt.Printf("datadog-cli %s\nCopyright © 2026 roboalchemist\nLicense MIT: <https://opensource.org/licenses/MIT>\n", version)
			return
		}
		if arg == "--" {
			break
		}
	}

	if err := rootCmd.Execute(); err != nil {
		output.PrintErrorWithOpts(err, GetOutputOptions())
		os.Exit(exitCodeForError(err))
	}
}

// exitCodeForError returns differentiated exit codes based on error type.
// 1 = user/auth error, 2 = usage error, 3 = system/network/server error.
func exitCodeForError(err error) int {
	var authErr *api.AuthenticationError
	var badReqErr *api.BadRequestError
	var notFoundErr *api.NotFoundError
	var rateLimitErr *api.RateLimitError
	var serverErr *api.ServerError
	var networkErr *api.NetworkError

	switch {
	case errors.As(err, &serverErr), errors.As(err, &networkErr):
		return 3
	case errors.As(err, &authErr), errors.As(err, &badReqErr),
		errors.As(err, &notFoundErr), errors.As(err, &rateLimitErr):
		return 1
	default:
		return 1
	}
}

func init() {
	pf := rootCmd.PersistentFlags()

	pf.BoolVarP(&flagJSON, "json", "j", false, "Output as JSON")
	pf.BoolVarP(&flagPlaintext, "plaintext", "p", false, "Plain text output (no color, no borders)")
	pf.BoolVar(&flagNoColor, "no-color", false, "Disable color output")
	pf.BoolVar(&flagDebug, "debug", false, "Enable debug logging")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	pf.BoolVar(&flagQuiet, "quiet", false, "Suppress progress output")
	pf.BoolVar(&flagQuiet, "silent", false, "Suppress progress output (synonym for --quiet)")
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
