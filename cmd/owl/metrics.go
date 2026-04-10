package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/metrics"
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "View agent run metrics",
}

// ── metrics show ─────────────────────────────────────────────────────────────

var (
	metricsShowJSON  bool
	metricsShowSince string
)

var metricsShowCmd = &cobra.Command{
	Use:   "show <run-id|agent-name>",
	Short: "Show metrics for a run ID or aggregate metrics for an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]

		store, err := metrics.NewStore()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening metrics store: %v\n", err)
			os.Exit(1)
		}

		// Try to load as a single run first
		if m, err := store.Load(target); err == nil {
			if metricsShowJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(m)
				return
			}
			printRunMetrics(m)
			return
		}

		// Fall back to aggregate view for an agent name
		var since time.Time
		if metricsShowSince != "" {
			d, err := time.ParseDuration(metricsShowSince)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid --since duration %q: %v\n", metricsShowSince, err)
				os.Exit(1)
			}
			since = time.Now().Add(-d)
		}

		records, err := store.List(metrics.ListOpts{AgentName: target, Since: since})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing metrics: %v\n", err)
			os.Exit(1)
		}

		if len(records) == 0 {
			fmt.Printf("No metrics found for %q.\n", target)
			return
		}

		summary, err := store.Aggregate(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error aggregating metrics: %v\n", err)
			os.Exit(1)
		}

		if metricsShowJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(summary)
			return
		}

		printAgentSummary(summary, records)
	},
}

