package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage owl configuration",
}

var importPath string

var importCmd = &cobra.Command{
	Use:   "import [source]",
	Short: "Import configuration from another tool (opencode|openclaw)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		source := args[0]
		switch source {
		case "openclaw":
			if err := config.ImportFromOpenclaw(importPath); err != nil {
				fmt.Println("Error importing:", err)
				os.Exit(1)
			}
		case "opencode":
			if err := config.ImportFromOpencode(importPath); err != nil {
				fmt.Println("Error importing:", err)
				os.Exit(1)
			}
		default:
			fmt.Println("Unknown source. Currently supported: opencode, openclaw")
			os.Exit(1)
		}
	},
}

var setModelCmd = &cobra.Command{
	Use:   "set-model [provider/model]",
	Short: "Set the default model (e.g. anthropic/claude-sonnet-4-6)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}
		cfg.Models.Default = args[0]
		if err := config.Save(cfg); err != nil {
			fmt.Println("Error saving config:", err)
			os.Exit(1)
		}
		fmt.Printf("Default model set to: %s\n", args[0])
	},
}

var setKeyCmd = &cobra.Command{
	Use:   "set-key [provider] [api_key]",
	Short: "Set an API key for a provider (e.g. owl config set-key anthropic sk-ant-...)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		provider := args[0]
		key := args[1]

		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}

		pCfg := cfg.Models.Providers[provider]
		pCfg.APIKey = key
		cfg.Models.Providers[provider] = pCfg

		if err := config.Save(cfg); err != nil {
			fmt.Println("Error saving config:", err)
			os.Exit(1)
		}
		fmt.Printf("API key set for provider: %s\n", provider)
	},
}

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current owl configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}
		fmt.Printf("Config: %s\n\n", config.ConfigPath())
		if cfg.Models.Default != "" {
			fmt.Printf("Default model: %s\n", cfg.Models.Default)
		} else {
			fmt.Println("Default model: (not set)")
		}
		fmt.Printf("Providers:     %d configured\n", len(cfg.Models.Providers))
		for name, p := range cfg.Models.Providers {
			base := "(default)"
			if p.BaseURL != "" {
				base = p.BaseURL
			}
			key := "***"
			if p.APIKey == "" {
				key = "(none)"
			}
			fmt.Printf("  %-12s key: %s  base: %s\n", name, key, base)
		}
	},
}

func init() {
	importCmd.Flags().StringVarP(&importPath, "path", "p", "", "Optional path to the config file")
	configCmd.AddCommand(importCmd)
	configCmd.AddCommand(setModelCmd)
	configCmd.AddCommand(setKeyCmd)
	configCmd.AddCommand(showCmd)
	rootCmd.AddCommand(configCmd)
}
