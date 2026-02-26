package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// setHome overrides the HOME env var to tmpdir so loadConfigFile() reads
// from tmpdir/.datadog-cli/config.yaml.  Returns a cleanup function.
func setHome(t *testing.T, dir string) func() {
	t.Helper()
	orig := os.Getenv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	return func() {
		_ = os.Setenv("HOME", orig)
	}
}

// writeConfig writes YAML content to <dir>/.datadog-cli/config.yaml.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	cfgDir := filepath.Join(dir, ".datadog-cli")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", cfgDir, err)
	}
	path := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// clearAuthEnv clears DD_* env vars and returns a restore function.
func clearAuthEnv(t *testing.T) func() {
	t.Helper()
	saved := map[string]string{
		"DD_API_KEY": os.Getenv("DD_API_KEY"),
		"DD_APP_KEY": os.Getenv("DD_APP_KEY"),
		"DD_SITE":    os.Getenv("DD_SITE"),
	}
	for k := range saved {
		_ = os.Unsetenv(k)
	}
	return func() {
		for k, v := range saved {
			if v != "" {
				_ = os.Setenv(k, v)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}

// --- ResolveCredentials: env vars ---

func TestResolveCredentials_EnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir) // no config file
	defer restoreHome()

	_ = os.Setenv("DD_API_KEY", "env-api-key")
	_ = os.Setenv("DD_APP_KEY", "env-app-key")
	_ = os.Setenv("DD_SITE", "datadoghq.eu")

	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIKey != "env-api-key" {
		t.Errorf("APIKey = %q, want env-api-key", creds.APIKey)
	}
	if creds.AppKey != "env-app-key" {
		t.Errorf("AppKey = %q, want env-app-key", creds.AppKey)
	}
	if creds.Site != "datadoghq.eu" {
		t.Errorf("Site = %q, want datadoghq.eu", creds.Site)
	}
}

func TestResolveCredentials_DefaultSite(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	_ = os.Setenv("DD_API_KEY", "k")
	_ = os.Setenv("DD_APP_KEY", "a")

	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Site != "datadoghq.com" {
		t.Errorf("expected default site datadoghq.com, got %q", creds.Site)
	}
}

// --- ResolveCredentials: config file ---

func TestResolveCredentials_ConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `
api_key: file-api-key
app_key: file-app-key
site: us3.datadoghq.com
`)

	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIKey != "file-api-key" {
		t.Errorf("APIKey = %q, want file-api-key", creds.APIKey)
	}
	if creds.AppKey != "file-app-key" {
		t.Errorf("AppKey = %q, want file-app-key", creds.AppKey)
	}
	if creds.Site != "us3.datadoghq.com" {
		t.Errorf("Site = %q, want us3.datadoghq.com", creds.Site)
	}
}

// --- ResolveCredentials: profile selection ---

func TestResolveCredentials_ProfileSelection(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `
api_key: default-api
app_key: default-app
site: datadoghq.com
profiles:
  prod:
    api_key: prod-api-key
    app_key: prod-app-key
    site: datadoghq.eu
  staging:
    api_key: staging-api-key
    app_key: staging-app-key
    site: us3.datadoghq.com
`)

	creds, err := ResolveCredentials("", "", "", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIKey != "prod-api-key" {
		t.Errorf("APIKey = %q, want prod-api-key", creds.APIKey)
	}
	if creds.Site != "datadoghq.eu" {
		t.Errorf("Site = %q, want datadoghq.eu", creds.Site)
	}
}

func TestResolveCredentials_ProfileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `
api_key: default-api
app_key: default-app
profiles:
  prod:
    api_key: prod-api-key
    app_key: prod-app-key
`)

	_, err := ResolveCredentials("", "", "", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

func TestResolveCredentials_ProfileNotFound_NoProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `
api_key: default-api
app_key: default-app
`)

	_, err := ResolveCredentials("", "", "", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile with no profiles defined, got nil")
	}
}

// --- ResolveCredentials: priority chain ---

func TestResolveCredentials_FlagsOverrideEnv(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	_ = os.Setenv("DD_API_KEY", "env-api-key")
	_ = os.Setenv("DD_APP_KEY", "env-app-key")
	_ = os.Setenv("DD_SITE", "datadoghq.eu")

	creds, err := ResolveCredentials("flag-api-key", "flag-app-key", "ddog-gov.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIKey != "flag-api-key" {
		t.Errorf("APIKey = %q, want flag-api-key (flags override env)", creds.APIKey)
	}
	if creds.AppKey != "flag-app-key" {
		t.Errorf("AppKey = %q, want flag-app-key", creds.AppKey)
	}
	if creds.Site != "ddog-gov.com" {
		t.Errorf("Site = %q, want ddog-gov.com", creds.Site)
	}
}

