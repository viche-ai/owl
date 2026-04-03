package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/config"
)

var vicheCmd = &cobra.Command{
	Use:   "viche",
	Short: "Manage Viche network settings",
}

var vicheAddRegistryCmd = &cobra.Command{
	Use:   "add-registry <token>",
	Short: "Add a private Viche registry token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		token := args[0]
		url, _ := cmd.Flags().GetString("url")

		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}

		// Check if it already exists
		for _, r := range cfg.Viche.Registries {
			if r.Token == token {
				fmt.Println("Registry already configured.")
				return
			}
		}

		cfg.Viche.Registries = append(cfg.Viche.Registries, config.RegistryConfig{
			Token: token,
			URL:   url,
		})

		// If this is the first private registry, auto-set as default
		if len(cfg.Viche.Registries) == 1 {
			cfg.Viche.DefaultRegistry = token
			fmt.Println("Set as default registry.")
		}

		if err := config.Save(cfg); err != nil {
			fmt.Println("Error saving config:", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Registry added: %s\n", token[:min(8, len(token))]+"...")
		fmt.Println("All new agents will now connect to this registry.")
	},
}

var vicheSetDefaultCmd = &cobra.Command{
	Use:   "set-default <token>",
	Short: "Set the default Viche registry",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		token := args[0]

		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}

		// Verify it exists
		found := false
		for _, r := range cfg.Viche.Registries {
			if r.Token == token {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Registry %s not found. Add it first with: owl viche add-registry %s\n", token, token)
			os.Exit(1)
		}

		cfg.Viche.DefaultRegistry = token
		if err := config.Save(cfg); err != nil {
			fmt.Println("Error saving config:", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Default registry set to: %s\n", token[:min(8, len(token))]+"...")
	},
}

var vicheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current Viche registry configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Error loading config:", err)
			os.Exit(1)
		}

		if len(cfg.Viche.Registries) == 0 {
			fmt.Println("No private registries configured.")
			fmt.Println("Using: https://viche.ai (public)")
			fmt.Println("\nTo add a private registry:")
			fmt.Println("  owl viche add-registry <token>")
			return
		}

		fmt.Printf("Registries: %d configured\n\n", len(cfg.Viche.Registries))
		for _, r := range cfg.Viche.Registries {
			url := r.URL
			if url == "" {
				url = "https://viche.ai"
			}
			short := r.Token
			if len(short) > 8 {
				short = short[:8] + "..."
			}
			marker := ""
			if r.Token == cfg.Viche.DefaultRegistry {
				marker = " ← default"
			}
			fmt.Printf("  %s  (%s)%s\n", short, url, marker)
		}
	},
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	vicheAddRegistryCmd.Flags().String("url", "", "Custom Viche registry URL (defaults to https://viche.ai)")
	vicheCmd.AddCommand(vicheAddRegistryCmd)
	vicheCmd.AddCommand(vicheSetDefaultCmd)
	vicheCmd.AddCommand(vicheStatusCmd)
	rootCmd.AddCommand(vicheCmd)
}
