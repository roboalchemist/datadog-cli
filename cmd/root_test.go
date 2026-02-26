package cmd

import (
	"errors"
	"fmt"
	"testing"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/api"
	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/output"
)

// ---- global flags ----

func TestRootGlobalFlagsExist(t *testing.T) {
	pf := rootCmd.PersistentFlags()
	flags := []string{
		"json",
		"plaintext",
		"no-color",
		"debug",
		"verbose",
		"quiet",
		"silent",
		"limit",
		"profile",
		"site",
		"api-key",
		"app-key",
		"fields",
		"jq",
	}
	for _, name := range flags {
		if pf.Lookup(name) == nil {
			t.Errorf("expected persistent flag --%s to be registered, but it was not", name)
		}
	}
}

// ---- subcommand registration ----

func TestRootSubcommandsRegistered(t *testing.T) {
	want := []string{
		"logs",
		"traces",
		"apm",
		"hosts",
		"tags",
		"metrics",
		"monitors",
		"dashboards",
		"events",
		"incidents",
		"downtimes",
		"notebooks",
		"rum",
		"audit",
		"containers",
		"processes",
		"slos",
		"usage",
		"users",
		"pipelines",
		"api-keys",
		"auth",
		"docs",
		"completion",
		"skill",
	}

	registered := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		registered[c.Use] = true
		// cobra Use strings sometimes include argument descriptions, e.g. "completion [bash|zsh|fish|powershell]"
		// Extract just the first word for comparison purposes.
		if idx := len(c.Use); idx > 0 {
			for i, ch := range c.Use {
				if ch == ' ' {
					registered[c.Use[:i]] = true
					break
				}
			}
		}
	}

	for _, name := range want {
		if !registered[name] {
			t.Errorf("expected subcommand %q to be registered on rootCmd", name)
		}
	}
}

// ---- GetOutputOptions ----

func TestGetOutputOptionsDefaults(t *testing.T) {
	// Reset all flag vars to their defaults before testing.
	flagJSON = false
	flagPlaintext = false
	flagNoColor = false
	flagDebug = false
	flagFields = ""
	flagJQ = ""

	opts := GetOutputOptions()
	if opts != (output.Options{}) {
		t.Errorf("expected zero-value Options with all defaults, got %+v", opts)
	}
}

func TestGetOutputOptionsJSON(t *testing.T) {
	flagJSON = true
	flagPlaintext = false
	flagNoColor = false
	flagDebug = false
	flagFields = ""
	flagJQ = ""

	opts := GetOutputOptions()
	if !opts.JSON {
		t.Error("expected JSON=true")
	}
	if opts.Plaintext {
		t.Error("expected Plaintext=false")
	}

	flagJSON = false // restore
}

func TestGetOutputOptionsPlaintext(t *testing.T) {
	flagJSON = false
	flagPlaintext = true
	flagNoColor = false
	flagDebug = false
	flagFields = ""
	flagJQ = ""

	opts := GetOutputOptions()
	if !opts.Plaintext {
		t.Error("expected Plaintext=true")
	}
	flagPlaintext = false
}

func TestGetOutputOptionsNoColor(t *testing.T) {
	flagJSON = false
	flagPlaintext = false
	flagNoColor = true
	flagDebug = false
	flagFields = ""
	flagJQ = ""

	opts := GetOutputOptions()
	if !opts.NoColor {
		t.Error("expected NoColor=true")
	}
	flagNoColor = false
}

func TestGetOutputOptionsDebug(t *testing.T) {
	flagJSON = false
	flagPlaintext = false
	flagNoColor = false
	flagDebug = true
	flagFields = ""
	flagJQ = ""

	opts := GetOutputOptions()
	if !opts.Debug {
		t.Error("expected Debug=true")
	}
	flagDebug = false
}

