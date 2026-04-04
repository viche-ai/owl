package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ProviderConfig struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl,omitempty"`
}

type ModelsConfig struct {
	Default   string                    `json:"default"`
	Providers map[string]ProviderConfig `json:"providers"`
}

type RegistryConfig struct {
	Token string `json:"token"`
	URL   string `json:"url,omitempty"` // defaults to https://viche.ai
}

type VicheConfig struct {
	// Default registry (used when hatching without --registry flag)
	DefaultRegistry string           `json:"defaultRegistry,omitempty"`
	Registries      []RegistryConfig `json:"registries"`
}

type Config struct {
	Models       ModelsConfig `json:"models"`
	Viche        VicheConfig  `json:"viche"`
	SystemPrompt string       `json:"system_prompt,omitempty"`
}

func DefaultSystemPrompt() string {
	return "You are a collaborative AI agent operating on the Viche network. You must proactively communicate with other agents to solve problems."
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".owl", "config.json")
}

func Load() (*Config, error) {
	path := ConfigPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				Models: ModelsConfig{
					Providers: make(map[string]ProviderConfig),
				},
			}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]ProviderConfig)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

// GetActiveRegistry returns the registry to use for agent connections.
// If no registries are configured, returns public (no token).
// If a default is set, returns that. Otherwise returns the first one.
func (cfg *Config) GetActiveRegistry() (url string, token string) {
	const defaultURL = "https://viche.ai"

	if len(cfg.Viche.Registries) == 0 {
		return defaultURL, "" // Public registry, no auth
	}

	// If a default is named, find it
	if cfg.Viche.DefaultRegistry != "" {
		for _, r := range cfg.Viche.Registries {
			if r.Token == cfg.Viche.DefaultRegistry {
				url := r.URL
				if url == "" {
					url = defaultURL
				}
				return url, r.Token
			}
		}
	}

	// Fallback to the first configured registry
	r := cfg.Viche.Registries[0]
	url = r.URL
	if url == "" {
		url = defaultURL
	}
	return url, r.Token
}

func ImportFromOpenclaw(customPath string) error {
	ocPath := customPath
	if ocPath == "" {
		home, _ := os.UserHomeDir()
		ocPath = filepath.Join(home, ".openclaw", "openclaw.json")
	}

	b, err := os.ReadFile(ocPath)
	if err != nil {
		return fmt.Errorf("could not read openclaw config: %w", err)
	}

	var ocConfig struct {
		Models struct {
			Providers map[string]ProviderConfig `json:"providers"`
		} `json:"models"`
	}

	if err := json.Unmarshal(b, &ocConfig); err != nil {
		return fmt.Errorf("failed to parse openclaw config: %w", err)
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	count := 0
	for provider, data := range ocConfig.Models.Providers {
		if data.APIKey != "" || data.BaseURL != "" {
			if cfg.Models.Providers == nil {
				cfg.Models.Providers = make(map[string]ProviderConfig)
			}
			cfg.Models.Providers[provider] = data
			count++
		}
	}

	if err := Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Successfully imported %d provider configs from OpenClaw (%s).\n", count, ocPath)
	return nil
}

func ImportFromOpencode(customPath string) error {
	var paths []string

	if customPath != "" {
		paths = []string{customPath}
	} else {
		home, _ := os.UserHomeDir()
		paths = []string{
			filepath.Join(home, ".opencode", "opencode.jsonc"),
			filepath.Join(home, ".config", "opencode", "opencode.json"),
		}
	}

	var b []byte
	var loadedPath string
	var err error

	for _, p := range paths {
		if b, err = os.ReadFile(p); err == nil {
			loadedPath = p
			break
		}
	}

	if loadedPath == "" {
		return fmt.Errorf("could not find opencode config. Please provide a path with --path")
	}

	var ocConfig struct {
		Providers map[string]ProviderConfig `json:"providers"`
	}

	err = json.Unmarshal(b, &ocConfig)
	if err != nil {
		ocConfig.Providers = make(map[string]ProviderConfig)
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	count := 0
	for provider, data := range ocConfig.Providers {
		if data.APIKey != "" || data.BaseURL != "" {
			if cfg.Models.Providers == nil {
				cfg.Models.Providers = make(map[string]ProviderConfig)
			}
			cfg.Models.Providers[provider] = data
			count++
		}
	}

	if err := Save(cfg); err != nil {
		return err
	}

	fmt.Printf("Successfully imported %d provider configs from Opencode (%s).\n", count, loadedPath)
	return nil
}

// AddRegistry adds a private registry token to the config.
// If it's the first registry, sets it as the default automatically.
func AddRegistry(token, url string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	for _, r := range cfg.Viche.Registries {
		if r.Token == token {
			return nil // already exists
		}
	}
	cfg.Viche.Registries = append(cfg.Viche.Registries, RegistryConfig{Token: token, URL: url})
	if len(cfg.Viche.Registries) == 1 {
		cfg.Viche.DefaultRegistry = token
	}
	return Save(cfg)
}

// SetDefaultRegistry sets the active default registry by token.
func SetDefaultRegistry(token string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	for _, r := range cfg.Viche.Registries {
		if r.Token == token {
			cfg.Viche.DefaultRegistry = token
			return Save(cfg)
		}
	}
	return fmt.Errorf("registry %q not found — add it first with: owl viche add-registry %s", token, token)
}
