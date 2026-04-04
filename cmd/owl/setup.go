package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/config"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive configuration wizard",
	Run:   runSetup,
}

type providerOption struct {
	name  string
	model string
}

var providers = []providerOption{
	{"openai", "gpt-4o"},
	{"anthropic", "claude-sonnet-4-6"},
	{"google", "gemini-2.0-flash"},
	{"ollama", "llama3.3"},
	{"custom", ""},
}

func runSetup(cmd *cobra.Command, args []string) {
	var selectedProvider string
	var apiKey string
	var vicheToken string
	var customModel string

	providerOptions := make([]huh.Option[string], len(providers))
	for i, p := range providers {
		providerOptions[i] = huh.NewOption(p.name, p.name)
	}

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

	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ProviderConfig)
	}

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

	cfg.Models.Default = selectedProvider + "/" + defaultModel
	cfg.Models.Providers[selectedProvider] = config.ProviderConfig{
		APIKey: apiKey,
	}

	if vicheToken != "" {
		cfg.Viche.DefaultRegistry = vicheToken
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