func TestGetOutputOptionsFields(t *testing.T) {
	flagJSON = false
	flagPlaintext = false
	flagNoColor = false
	flagDebug = false
	flagFields = "name,status"
	flagJQ = ""

	opts := GetOutputOptions()
	if opts.Fields != "name,status" {
		t.Errorf("expected Fields=%q, got %q", "name,status", opts.Fields)
	}
	flagFields = ""
}

func TestGetOutputOptionsJQ(t *testing.T) {
	flagJSON = false
	flagPlaintext = false
	flagNoColor = false
	flagDebug = false
	flagFields = ""
	flagJQ = ".data[]"

	opts := GetOutputOptions()
	if opts.JQExpr != ".data[]" {
		t.Errorf("expected JQExpr=%q, got %q", ".data[]", opts.JQExpr)
	}
	flagJQ = ""
}

func TestGetOutputOptionsCombined(t *testing.T) {
	flagJSON = true
	flagPlaintext = false
	flagNoColor = true
	flagDebug = true
	flagFields = "id,name"
	flagJQ = ".results"

	opts := GetOutputOptions()
	want := output.Options{
		JSON:      true,
		Plaintext: false,
		NoColor:   true,
		Debug:     true,
		Fields:    "id,name",
		JQExpr:    ".results",
	}
	if opts != want {
		t.Errorf("expected %+v, got %+v", want, opts)
	}

	// restore
	flagJSON = false
	flagNoColor = false
	flagDebug = false
	flagFields = ""
	flagJQ = ""
}

// ---- exitCodeForError ----

func TestExitCodeForErrorServerError(t *testing.T) {
	err := &api.ServerError{Message: "server exploded"}
	if code := exitCodeForError(err); code != 3 {
		t.Errorf("ServerError: expected exit code 3, got %d", code)
	}
}

func TestExitCodeForErrorNetworkError(t *testing.T) {
	err := &api.NetworkError{Cause: errors.New("connection refused")}
	if code := exitCodeForError(err); code != 3 {
		t.Errorf("NetworkError: expected exit code 3, got %d", code)
	}
}

func TestExitCodeForErrorAuthenticationError(t *testing.T) {
	err := &api.AuthenticationError{Message: "invalid API key"}
	if code := exitCodeForError(err); code != 1 {
		t.Errorf("AuthenticationError: expected exit code 1, got %d", code)
	}
}

func TestExitCodeForErrorBadRequestError(t *testing.T) {
	err := &api.BadRequestError{Message: "bad request"}
	if code := exitCodeForError(err); code != 1 {
		t.Errorf("BadRequestError: expected exit code 1, got %d", code)
	}
}

func TestExitCodeForErrorNotFoundError(t *testing.T) {
	err := &api.NotFoundError{Message: "not found"}
	if code := exitCodeForError(err); code != 1 {
		t.Errorf("NotFoundError: expected exit code 1, got %d", code)
	}
}

func TestExitCodeForErrorRateLimitError(t *testing.T) {
	err := &api.RateLimitError{Message: "rate limited"}
	if code := exitCodeForError(err); code != 1 {
		t.Errorf("RateLimitError: expected exit code 1, got %d", code)
	}
}

func TestExitCodeForErrorGenericError(t *testing.T) {
	err := errors.New("some unknown error")
	if code := exitCodeForError(err); code != 1 {
		t.Errorf("generic error: expected exit code 1, got %d", code)
	}
}

func TestExitCodeForErrorWrappedServerError(t *testing.T) {
	inner := &api.ServerError{Message: "upstream failure"}
	wrapped := fmt.Errorf("operation failed: %w", inner)
	if code := exitCodeForError(wrapped); code != 3 {
		t.Errorf("wrapped ServerError: expected exit code 3, got %d", code)
	}
}

func TestExitCodeForErrorWrappedNetworkError(t *testing.T) {
	inner := &api.NetworkError{Cause: errors.New("timeout")}
	wrapped := fmt.Errorf("request failed: %w", inner)
	if code := exitCodeForError(wrapped); code != 3 {
		t.Errorf("wrapped NetworkError: expected exit code 3, got %d", code)
	}
}