func TestResolveCredentials_EnvOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `
api_key: file-api-key
app_key: file-app-key
site: datadoghq.com
`)

	_ = os.Setenv("DD_API_KEY", "env-api-key")
	_ = os.Setenv("DD_APP_KEY", "env-app-key")

	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIKey != "env-api-key" {
		t.Errorf("APIKey = %q, want env-api-key (env overrides file)", creds.APIKey)
	}
	if creds.AppKey != "env-app-key" {
		t.Errorf("AppKey = %q, want env-app-key", creds.AppKey)
	}
}

func TestResolveCredentials_FlagsOverrideFile(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `
api_key: file-api-key
app_key: file-app-key
site: datadoghq.com
`)

	creds, err := ResolveCredentials("flag-api-key", "flag-app-key", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.APIKey != "flag-api-key" {
		t.Errorf("APIKey = %q, want flag-api-key", creds.APIKey)
	}
}

// --- ResolveCredentials: missing credentials ---

func TestResolveCredentials_MissingCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	// No config file, no env vars, no flags
	_, err := ResolveCredentials("", "", "", "")
	if err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}
}

func TestResolveCredentials_OnlyAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	// Only API key provided (no app key) - should succeed since we only require one
	_ = os.Setenv("DD_API_KEY", "just-api-key")
	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("expected success with just API key, got: %v", err)
	}
	if creds.APIKey != "just-api-key" {
		t.Errorf("APIKey = %q, want just-api-key", creds.APIKey)
	}
}

func TestResolveCredentials_OnlyAppKey(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	_ = os.Setenv("DD_APP_KEY", "just-app-key")
	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("expected success with just app key, got: %v", err)
	}
	if creds.AppKey != "just-app-key" {
		t.Errorf("AppKey = %q, want just-app-key", creds.AppKey)
	}
}

// --- Validate ---

func TestValidate_BothKeys(t *testing.T) {
	creds := &Credentials{APIKey: "k", AppKey: "a", Site: "datadoghq.com"}
	if err := Validate(creds); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_MissingAPIKey(t *testing.T) {
	creds := &Credentials{AppKey: "a", Site: "datadoghq.com"}
	if err := Validate(creds); err == nil {
		t.Error("expected error for missing API key, got nil")
	}
}

func TestValidate_MissingAppKey(t *testing.T) {
	creds := &Credentials{APIKey: "k", Site: "datadoghq.com"}
	if err := Validate(creds); err == nil {
		t.Error("expected error for missing app key, got nil")
	}
}

// --- APIBaseURL ---

func TestAPIBaseURL_Standard(t *testing.T) {
	tests := []struct {
		site string
		want string
	}{
		{"datadoghq.com", "https://api.datadoghq.com"},
		{"datadoghq.eu", "https://api.datadoghq.eu"},
		{"us3.datadoghq.com", "https://api.us3.datadoghq.com"},
		{"us5.datadoghq.com", "https://api.us5.datadoghq.com"},
	}
	for _, tc := range tests {
		got := APIBaseURL(tc.site)
		if got != tc.want {
			t.Errorf("APIBaseURL(%q) = %q, want %q", tc.site, got, tc.want)
		}
	}
}

func TestAPIBaseURL_GovSite(t *testing.T) {
	got := APIBaseURL("ddog-gov.com")
	want := "https://api.ddog-gov.com"
	if got != want {
		t.Errorf("APIBaseURL(ddog-gov.com) = %q, want %q", got, want)
	}
}

// --- ConfigPath ---

func TestConfigPath_ReturnsPath(t *testing.T) {
	tmpDir := t.TempDir()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(tmpDir, ".datadog-cli", "config.yaml")
	if path != want {
		t.Errorf("ConfigPath() = %q, want %q", path, want)
	}
}

// --- loadConfigFile: no file ---

func TestLoadConfigFile_NoFile_ReturnsEmptyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	cfg, err := loadConfigFile()
	if err != nil {
		t.Fatalf("expected no error when config file missing, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.APIKey != "" || cfg.AppKey != "" {
		t.Errorf("expected empty config, got: %+v", cfg)
	}
}

// --- loadConfigFile: invalid YAML ---

func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	writeConfig(t, tmpDir, `{ this is: [invalid yaml`)

	_, err := loadConfigFile()
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// --- ConfigPath: error when HOME unset ---
// Note: os.UserHomeDir() uses HOME on unix. We test it works with a valid dir here.

func TestConfigPath_ValidHome(t *testing.T) {
	tmpDir := t.TempDir()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	p, err := ConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == "" {
		t.Error("expected non-empty path")
	}
}

// --- Config file with partial fields ---

func TestResolveCredentials_ConfigFilePartialSite(t *testing.T) {
	tmpDir := t.TempDir()
	restore := clearAuthEnv(t)
	defer restore()
	restoreHome := setHome(t, tmpDir)
	defer restoreHome()

	// Config file has no site - should fall through to default
	writeConfig(t, tmpDir, `
api_key: file-api-key
app_key: file-app-key
`)

	creds, err := ResolveCredentials("", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Site != "datadoghq.com" {
		t.Errorf("expected default site when config has no site, got %q", creds.Site)
	}
}
