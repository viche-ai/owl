package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/ipc"
)

var designCmd = &cobra.Command{
	Use:   "design [description...]",
	Short: "Start an interactive agent design interview with the meta-agent",
	Long: `Launches a guided interview in the meta-agent to help you design a
thorough agent definition. The meta-agent will ask about the agent's
purpose, process, coordination needs, and constraints, then generate
AGENTS.md, agent.yaml, and optional supplementary files.

Optionally provide an initial description to skip early questions:
  owl design "a code reviewer that checks PRs against our style guide"

The conversation continues in the TUI console (owl tab).`,
	Run: func(cmd *cobra.Command, args []string) {
		client, err := dialDaemon()
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		desc := strings.Join(args, " ")

		var prompt string
		if desc != "" {
			prompt = fmt.Sprintf(
				"I want to design a new agent. Here is what I have in mind: %s\n\n"+
					"Start the agent design interview. Begin by gathering context "+
					"(read_project_config, list_agents, list_models), then ask me "+
					"clarifying questions one at a time to fill in any gaps before "+
					"generating a draft.",
				desc,
			)
		} else {
			prompt = "I want to design a new agent.\n\n" +
				"Start the agent design interview. Begin by gathering context " +
				"(read_project_config, list_agents, list_models), then interview me " +
				"to understand what I need."
		}

		// Find the meta-agent by name "owl"
		var listReply ipc.ListReply
		if err := client.Call("Daemon.ListAgents", &ipc.ListArgs{}, &listReply); err != nil {
			fmt.Fprintf(os.Stderr, "Error listing agents: %v\n", err)
			os.Exit(1)
		}

		metaIdx := -1
		for i, ag := range listReply.Agents {
			if ag.Name == "owl" {
				metaIdx = i
				break
			}
		}
		if metaIdx < 0 {
			fmt.Fprintln(os.Stderr, "Error: meta-agent (owl) is not running. Start owld first.")
			os.Exit(1)
		}

		var sendReply ipc.SendMessageReply
		if err := client.Call("Daemon.SendMessage", &ipc.SendMessageArgs{
			AgentIndex: metaIdx,
			Content:    prompt,
		}, &sendReply); err != nil {
			fmt.Fprintf(os.Stderr, "Error sending message to meta-agent: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Agent design interview started.")
		fmt.Println("Switch to the TUI console (owl tab) to continue the conversation.")
	},
}
