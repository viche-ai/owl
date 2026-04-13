package main

import (
	"encoding/json"
	"fmt"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/tui"
)

// resolveAgentTemplate loads a legacy template JSON from ~/.owl/templates/<name>.json.
// Returns the parsed Template and the path it was found at, or an error.
func resolveAgentTemplate(name string) (*Template, string, error) {
	home, _ := os.UserHomeDir()
	tmplPath := filepath.Join(home, ".owl", "templates", name+".json")
	b, err := os.ReadFile(tmplPath)
	if err != nil {
		return nil, tmplPath, err
	}
	var tmpl Template
	if err := json.Unmarshal(b, &tmpl); err != nil {
		return nil, tmplPath, fmt.Errorf("failed to parse template JSON: %w", err)
	}
	return &tmpl, tmplPath, nil
}

var rootCmd = &cobra.Command{
	Use:   "owl",
	Short: "Owl terminal coding tool",
	Run: func(cmd *cobra.Command, args []string) {
		tui.Run()
	},
}

var (
	modelFlag       string
	templateFlag    string // deprecated: use agentFlag
	agentFlag       string
	fromFileFlag    string
	scopeFlag       string
	dryRunFlag      bool
	registryFlag    string
	thinkingFlag    bool
	effortFlag      string
	nameFlag        string
	ambientFlag     bool
	dirFlag         string
	countFlag       int
	harnessFlag     string
	harnessArgsFlag string
	noNetInjectFlag bool
)

