package config

import (
	"testing"
)

func TestConfig_GetActiveRegistry(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		expectURL   string
		expectToken string
	}{
		{
			name: "no registries configured",
			cfg: &Config{
				Viche: VicheConfig{
					Registries: []RegistryConfig{},
				},
			},
			expectURL:   "https://viche.ai",
			expectToken: "",
		},
		{
			name: "one registry configured without default set",
			cfg: &Config{
				Viche: VicheConfig{
					Registries: []RegistryConfig{
						{Token: "token123", URL: "https://custom.registry"},
					},
				},
			},
			expectURL:   "https://custom.registry",
			expectToken: "token123",
		},
		{
			name: "multiple registries with default set",
			cfg: &Config{
				Viche: VicheConfig{
					DefaultRegistry: "token456",
					Registries: []RegistryConfig{
						{Token: "token123", URL: "https://custom1.registry"},
						{Token: "token456", URL: "https://custom2.registry"},
					},
				},
			},
			expectURL:   "https://custom2.registry",
			expectToken: "token456",
		},
		{
			name: "registry with empty URL should fallback to default",
			cfg: &Config{
				Viche: VicheConfig{
					Registries: []RegistryConfig{
						{Token: "token123", URL: ""},
					},
				},
			},
			expectURL:   "https://viche.ai",
			expectToken: "token123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, token := tc.cfg.GetActiveRegistry()
			if url != tc.expectURL {
				t.Errorf("expected URL %q, got %q", tc.expectURL, url)
			}
			if token != tc.expectToken {
				t.Errorf("expected Token %q, got %q", tc.expectToken, token)
			}
		})
	}
}
