package llm

import (
	"fmt"
	"strings"

	"github.com/viche-ai/owl/internal/config"
)

// Known providers that speak OpenAI-compatible protocol
var openAICompatible = map[string]string{
	"openai":     "https://api.openai.com/v1",
	"google":     "https://generativelanguage.googleapis.com/v1beta/openai",
	"openrouter": "https://openrouter.ai/api/v1",
	"groq":       "https://api.groq.com/openai/v1",
	"together":   "https://api.together.xyz/v1",
	"deepseek":   "https://api.deepseek.com/v1",
	"mistral":    "https://api.mistral.ai/v1",
}

// Router resolves a "provider/model" string into the correct Provider and model name
type Router struct {
	providers map[string]Provider
}

// NewRouter builds providers from the owl config
func NewRouter(cfg *config.Config) *Router {
	r := &Router{
		providers: make(map[string]Provider),
	}

	for name, pCfg := range cfg.Models.Providers {
		if name == "anthropic" {
			r.providers[name] = NewAnthropicProvider(pCfg.APIKey, pCfg.BaseURL)
		} else if pCfg.BaseURL != "" {
			// Custom base URL = OpenAI-compatible endpoint
			r.providers[name] = NewOpenAIProvider(pCfg.APIKey, pCfg.BaseURL)
		} else if defaultURL, ok := openAICompatible[name]; ok {
			// Known OpenAI-compatible provider
			r.providers[name] = NewOpenAIProvider(pCfg.APIKey, defaultURL)
		} else {
			// Fallback: assume OpenAI-compatible
			r.providers[name] = NewOpenAIProvider(pCfg.APIKey, "")
		}
	}

	return r
}

// Resolve parses "provider/model" and returns the Provider + bare model name
func (r *Router) Resolve(modelID string) (Provider, string, error) {
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid model format %q — expected 'provider/model'", modelID)
	}

	providerName := parts[0]
	modelName := parts[1]

	if providerName == "cli" && modelName == "claude-code" {
		return NewClaudeCodeProvider(), "claude-code", nil
	}

	provider, ok := r.providers[providerName]
	if !ok {
		return nil, "", fmt.Errorf("no provider configured for %q. Run 'owl config import openclaw' or add it to ~/.owl/config.json", providerName)
	}

	return provider, modelName, nil
}

// ListProviders returns configured provider names
func (r *Router) ListProviders() []string {
	var names []string
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
