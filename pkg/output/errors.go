package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/api"
)

// CLIError is the structured error type emitted to stderr when --json is active.
type CLIError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// cliErrorFromErr maps a Go error to a CLIError with an appropriate code,
// recoverability flag, and optional suggestion.
func cliErrorFromErr(err error) CLIError {
	var authErr *api.AuthenticationError
	var rateLimitErr *api.RateLimitError
	var notFoundErr *api.NotFoundError
	var badReqErr *api.BadRequestError
	var serverErr *api.ServerError
	var networkErr *api.NetworkError

	switch {
	case errors.As(err, &authErr):
		return CLIError{
			Code:        "auth_error",
			Message:     err.Error(),
			Recoverable: false,
			Suggestion:  "Check DD_API_KEY and DD_APP_KEY environment variables or your config profile.",
		}
	case errors.As(err, &rateLimitErr):
		return CLIError{
			Code:        "rate_limit",
			Message:     err.Error(),
			Recoverable: true,
			Suggestion:  "Wait a moment and retry. Use --limit to reduce result set size.",
		}
	case errors.As(err, &notFoundErr):
		return CLIError{
			Code:        "not_found",
			Message:     err.Error(),
			Recoverable: false,
		}
	case errors.As(err, &badReqErr):
		return CLIError{
			Code:        "bad_request",
			Message:     err.Error(),
			Recoverable: false,
		}
	case errors.As(err, &serverErr):
		return CLIError{
			Code:        "server_error",
			Message:     err.Error(),
			Recoverable: true,
			Suggestion:  "The Datadog API returned a server error. Retry the request or check https://status.datadoghq.com.",
		}
	case errors.As(err, &networkErr):
		return CLIError{
			Code:        "network_error",
			Message:     err.Error(),
			Recoverable: true,
			Suggestion:  "Check your network connectivity and that DD_SITE is set correctly.",
		}
	default:
		return CLIError{
			Code:        "error",
			Message:     err.Error(),
			Recoverable: false,
		}
	}
}

// PrintErrorWithOpts prints a user-friendly error message to stderr.
// When opts.JSON is true a structured CLIError JSON object is emitted;
// otherwise the same plain-text format as PrintError is used.
func PrintErrorWithOpts(err error, opts Options) {
	if err == nil {
		return
	}

	if opts.JSON {
		ce := cliErrorFromErr(err)
		b, jsonErr := json.Marshal(ce)
		if jsonErr != nil {
			// Fallback: plain text if marshaling somehow fails.
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
		fmt.Fprintln(os.Stderr, string(b))
		return
	}

	// Plain-text path (same as PrintError).
	if color.NoColor || opts.NoColor {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "%s %v\n", color.RedString("Error:"), err)
	}
}