type Template struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"system_prompt"`
	Capabilities []string `json:"capabilities"`
	Model        string   `json:"model"`
	Thinking     bool     `json:"thinking"`
	Effort       string   `json:"effort"`
}

var hatchCmd = &cobra.Command{
	Use:   "hatch [description...]",
	Short: "Hatch a new agent",
	Run: func(cmd *cobra.Command, args []string) {
		// --template is deprecated: copy to agentFlag with warning
		if cmd.Flags().Changed("template") && !cmd.Flags().Changed("agent") {
			fmt.Fprintln(os.Stderr, "Warning: --template is deprecated, use --agent instead")
			agentFlag = templateFlag
		}

		desc := strings.Join(args, " ")

		// Tip for bare positional usage without --agent
		if len(args) > 0 && agentFlag == "" {
			fmt.Fprintln(os.Stderr, "Tip: For reproducible hatching, define an agent and use: owl hatch --agent <name>")
		}

		// --from-file overrides description with file contents
		if fromFileFlag != "" {
			b, err := os.ReadFile(fromFileFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: could not read file %q: %v\n", fromFileFlag, err)
				os.Exit(1)
			}
			desc = strings.TrimSpace(string(b))
		}

		// Validate --scope value
		if scopeFlag != "" && scopeFlag != "project" && scopeFlag != "global" {
			fmt.Fprintln(os.Stderr, "Error: --scope must be \"project\" or \"global\"")
			os.Exit(1)
		}

		// Default WorkDir to client's cwd so the daemon resolves
		// agent definitions and project config from the right directory.
		workDir := dirFlag
		if workDir == "" {
			workDir, _ = os.Getwd()
		}

		hatchArgs := ipc.HatchArgs{
			Description: desc,
			ModelID:     modelFlag,
			Agent:       agentFlag,
			Scope:       scopeFlag,
			DryRun:      dryRunFlag,
			Registry:    registryFlag,
			Thinking:    thinkingFlag,
			Effort:      effortFlag,
			Name:        nameFlag,
			Ambient:     ambientFlag,
			WorkDir:     workDir,
			Harness:     harnessFlag,
			HarnessArgs: harnessArgsFlag,
			NoNetInject: noNetInjectFlag,
		}

		// Resolve agent definition (project > global > legacy templates)
		if agentFlag != "" {
			home, _ := os.UserHomeDir()
			cwd, _ := os.Getwd()

			type candidate struct {
				scope string
				path  string
			}

			var candidates []candidate
			if scopeFlag == "" || scopeFlag == "project" {
				candidates = append(candidates, candidate{"project", filepath.Join(cwd, ".owl", "agents", agentFlag)})
			}
			if scopeFlag == "" || scopeFlag == "global" {
				candidates = append(candidates, candidate{"global", filepath.Join(home, ".owl", "agents", agentFlag)})
			}
			if scopeFlag == "" {
				candidates = append(candidates, candidate{"legacy", filepath.Join(home, ".owl", "templates", agentFlag+".json")})
			}

			resolved := false
			for _, c := range candidates {
				if c.scope == "legacy" {
					tmpl, tmplPath, err := resolveAgentTemplate(agentFlag)
					if err == nil {
						fmt.Fprintf(os.Stderr, "Warning: using legacy template at %s — run 'owl migrate templates' to convert\n", tmplPath)
						if hatchArgs.Description == "" {
							hatchArgs.Description = tmpl.Description
						} else {
							hatchArgs.Description = tmpl.SystemPrompt + "\n\nTask: " + hatchArgs.Description
						}
						if hatchArgs.ModelID == "" {
							hatchArgs.ModelID = tmpl.Model
						}
						if !cmd.Flags().Changed("thinking") {
							hatchArgs.Thinking = tmpl.Thinking
						}
						if hatchArgs.Effort == "" {
							hatchArgs.Effort = tmpl.Effort
						}
						if hatchArgs.Name == "" {
							hatchArgs.Name = tmpl.Name
						}
						resolved = true
						break
					}
				} else {
					agentsMD := filepath.Join(c.path, "AGENTS.md")
					if _, err := os.Stat(agentsMD); err == nil {
						resolved = true
						break
					}
				}
			}

			if !resolved {
				fmt.Fprintf(os.Stderr, "Error: agent definition %q not found (searched project, global, and legacy scopes)\n", agentFlag)
				os.Exit(1)
			}
		}

		if hatchArgs.Description == "" && agentFlag == "" {
			fmt.Fprintln(os.Stderr, "Error: description is required (or use --agent <name>)")
			os.Exit(1)
		}

		client, err := rpc.Dial("unix", "/tmp/owld.sock")
		if err != nil {
			fmt.Println("Error connecting to owld daemon. Run 'go run ./cmd/owld' in another terminal.")
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		// --dry-run: print resolved config without spawning
		if dryRunFlag {
			var reply ipc.DryRunReply
			if err := client.Call("Daemon.DryRunHatch", &hatchArgs, &reply); err != nil {
				fmt.Fprintf(os.Stderr, "Error: dry-run failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Dry-run result:\n")
			fmt.Printf("  Agent:      %s\n", reply.ResolvedAgent)
			fmt.Printf("  Scope:      %s\n", reply.Scope)
			fmt.Printf("  SourcePath: %s\n", reply.SourcePath)
			fmt.Printf("  Model:      %s\n", reply.ModelID)
			fmt.Printf("  Valid:      %v\n", reply.Valid)
			if len(reply.Errors) > 0 {
				fmt.Printf("  Errors:\n")
				for _, e := range reply.Errors {
					fmt.Printf("    - %s\n", e)
				}
			}
			if reply.PromptStack != "" {
				fmt.Printf("  Prompt stack:\n%s\n", reply.PromptStack)
			}
			if !reply.Valid {
				os.Exit(1)
			}
			return
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var errCount int

		for i := 0; i < countFlag; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				var reply ipc.HatchReply
				err := client.Call("Daemon.Hatch", &hatchArgs, &reply)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					fmt.Printf("RPC error for agent %d: %v\n", idx+1, err)
					errCount++
				} else {
					fmt.Printf("[%d/%d] %s\n", idx+1, countFlag, reply.Message)
				}
			}(i)
		}
		wg.Wait()

		if errCount > 0 {
			os.Exit(1)
		}
	},
}

var cloneCmd = &cobra.Command{
	Use:   "clone <agent_id>",
	Short: "Clone an existing agent",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := rpc.Dial("unix", "/tmp/owld.sock")
		if err != nil {
			fmt.Println("Error connecting to owld daemon. Run 'go run ./cmd/owld' in another terminal.")
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		agentID := strings.TrimSpace(args[0])
		agentIndex := -1
		if n, err := strconv.Atoi(agentID); err == nil {
			agentIndex = n - 1
		}

		if agentIndex < 0 {
			fmt.Println("Error: agent_id must be a number (agent index)")
			os.Exit(1)
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var errCount int

		for i := 0; i < countFlag; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				var reply ipc.CloneResponse
				err := client.Call("Daemon.CloneAgent", &ipc.CloneRequest{AgentIndex: agentIndex}, &reply)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					fmt.Printf("RPC error for clone %d: %v\n", idx+1, err)
					errCount++
				} else {
					fmt.Printf("[%d/%d] %s (new ID: %s)\n", idx+1, countFlag, reply.Message, reply.NewID)
				}
			}(i)
		}
		wg.Wait()

		if errCount > 0 {
			os.Exit(1)
		}
	},
}

func main() {
	hatchCmd.Flags().IntVarP(&countFlag, "count", "n", 1, "Number of agents to hatch")
	hatchCmd.Flags().StringVar(&modelFlag, "model", "", "Override the default model")
	hatchCmd.Flags().StringVar(&agentFlag, "agent", "", "Use a named agent definition")
	hatchCmd.Flags().StringVar(&templateFlag, "template", "", "Use a prompt template (deprecated: use --agent)")
	hatchCmd.Flags().StringVar(&fromFileFlag, "from-file", "", "Load prompt from file path")
	hatchCmd.Flags().StringVar(&scopeFlag, "scope", "", "Scope hint for agent resolution (project|global)")
	hatchCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Show resolved agent setup without executing")
	_ = hatchCmd.Flags().MarkDeprecated("template", "use --agent instead")
	hatchCmd.Flags().StringVar(&registryFlag, "registry", "", "Override the Viche registry")
	hatchCmd.Flags().BoolVar(&thinkingFlag, "thinking", false, "Enable extended thinking")
	hatchCmd.Flags().StringVar(&effortFlag, "effort", "", "Set reasoning effort (low/medium/high)")
	hatchCmd.Flags().StringVar(&nameFlag, "name", "", "Override the agent's display name")
	hatchCmd.Flags().BoolVar(&ambientFlag, "ambient", false, "Hatch into an ambient background mode (waits for messages without starting work immediately)")
	hatchCmd.Flags().StringVar(&dirFlag, "dir", "", "Set the working directory for the agent (finds .owl/config.json and sets file operation root)")
	hatchCmd.Flags().StringVar(&harnessFlag, "harness", "", "Run an external coding harness (codex|opencode|claude-code)")
	hatchCmd.Flags().StringVar(&harnessArgsFlag, "harness-args", "", "Additional args passed to the selected harness")
	hatchCmd.Flags().BoolVar(&noNetInjectFlag, "no-network-inject", false, "Disable Viche/Owl env injection into harness process")

	cloneCmd.Flags().IntVarP(&countFlag, "count", "n", 1, "Number of clones to spawn")

	rootCmd.AddCommand(hatchCmd)
	rootCmd.AddCommand(cloneCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(runsCmd)
	rootCmd.AddCommand(metricsCmd)
	rootCmd.AddCommand(recommendCmd)
	rootCmd.AddCommand(designCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
