// smoke_test.go — subprocess-based smoke tests that require no API key.
// These run with plain `go test -v ./...` or `make test`.
// No build tags are set, so they are always included.

package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// smokeBinaryPath is the path to the test binary built by TestMain.
var smokeBinaryPath string

// TestMain builds the binary once and runs all smoke tests.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "datadog-cli-smoke-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "smoke: failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	smokeBinaryPath = filepath.Join(tmp, "datadog-cli-smoke")

	buildCmd := exec.Command("go", "build", "-o", smokeBinaryPath, ".")
	buildCmd.Dir = projectRoot()
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "smoke: failed to build binary: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// projectRoot returns the working directory (project root where this file lives).
func projectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("smoke: cannot determine working directory: %v", err))
	}
	return wd
}

// runSmoke executes the smoke binary with args (no API keys set).
// Returns stdout, stderr, and any error.
func runSmoke(args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(smokeBinaryPath, args...)
	// Explicitly unset API key env vars so commands that don't need them
	// won't accidentally pick up credentials from the environment.
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "DD_API_KEY=") && !strings.HasPrefix(e, "DD_APP_KEY=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

// TestSmokeBuild verifies `make build` succeeds.
func TestSmokeBuild(t *testing.T) {
	t.Log("building binary via 'make build'")
	cmd := exec.Command("make", "build")
	cmd.Dir = projectRoot()
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Errorf("make build failed: %v\nstdout: %s\nstderr: %s", err, outBuf.String(), errBuf.String())
	}
}

// TestSmokeHelp verifies `--help` exits 0 and contains expected strings.
func TestSmokeHelp(t *testing.T) {
	stdout, stderr, err := runSmoke("--help")
	if err != nil {
		t.Errorf("--help failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "Usage:") {
		t.Errorf("--help output missing 'Usage:'\noutput: %s", combined)
	}
	// Spot-check a few command names that should always be present.
	for _, name := range []string{"logs", "metrics", "hosts", "monitors"} {
		if !strings.Contains(combined, name) {
			t.Errorf("--help output missing command %q\noutput: %s", name, combined)
		}
	}
}

// TestSmokeVersion verifies `--version` exits 0 and mentions "datadog-cli version".
func TestSmokeVersion(t *testing.T) {
	stdout, stderr, err := runSmoke("--version")
	if err != nil {
		t.Errorf("--version failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "datadog-cli version") {
		t.Errorf("--version output missing 'datadog-cli version'\noutput: %s", combined)
	}
}

// TestSmokeDocs verifies `docs` exits 0 and produces non-empty output.
func TestSmokeDocs(t *testing.T) {
	stdout, stderr, err := runSmoke("docs")
	if err != nil {
		t.Errorf("docs failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("docs: expected non-empty stdout\nstderr: %s", stderr)
	}
}

// TestSmokeCompletionBash verifies `completion bash` exits 0 with non-empty output.
func TestSmokeCompletionBash(t *testing.T) {
	stdout, stderr, err := runSmoke("completion", "bash")
	if err != nil {
		t.Errorf("completion bash failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("completion bash: expected non-empty output\nstderr: %s", stderr)
	}
}

// TestSmokeCompletionZsh verifies `completion zsh` exits 0.
func TestSmokeCompletionZsh(t *testing.T) {
	stdout, stderr, err := runSmoke("completion", "zsh")
	if err != nil {
		t.Errorf("completion zsh failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("completion zsh: expected non-empty output\nstderr: %s", stderr)
	}
}

// TestSmokeCompletionFish verifies `completion fish` exits 0.
func TestSmokeCompletionFish(t *testing.T) {
	stdout, stderr, err := runSmoke("completion", "fish")
	if err != nil {
		t.Errorf("completion fish failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("completion fish: expected non-empty output\nstderr: %s", stderr)
	}
}

// TestSmokeSkillPrint verifies `skill print` exits 0 with non-empty output.
func TestSmokeSkillPrint(t *testing.T) {
	stdout, stderr, err := runSmoke("skill", "print")
	if err != nil {
		t.Errorf("skill print failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("skill print: expected non-empty output\nstderr: %s", stderr)
	}
}

// TestSmokeAuthScopes verifies `auth scopes` exits 0 and contains scope info.
func TestSmokeAuthScopes(t *testing.T) {
	stdout, stderr, err := runSmoke("auth", "scopes")
	if err != nil {
		t.Errorf("auth scopes failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("auth scopes: expected non-empty output\nstderr: %s", stderr)
	}
	// The scopes table should mention scopes (permissions).
	if !strings.Contains(stdout, "Scope") && !strings.Contains(stdout, "scope") {
		t.Errorf("auth scopes: output does not appear to contain scope info\noutput: %s", stdout)
	}
}

// TestSmokeSubcommandHelp verifies that `<cmd> --help` exits 0 for every top-level command group.
func TestSmokeSubcommandHelp(t *testing.T) {
	// All top-level command groups that don't require API keys to show --help.
	commands := []string{
		"api-keys",
		"apm",
		"audit",
		"auth",
		"completion",
		"containers",
		"dashboards",
		"docs",
		"downtimes",
		"events",
		"hosts",
		"incidents",
		"logs",
		"metrics",
		"monitors",
		"notebooks",
		"pipelines",
		"processes",
		"rum",
		"skill",
		"slos",
		"tags",
		"traces",
		"usage",
		"users",
	}

	for _, cmd := range commands {
		cmd := cmd // capture
		t.Run(cmd, func(t *testing.T) {
			stdout, stderr, err := runSmoke(cmd, "--help")
			if err != nil {
				t.Errorf("%s --help failed: %v\nstdout: %s\nstderr: %s", cmd, err, stdout, stderr)
			}
			combined := stdout + stderr
			if strings.TrimSpace(combined) == "" {
				t.Errorf("%s --help: expected non-empty output", cmd)
			}
		})
	}
}
