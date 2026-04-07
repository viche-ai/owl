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

var rootCmd = &cobra.Command{
	Use:   "owl",
	Short: "Owl terminal coding tool",
	Run: func(cmd *cobra.Command, args []string) {
		tui.Run()
	},
}

var (
	modelFlag       string
	templateFlag    string
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
		client, err := rpc.Dial("unix", "/tmp/owld.sock")
		if err != nil {
			fmt.Println("Error connecting to owld daemon. Run 'go run ./cmd/owld' in another terminal.")
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		desc := strings.Join(args, " ")

		hatchArgs := ipc.HatchArgs{
			Description: desc,
			ModelID:     modelFlag,
			Template:    templateFlag,
			Registry:    registryFlag,
			Thinking:    thinkingFlag,
			Effort:      effortFlag,
			Name:        nameFlag,
			Ambient:     ambientFlag,
			WorkDir:     dirFlag,
			Harness:     harnessFlag,
			HarnessArgs: harnessArgsFlag,
			NoNetInject: noNetInjectFlag,
		}

		if templateFlag != "" {
			home, _ := os.UserHomeDir()
			tmplPath := filepath.Join(home, ".owl", "templates", templateFlag+".json")
			if b, err := os.ReadFile(tmplPath); err == nil {
				var tmpl Template
				if err := json.Unmarshal(b, &tmpl); err == nil {
					// Merge template defaults if not explicitly provided
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
				} else {
					fmt.Println("Warning: Failed to parse template JSON:", err)
				}
			} else {
				fmt.Println("Warning: Template not found at", tmplPath)
			}
		}

		if hatchArgs.Description == "" {
			fmt.Println("Error: description is required")
			os.Exit(1)
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
	hatchCmd.Flags().StringVar(&templateFlag, "template", "", "Use a prompt template")
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
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
