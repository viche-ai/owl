package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/agents"
	"github.com/viche-ai/owl/internal/config"
)

var explainCmd = &cobra.Command{
	Use:   "explain <agent>",
	Short: "Show the resolved prompt stack for an agent definition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		scope, _ := cmd.Flags().GetString("scope")

		if scope != "" && scope != "project" && scope != "global" {
			fmt.Fprintln(os.Stderr, `Error: --scope must be "project" or "global"`)
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(name, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		globalCfg, _ := config.Load()
		projectCfg, _ := config.LoadProjectConfig(cwd)

		stack := agents.BuildPromptStack(def, globalCfg, projectCfg, "")
		fmt.Print(stack.Explain())
	},
}

func init() {
	explainCmd.Flags().String("scope", "", "Restrict resolution to scope (project|global)")
	rootCmd.AddCommand(explainCmd)
}
