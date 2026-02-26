package auth

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	defaultSite    = "datadoghq.com"
	configDir      = ".datadog-cli"
	configFileName = "config.yaml"
	envAPIKey      = "DD_API_KEY"
	envAppKey      = "DD_APP_KEY"
	envSite        = "DD_SITE"
)

// ValidSites maps Datadog site hostnames to their region names.
var ValidSites = map[string]string{
	"datadoghq.com":     "US1 (default)",
	"us3.datadoghq.com": "US3",
	"us5.datadoghq.com": "US5",
	"datadoghq.eu":      "EU",
	"ap1.datadoghq.com": "AP1",
	"ap2.datadoghq.com": "AP2",
	"ddog-gov.com":      "US1-FED (Government)",
}

// Credentials holds Datadog API credentials.
type Credentials struct {
	APIKey string
	AppKey string
	Site   string
}

// Profile represents a named credential profile in the config file.
type Profile struct {
	APIKey string `yaml:"api_key"`
	AppKey string `yaml:"app_key"`
	Site   string `yaml:"site"`
}

// Config is the structure of ~/.datadog-cli/config.yaml.
//
// Example:
//
//	api_key: "key"
//	app_key: "key"
//	site: "datadoghq.com"
//	profiles:
//	  prod:
//	    api_key: "prod-key"
//	    app_key: "prod-key"
//	    site: "datadoghq.com"
type Config struct {
	APIKey   string             `yaml:"api_key"`
	AppKey   string             `yaml:"app_key"`
	Site     string             `yaml:"site"`
	Profiles map[string]Profile `yaml:"profiles"`
}

// ResolveCredentials resolves credentials from flags, environment variables, and config file.
//
// Priority order (highest to lowest):
//  1. CLI flags (flagAPIKey, flagAppKey, flagSite) if non-empty
//  2. Environment variables DD_API_KEY, DD_APP_KEY, DD_SITE
//  3. Config file (~/.datadog-cli/config.yaml)
//     - If flagProfile is set, load from profiles[flagProfile]
//     - Otherwise load from top-level api_key/app_key/site fields
//
// Default site is "datadoghq.com" if not set anywhere.
// Returns an error if neither API key nor app key is found after checking all sources.
func ResolveCredentials(flagAPIKey, flagAppKey, flagSite, flagProfile string) (*Credentials, error) {
	creds := &Credentials{}

	// Priority 3 (lowest): config file
	cfg, cfgErr := loadConfigFile()
	if cfgErr == nil {
		if flagProfile != "" {
			// Load from named profile
			if p, ok := cfg.Profiles[flagProfile]; ok {
				creds.APIKey = p.APIKey
				creds.AppKey = p.AppKey
				creds.Site = p.Site
			} else {
				// Profile specified but not found - collect available profiles for error message
				available := make([]string, 0, len(cfg.Profiles))
				for name := range cfg.Profiles {
					available = append(available, name)
				}
				if len(available) > 0 {
					return nil, fmt.Errorf("profile %q not found in config; available profiles: %v", flagProfile, available)
				}
				return nil, fmt.Errorf("profile %q not found in config; no profiles are defined", flagProfile)
			}
		} else {
			// Load from top-level config fields
			creds.APIKey = cfg.APIKey
			creds.AppKey = cfg.AppKey
			creds.Site = cfg.Site
		}
	}

	// Priority 2: environment variables (override config file)
	if v := os.Getenv(envAPIKey); v != "" {
		creds.APIKey = v
	}
	if v := os.Getenv(envAppKey); v != "" {
		creds.AppKey = v
	}
	if v := os.Getenv(envSite); v != "" {
		creds.Site = v
	}

	// Priority 1 (highest): CLI flags (override everything)
	if flagAPIKey != "" {
		creds.APIKey = flagAPIKey
	}
	if flagAppKey != "" {
		creds.AppKey = flagAppKey
	}
	if flagSite != "" {
		creds.Site = flagSite
	}

	// Apply default site if still unset
	if creds.Site == "" {
		creds.Site = defaultSite
	}

	// Validate that at least one key was found
	if creds.APIKey == "" && creds.AppKey == "" {
		return nil, fmt.Errorf(
			"no credentials found: set DD_API_KEY/DD_APP_KEY env vars, use --api-key/--app-key flags, or configure ~/.datadog-cli/config.yaml",
		)
	}

	return creds, nil
}

// Validate checks that all required credentials are present.
func Validate(creds *Credentials) error {
	if creds.APIKey == "" {
		return fmt.Errorf("API key is required: set DD_API_KEY env var, --api-key flag, or configure ~/.datadog-cli/config.yaml")
	}
	if creds.AppKey == "" {
		return fmt.Errorf("application key is required: set DD_APP_KEY env var, --app-key flag, or configure ~/.datadog-cli/config.yaml")
	}
	return nil
}

// loadConfigFile reads and parses the YAML config file at ~/.datadog-cli/config.yaml.
// Returns nil error and empty Config if the file does not exist.
func loadConfigFile() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}

	path := filepath.Join(home, configDir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return &cfg, nil
}

// ConfigPath returns the expected path to the config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, configDir, configFileName), nil
}

// APIBaseURL returns the Datadog API base URL for the given site.
// For example, "datadoghq.com" -> "https://api.datadoghq.com".
func APIBaseURL(site string) string {
	if site == "ddog-gov.com" {
		return "https://api.ddog-gov.com"
	}
	return fmt.Sprintf("https://api.%s", site)
}
