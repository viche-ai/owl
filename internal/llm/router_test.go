package llm

import (
	"testing"

	"github.com/viche-ai/owl/internal/config"
)

func TestRouter_NewRouter(t *testing.T) {
	cfg := &config.Config{
		Models: config.ModelsConfig{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {
					APIKey: "anthropic-key",
				},
				"openai": {
					APIKey: "openai-key",
				},
				"custom": {
					APIKey:  "custom-key",
					BaseURL: "http://custom-url.com",
				},
			},
		},
	}

	router := NewRouter(cfg)

	if len(router.providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(router.providers))
	}

	providers := router.ListProviders()
	if len(providers) != 3 {
		t.Fatalf("expected 3 providers from ListProviders, got %d", len(providers))
	}

	// Make sure custom provider is created properly
	if _, ok := router.providers["custom"]; !ok {
		t.Errorf("custom provider missing")
	}
}

func TestRouter_Resolve(t *testing.T) {
	cfg := &config.Config{
		Models: config.ModelsConfig{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {
					APIKey: "anthropic-key",
				},
			},
		},
	}

	router := NewRouter(cfg)

	tests := []struct {
		name        string
		modelID     string
		expectError bool
		expectModel string
	}{
		{
			name:        "valid format and provider",
			modelID:     "anthropic/claude-3-5-sonnet",
			expectError: false,
			expectModel: "claude-3-5-sonnet",
		},
		{
			name:        "invalid format",
			modelID:     "anthropic_claude",
			expectError: true,
		},
		{
			name:        "unknown provider",
			modelID:     "openai/gpt-4o",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, modelName, err := router.Resolve(tc.modelID)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for modelID %q, got none", tc.modelID)
				}
			} else {
				if err != nil {
					t.Fatalf("did not expect error for modelID %q, got: %v", tc.modelID, err)
				}
				if modelName != tc.expectModel {
					t.Errorf("expected model name %q, got %q", tc.expectModel, modelName)
				}
			}
		})
	}
}