func printRunMetrics(m *metrics.RunMetrics) {
	dur := "—"
	if m.DurationMS > 0 {
		dur = fmt.Sprintf("%.1fs", float64(m.DurationMS)/1000)
	}
	activeDur := "—"
	if m.ActiveDurationMS > 0 {
		activeDur = fmt.Sprintf("%.1fs", float64(m.ActiveDurationMS)/1000)
	}
	cost := "—"
	if m.EstimatedCost > 0 {
		cost = fmt.Sprintf("~$%.4f", m.EstimatedCost)
	}

	fmt.Println("Run Metrics")
	fmt.Println("───────────────────────────────────────")
	fmt.Printf("  Run ID:      %s\n", m.RunID)
	fmt.Printf("  Agent:       %s\n", m.AgentName)
	if m.AgentVersion != "" {
		fmt.Printf("  Version:     %s\n", m.AgentVersion)
	}
	fmt.Printf("  Model:       %s\n", m.Model)
	fmt.Printf("  Status:      %s\n", m.Status)
	fmt.Printf("  Started:     %s\n", m.StartTS.Format("2006-01-02 15:04:05"))
	if m.EndTS != nil {
		fmt.Printf("  Ended:       %s\n", m.EndTS.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("  Duration:    %s (active: %s)\n", dur, activeDur)
	fmt.Printf("  Cost (est.): %s\n", cost)
	fmt.Println()
	fmt.Println("  Token Usage")
	fmt.Printf("    Input:     %d\n", m.TokenInput)
	fmt.Printf("    Output:    %d\n", m.TokenOutput)
	fmt.Println()
	fmt.Println("  Tool Usage")
	fmt.Printf("    Calls:     %d\n", m.ToolCallCount)
	fmt.Printf("    Failures:  %d\n", m.ToolFailCount)
	fmt.Println()
	fmt.Println("  Task Ledger")
	fmt.Printf("    Created:   %d\n", m.TasksCreated)
	fmt.Printf("    Completed: %d\n", m.TasksCompleted)
	if m.HandoffCount > 0 {
		fmt.Printf("  Handoffs:    %d\n", m.HandoffCount)
	}
	if m.PromptHash != "" {
		short := m.PromptHash
		if len(short) > 8 {
			short = short[:8]
		}
		fmt.Printf("  Prompt hash: %s\n", short)
	}
}

func printAgentSummary(summary *metrics.AgentMetricsSummary, records []metrics.RunMetrics) {
	fmt.Printf("Agent Metrics: %s\n", summary.AgentName)
	fmt.Println("───────────────────────────────────────")
	fmt.Printf("  Total runs:    %d\n", summary.TotalRuns)
	fmt.Printf("  Success rate:  %.0f%%\n", summary.SuccessRate*100)
	if summary.AvgActiveDurationMS > 0 {
		fmt.Printf("  Avg duration:  %.1fs (active: %.1fs)\n", float64(summary.AvgDurationMS)/1000, float64(summary.AvgActiveDurationMS)/1000)
	} else {
		fmt.Printf("  Avg duration:  %.1fs\n", float64(summary.AvgDurationMS)/1000)
	}
	fmt.Printf("  Avg tokens in: %d\n", summary.AvgTokensIn)
	fmt.Printf("  Avg tokens out:%d\n", summary.AvgTokensOut)
	fmt.Printf("  Total cost:    ~$%.4f\n", summary.TotalCost)
	fmt.Println()

	if len(summary.PromptVersions) > 0 {
		fmt.Println("  Prompt versions:")
		for _, pv := range summary.PromptVersions {
			hash := pv.PromptHash
			if len(hash) > 8 {
				hash = hash[:8]
			}
			fmt.Printf("    %s  runs=%-4d  success=%.0f%%\n", hash, pv.RunCount, pv.SuccessRate*100)
		}
		fmt.Println()
	}

	fmt.Println("  Recent runs:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "    STARTED\tRUN ID\tSTATUS\tDURATION\tTOKENS\tCOST")
	limit := 10
	if len(records) < limit {
		limit = len(records)
	}
	for _, m := range records[:limit] {
		dur := "—"
		if m.DurationMS > 0 {
			dur = fmt.Sprintf("%.1fs", float64(m.DurationMS)/1000)
		}
		cost := "—"
		if m.EstimatedCost > 0 {
			cost = fmt.Sprintf("~$%.4f", m.EstimatedCost)
		}
		_, _ = fmt.Fprintf(tw, "    %s\t%s\t%s\t%s\t%d/%d\t%s\n",
			m.StartTS.Format("01-02 15:04"),
			m.RunID,
			m.Status,
			dur,
			m.TokenInput, m.TokenOutput,
			cost,
		)
	}
	_ = tw.Flush()
}

// ── owl recommend ────────────────────────────────────────────────────────────

var recommendAgent string

var recommendCmd = &cobra.Command{
	Use:   "recommend",
	Short: "Ask the meta-agent to analyze metrics and suggest prompt improvements",
	Run: func(cmd *cobra.Command, args []string) {
		if recommendAgent == "" {
			fmt.Fprintln(os.Stderr, "Error: --agent is required")
			os.Exit(1)
		}

		client, err := dialDaemon()
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		prompt := fmt.Sprintf(
			"Analyze agent %q and suggest improvements.\n"+
				"Steps:\n"+
				"1. Call query_metrics with agent_name=%q to review recent run data.\n"+
				"   Note: metrics include both wall-clock duration and active duration.\n"+
				"   For ambient agents (long-running, waiting for messages), focus on active duration — wall-clock time includes idle wait and is not meaningful for performance analysis.\n"+
				"2. Call compare_versions with agent_name=%q to compare prompt versions.\n"+
				"3. Call query_logs with agent_name=%q and level=error to identify recurring failures.\n"+
				"4. Call read_agent_file with name=%q and file=AGENTS.md to understand the agent's task and role.\n"+
				"5. Call list_models to see configured providers.\n"+
				"6. Based on the agent's task complexity (from AGENTS.md), token usage patterns, and available models, recommend whether the current model is appropriate or if a different model would be better suited. Consider cost vs capability tradeoffs.\n"+
				"7. Summarize findings: top failure modes, performance insights, model recommendation, and proposed prompt edits.\n"+
				"Do not apply any changes — only suggest them with suggest_edit.",
			recommendAgent, recommendAgent, recommendAgent, recommendAgent, recommendAgent,
		)

		// Find the meta-agent (always index 0) and send the message
		var listReply ipc.ListReply
		if err := client.Call("Daemon.ListAgents", &ipc.ListArgs{}, &listReply); err != nil {
			fmt.Fprintf(os.Stderr, "Error listing agents: %v\n", err)
			os.Exit(1)
		}

		// Find the meta-agent by name "owl"
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

		fmt.Printf("Analysis request sent to meta-agent for %q.\n", recommendAgent)
		fmt.Println("Check the TUI console (owl tab) for recommendations.")
	},
}

func init() {
	metricsShowCmd.Flags().BoolVar(&metricsShowJSON, "json", false, "Output as JSON")
	metricsShowCmd.Flags().StringVar(&metricsShowSince, "since", "", "Filter runs since duration (e.g. 1h, 24h)")

	metricsCmd.AddCommand(metricsShowCmd)

	recommendCmd.Flags().StringVar(&recommendAgent, "agent", "", "Agent name to analyze")
}
