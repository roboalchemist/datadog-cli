//go:build integration

package cmd

import (
	"os"
	"reflect"
	"testing"

	"gitea.roboalch.com/roboalchemist/datadog-cli/pkg/api"
)

// skipIfNoCredentials skips the test if DD_API_KEY or DD_APP_KEY are not set.
func skipIfNoCredentials(t *testing.T) {
	t.Helper()
	if os.Getenv("DD_API_KEY") == "" || os.Getenv("DD_APP_KEY") == "" {
		t.Skip("skipping integration test: DD_API_KEY and DD_APP_KEY must be set")
	}
}

// integrationClient returns a real Datadog API client pointed at api.datadoghq.com
// using credentials from DD_API_KEY and DD_APP_KEY environment variables.
func integrationClient() *api.Client {
	return api.NewClient(api.ClientConfig{
		APIKey: os.Getenv("DD_API_KEY"),
		AppKey: os.Getenv("DD_APP_KEY"),
		Site:   "datadoghq.com",
	})
}

// requireNonEmpty fails the test if items is an empty slice or nil.
// cmd is used in the failure message to identify which command produced no results.
func requireNonEmpty(t *testing.T, items interface{}, cmd string) {
	t.Helper()
	v := reflect.ValueOf(items)
	if !v.IsValid() || v.IsNil() {
		t.Fatalf("command %q returned nil; expected non-empty results from live API", cmd)
	}
	if v.Kind() == reflect.Slice && v.Len() == 0 {
		t.Fatalf("command %q returned empty slice; expected at least one result from live API", cmd)
	}
}
