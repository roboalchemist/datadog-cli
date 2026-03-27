//go:build !integration

package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

var (
	specV1Router routers.Router
	specV2Router routers.Router
)

func TestMain(m *testing.M) {
	// Find repo root relative to this test file's location.
	// runtime.Caller(0) returns the path of this source file at compile time,
	// so it works in both local and CI contexts regardless of working directory.
	_, f, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(f), "..")

	loader := openapi3.NewLoader()
	// Keep external refs disabled (default false) to avoid network calls during tests.

	// Load v1 spec.
	v1Path := filepath.Join(repoRoot, "specs", "v1.yaml")
	v1, err := loader.LoadFromFile(v1Path)
	if err != nil {
		// Non-fatal: log a warning but don't block CI if spec is missing.
		_, _ = os.Stderr.WriteString("spec_validator: WARNING: failed to load v1 spec: " + err.Error() + "\n")
	} else {
		// Use a relative server URL so kin-openapi matches any host in tests.
		v1.Servers = openapi3.Servers{{URL: "/"}}
		// DisableExamplesValidation skips example-value validation in the spec
		// itself — some Datadog spec examples have format errors that would
		// prevent the router from building.  Request parameter validation
		// (the thing we actually want) is unaffected.
		if r, err := legacy.NewRouter(v1, openapi3.DisableExamplesValidation()); err == nil {
			specV1Router = r
		} else {
			_, _ = os.Stderr.WriteString("spec_validator: WARNING: failed to build v1 router: " + err.Error() + "\n")
		}
	}

	// Load v2 spec.
	v2Path := filepath.Join(repoRoot, "specs", "v2.yaml")
	v2, err := loader.LoadFromFile(v2Path)
	if err != nil {
		_, _ = os.Stderr.WriteString("spec_validator: WARNING: failed to load v2 spec: " + err.Error() + "\n")
	} else {
		v2.Servers = openapi3.Servers{{URL: "/"}}
		if r, err := legacy.NewRouter(v2, openapi3.DisableExamplesValidation()); err == nil {
			specV2Router = r
		} else {
			_, _ = os.Stderr.WriteString("spec_validator: WARNING: failed to build v2 router: " + err.Error() + "\n")
		}
	}

	os.Exit(m.Run())
}

// validateRequestAgainstSpec validates an HTTP request against the appropriate
// OpenAPI spec (v1 or v2) based on the URL path prefix.
// Validation failures are reported via t.Errorf (non-fatal) so the mock server
// still responds and the test completes normally.
func validateRequestAgainstSpec(t *testing.T, r *http.Request) {
	t.Helper()

	// Pick the right router based on the path prefix.
	var router routers.Router
	if strings.HasPrefix(r.URL.Path, "/api/v2/") {
		router = specV2Router
	} else {
		router = specV1Router
	}
	if router == nil {
		// Spec not loaded; skip validation rather than failing.
		return
	}

	// The spec routers are built with server URL "/" (a relative URL).
	// kin-openapi only matches when the request URL is also relative (no host).
	// Httptest servers listen on http://127.0.0.1:PORT, so we build a
	// path-only clone of the request for FindRoute.
	pathOnlyURL := *r.URL
	pathOnlyURL.Scheme = ""
	pathOnlyURL.Host = ""
	pathOnlyReq := r.Clone(r.Context())
	pathOnlyReq.URL = &pathOnlyURL
	pathOnlyReq.RequestURI = r.URL.RequestURI()

	// Find the matching route in the spec.
	route, pathParams, err := router.FindRoute(pathOnlyReq)
	if err != nil {
		t.Errorf("spec validation: no matching route for %s %s: %v", r.Method, r.URL.Path, err)
		return
	}

	// Validate the request parameters (query, path, headers) against the spec.
	input := &openapi3filter.RequestValidationInput{
		Request:    pathOnlyReq,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			// Skip body validation — mock requests rarely carry a body.
			ExcludeRequestBody: true,
			// Collect all errors rather than stopping at the first one.
			MultiError: true,
			// Skip security/auth validation — test requests don't carry real API keys.
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}
	if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
		t.Errorf("spec validation: request %s %s failed: %v", r.Method, r.URL.Path, err)
	}
}
