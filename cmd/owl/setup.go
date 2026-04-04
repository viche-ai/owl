package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/config"
)

// setupCmd launches the interactive configuration wizard for first-time setup.
var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive configuration wizard",
	Run:   runSetup,
}

// providerOption pairs a provider name with its default model identifier.
type providerOption struct {
	name  string
	model string
}

// providers defines the supported LLM providers and their recommended default models.
var providers = []providerOption{
	{"openai", "gpt-4o"},
	{"anthropic", "claude-sonnet-4-6"},
	{"google", "gemini-2.0-flash"},
	{"ollama", "llama3.3"},
	{"custom", ""},
}

// runSetup executes the interactive setup wizard using huh forms.
// It prompts for the default provider, API key, optional custom model,
// and optional Viche registry token, then writes the result to ~/.owl/config.json.
func runSetup(cmd *cobra.Command, args []string) {
	var selectedProvider string
	var apiKey string
	var vicheToken string
	var customModel string

	// Build provider selection options for the huh form.
	providerOptions := make([]huh.Option[string], len(providers))
	for i, p := range providers {
		providerOptions[i] = huh.NewOption(p.name, p.name)
	}

	// Construct the multi-step setup form.
	// Step 1: Provider selection dropdown.
	// Step 2: API key input (required, masked).
	// Step 3: Custom model input (only relevant if "custom" provider selected).
	// Step 4: Optional Viche registry token with warning about public registry.
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select Default Provider").
				Description("Choose the LLM provider you want to use as default").
				Options(providerOptions...).
				Value(&selectedProvider),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("API Key").
				Description("Enter your API key for "+selectedProvider).
				Prompt("Key: ").
				Value(&apiKey).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Custom Model (optional)").
				Description("Enter a custom model identifier if using 'custom' provider").
				Value(&customModel),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Viche Registry Token (optional)").
				Description("Join a private registry for secure agent messaging. Skip to use the public global registry (possibly insecure). Sign up at viche.ai/signup").
				Prompt("Token: ").
				Value(&vicheToken),
		),
	)

	if err := form.WithTheme(huh.ThemeCatppuccin()).Run(); err != nil {
		fmt.Println("Error running setup wizard:", err)
		os.Exit(1)
	}

	// Load existing config (or create a fresh one if none exists).
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ProviderConfig)
	}

	// Resolve the default model for the selected provider.
	// For "custom" provider, use the user-provided model identifier.
	defaultModel := ""
	for _, p := range providers {
		if p.name == selectedProvider {
			if p.name == "custom" {
				defaultModel = customModel
			} else {
				defaultModel = p.model
			}
			break
		}
	}

	// Persist the selected provider and API key to config.
	cfg.Models.Default = selectedProvider + "/" + defaultModel
	cfg.Models.Providers[selectedProvider] = config.ProviderConfig{
		APIKey: apiKey,
	}

	// If a Viche token was provided, register it as the default.
	if vicheToken != "" {
		cfg.Viche.DefaultRegistry = vicheToken
		// Avoid duplicate registry entries.
		for _, r := range cfg.Viche.Registries {
			if r.Token == vicheToken {
				vicheToken = ""
				break
			}
		}
		if vicheToken != "" {
			cfg.Viche.Registries = append(cfg.Viche.Registries, config.RegistryConfig{
				Token: vicheToken,
			})
		}
	}

	if err := config.Save(cfg); err != nil {
		fmt.Println("Error saving config:", err)
		os.Exit(1)
	}

	fmt.Println("\nConfiguration saved successfully!")
	fmt.Printf("  Default model: %s\n", cfg.Models.Default)
	fmt.Printf("  Config file: %s\n", config.ConfigPath())
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
