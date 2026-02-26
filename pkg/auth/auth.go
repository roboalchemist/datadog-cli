package auth

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	defaultSite       = "datadoghq.com"
	configDir         = ".datadog-cli"
	configFile        = "config.yaml"
	defaultProfile    = "default"
	envAPIKey         = "DD_API_KEY"
	envAppKey         = "DD_APP_KEY"
	envSite           = "DD_SITE"
)

// Credentials holds Datadog API credentials.
type Credentials struct {
	APIKey  string
	AppKey  string
	Site    string
	Profile string
}

// Profile represents a named credential profile in the config file.
type Profile struct {
	APIKey string `yaml:"api_key"`
	AppKey string `yaml:"app_key"`
	Site   string `yaml:"site"`
}

// Config is the structure of ~/.datadog-cli/config.yaml.
type Config struct {
	DefaultProfile string             `yaml:"default_profile"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

// Resolve resolves credentials from flags, environment variables, and config file.
// Priority: flags > env vars > config file.
func Resolve(flagAPIKey, flagAppKey, flagSite, flagProfile string) (*Credentials, error) {
	creds := &Credentials{
		APIKey:  flagAPIKey,
		AppKey:  flagAppKey,
		Site:    flagSite,
		Profile: flagProfile,
	}

	// Fill from env vars if not set by flags
	if creds.APIKey == "" {
		creds.APIKey = os.Getenv(envAPIKey)
	}
	if creds.AppKey == "" {
		creds.AppKey = os.Getenv(envAppKey)
	}
	if creds.Site == "" {
		creds.Site = os.Getenv(envSite)
	}

	// Fill from config file if still missing
	if creds.APIKey == "" || creds.AppKey == "" {
		cfg, err := loadConfig()
		if err == nil {
			profile := creds.Profile
			if profile == "" {
				profile = cfg.DefaultProfile
			}
			if profile == "" {
				profile = defaultProfile
			}
			if p, ok := cfg.Profiles[profile]; ok {
				if creds.APIKey == "" {
					creds.APIKey = p.APIKey
				}
				if creds.AppKey == "" {
					creds.AppKey = p.AppKey
				}
				if creds.Site == "" {
					creds.Site = p.Site
				}
			}
		}
	}

	// Apply default site
	if creds.Site == "" {
		creds.Site = defaultSite
	}

	return creds, nil
}

// Validate checks that required credentials are present.
func Validate(creds *Credentials) error {
	if creds.APIKey == "" {
		return fmt.Errorf("API key is required: set DD_API_KEY env var, --api-key flag, or configure ~/.datadog-cli/config.yaml")
	}
	if creds.AppKey == "" {
		return fmt.Errorf("application key is required: set DD_APP_KEY env var, --app-key flag, or configure ~/.datadog-cli/config.yaml")
	}
	return nil
}

// loadConfig reads the config file from ~/.datadog-cli/config.yaml.
func loadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}

	path := filepath.Join(home, configDir, configFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}

// ConfigPath returns the path to the config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}
