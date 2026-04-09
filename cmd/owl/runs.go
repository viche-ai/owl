package main

import (
	"encoding/json"
	"fmt"
	"net/rpc"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/ipc"
)

var runsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Manage agent runs (list, stop, remove, inspect)",
}

// ── runs list ────────────────────────────────────────────────────────────────

var (
	runsListAll         bool
	runsListStateFilter string
	runsListJSON        bool
)

var runsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent runs",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := dialDaemon()
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		var reply ipc.ListRunsReply
		err = client.Call("Daemon.ListRuns", &ipc.ListRunsArgs{
			All:         runsListAll,
			StateFilter: runsListStateFilter,
		}, &reply)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if runsListJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(reply.Records)
			return
		}

		if len(reply.Records) == 0 {
			fmt.Println("No runs found.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "RUN_ID\tAGENT\tSTATE\tSTARTED\tDURATION\tMODEL")
		for _, r := range reply.Records {
			duration := "-"
			if r.EndTime != nil {
				duration = r.EndTime.Sub(r.StartTime).Round(time.Second).String()
			} else if !r.StartTime.IsZero() {
				duration = time.Since(r.StartTime).Round(time.Second).String()
			}

			started := "-"
			if !r.StartTime.IsZero() {
				started = r.StartTime.Format("2006-01-02 15:04:05")
			}

			modelID := r.ModelID
			if r.Harness != "" {
				modelID = "harness:" + r.Harness
			}

			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.RunID, r.AgentName, r.State, started, duration, modelID)
		}
		_ = w.Flush()
	},
}

// ── runs stop ────────────────────────────────────────────────────────────────

var runsStopForce bool

var runsStopCmd = &cobra.Command{
	Use:   "stop <run-id>",
	Short: "Stop a running agent (graceful by default)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runID := strings.TrimSpace(args[0])

		if runsStopForce {
			fmt.Fprintf(os.Stderr, "Warning: force-stopping %s. This immediately terminates the agent.\n", runID)
		}

		client, err := dialDaemon()
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		var reply ipc.StopReply
		err = client.Call("Daemon.StopAgent", &ipc.StopArgs{
			RunID: runID,
			Force: runsStopForce,
		}, &reply)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(reply.Message)
	},
}

// ── runs remove ──────────────────────────────────────────────────────────────

var (
	runsRemoveNoArchive bool
	runsRemoveForce     bool
)

var runsRemoveCmd = &cobra.Command{
	Use:   "remove <run-id>",
	Short: "Remove an agent run (archived by default; --no-archive --force for hard delete)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runID := strings.TrimSpace(args[0])

		archive := !runsRemoveNoArchive
		if !archive && !runsRemoveForce {
			fmt.Fprintln(os.Stderr, "Error: hard delete requires --no-archive and --force")
			os.Exit(1)
		}

		if !archive && runsRemoveForce {
			fmt.Fprintf(os.Stderr, "Warning: permanently deleting run %s and its record.\n", runID)
		}

		client, err := dialDaemon()
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		var reply ipc.RemoveReply
		err = client.Call("Daemon.RemoveAgent", &ipc.RemoveArgs{
			RunID:   runID,
			Archive: archive,
			Force:   runsRemoveForce,
		}, &reply)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(reply.Message)
	},
}

// ── runs inspect ─────────────────────────────────────────────────────────────

var (
	runsInspectJSON     bool
	runsInspectFullLogs bool
)

var runsInspectCmd = &cobra.Command{
	Use:   "inspect <run-id>",
	Short: "Show detailed info and recent logs for a run",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runID := strings.TrimSpace(args[0])

		client, err := dialDaemon()
		if err != nil {
			os.Exit(1)
		}
		defer func() { _ = client.Close() }()

		var reply ipc.InspectReply
		err = client.Call("Daemon.InspectAgent", &ipc.InspectArgs{
			RunID:    runID,
			FullLogs: runsInspectFullLogs,
		}, &reply)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if runsInspectJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(reply)
			return
		}

		r := reply.RunRecord
		fmt.Printf("Run ID:     %s\n", r.RunID)
		fmt.Printf("Agent:      %s\n", r.AgentName)
		if r.AgentDef != "" {
			fmt.Printf("Definition: %s\n", r.AgentDef)
		}
		fmt.Printf("State:      %s\n", r.State)
		fmt.Printf("Model:      %s\n", r.ModelID)
		if r.Harness != "" {
			fmt.Printf("Harness:    %s\n", r.Harness)
		}
		if !r.StartTime.IsZero() {
			fmt.Printf("Started:    %s\n", r.StartTime.Format(time.RFC3339))
		}
		if r.EndTime != nil {
			fmt.Printf("Ended:      %s\n", r.EndTime.Format(time.RFC3339))
			fmt.Printf("Duration:   %s\n", r.EndTime.Sub(r.StartTime).Round(time.Second))
		} else if !r.StartTime.IsZero() {
			fmt.Printf("Running:    %s\n", time.Since(r.StartTime).Round(time.Second))
		}
		if r.ExitReason != "" {
			fmt.Printf("Exit:       %s\n", r.ExitReason)
		}
		if r.LogPath != "" {
			fmt.Printf("Log:        %s\n", r.LogPath)
		}
		if r.WorkDir != "" {
			fmt.Printf("WorkDir:    %s\n", r.WorkDir)
		}

		if reply.AgentState != nil {
			ag := reply.AgentState
			fmt.Printf("\n[Active] Context: %s  VicheID: %s\n", ag.Ctx, ag.VicheID)
		}

		if reply.RecentLogs != "" {
			fmt.Printf("\n--- Recent logs ---\n%s", reply.RecentLogs)
		}
	},
}

// ── helpers ───────────────────────────────────────────────────────────────────

func dialDaemon() (*rpc.Client, error) {
	client, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: cannot connect to owld daemon. Run 'owld' first.")
	}
	return client, err
}

func init() {
	runsListCmd.Flags().BoolVar(&runsListAll, "all", false, "Include archived runs")
	runsListCmd.Flags().StringVar(&runsListStateFilter, "state", "", "Filter by state (e.g. stopped, archived)")
	runsListCmd.Flags().BoolVar(&runsListJSON, "json", false, "Output as JSON")

	runsStopCmd.Flags().BoolVar(&runsStopForce, "force", false, "Force-stop (immediate kill)")

	runsRemoveCmd.Flags().BoolVar(&runsRemoveNoArchive, "no-archive", false, "Hard delete instead of archiving (requires --force)")
	runsRemoveCmd.Flags().BoolVar(&runsRemoveForce, "force", false, "Confirm hard delete")

	runsInspectCmd.Flags().BoolVar(&runsInspectJSON, "json", false, "Output as JSON")
	runsInspectCmd.Flags().BoolVar(&runsInspectFullLogs, "logs", false, "Show full log history (last 100 entries)")

	runsCmd.AddCommand(runsListCmd)
	runsCmd.AddCommand(runsStopCmd)
	runsCmd.AddCommand(runsRemoveCmd)
	runsCmd.AddCommand(runsInspectCmd)
}
