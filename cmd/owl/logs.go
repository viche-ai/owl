package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/logs"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View and query structured agent logs",
}

var logsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available log runs",
	RunE:  runLogsList,
}

var logsShowCmd = &cobra.Command{
	Use:   "show <run-id>",
	Short: "Show log entries for a run",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogsShow,
}

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream recent log entries (latest run or by agent name)",
	RunE:  runLogsTail,
}

var logsQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query logs with structured filters",
	RunE:  runLogsQuery,
}

var (
	logsAgentFlag  string
	logsJSONFlag   bool
	logsLevelFlag  string
	logsFollowFlag bool
	logsSinceFlag  string
	logsUntilFlag  string
	logsLimitFlag  int
)

func init() {
	logsListCmd.Flags().StringVar(&logsAgentFlag, "agent", "", "Filter by agent name")
	logsListCmd.Flags().BoolVar(&logsJSONFlag, "json", false, "Output as JSON lines")

	logsShowCmd.Flags().BoolVar(&logsJSONFlag, "json", false, "Output raw JSON lines")
	logsShowCmd.Flags().StringVar(&logsLevelFlag, "level", "", "Filter by level (info, warn, error, debug, tool, thinking)")

	logsTailCmd.Flags().StringVar(&logsAgentFlag, "agent", "", "Agent name to tail (defaults to most recent run)")
	logsTailCmd.Flags().BoolVarP(&logsFollowFlag, "follow", "f", false, "Follow log output continuously")
	logsTailCmd.Flags().BoolVar(&logsJSONFlag, "json", false, "Output raw JSON lines")

	logsQueryCmd.Flags().StringVar(&logsAgentFlag, "agent", "", "Filter by agent name")
	logsQueryCmd.Flags().StringVar(&logsSinceFlag, "since", "", "Return entries after this time (RFC3339 or duration like 1h, 7d)")
	logsQueryCmd.Flags().StringVar(&logsUntilFlag, "until", "", "Return entries before this time (RFC3339)")
	logsQueryCmd.Flags().StringVar(&logsLevelFlag, "level", "", "Filter by level (info, warn, error, debug, tool, thinking)")
	logsQueryCmd.Flags().IntVar(&logsLimitFlag, "limit", 0, "Maximum number of entries to return")
	logsQueryCmd.Flags().BoolVar(&logsJSONFlag, "json", false, "Output as JSON lines")

	logsCmd.AddCommand(logsListCmd, logsShowCmd, logsTailCmd, logsQueryCmd)
	rootCmd.AddCommand(logsCmd)
}

func newLogReader() *logs.Reader {
	home, _ := os.UserHomeDir()
	return &logs.Reader{LogDir: home + "/.owl/logs"}
}

func runLogsList(cmd *cobra.Command, args []string) error {
	r := newLogReader()
	metas, err := r.List()
	if err != nil {
		return fmt.Errorf("listing logs: %w", err)
	}
	if len(metas) == 0 {
		fmt.Println("No log runs found.")
		return nil
	}
	if logsJSONFlag {
		for _, m := range metas {
			if logsAgentFlag != "" && m.AgentName != logsAgentFlag {
				continue
			}
			b, _ := json.Marshal(m)
			fmt.Println(string(b))
		}
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "RUN_ID\tAGENT\tSTARTED\tSIZE")
	for _, m := range metas {
		if logsAgentFlag != "" && m.AgentName != logsAgentFlag {
			continue
		}
		started := m.StartTime.Format("2006-01-02 15:04:05")
		size := formatLogSize(m.Size)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.RunID, m.AgentName, started, size)
	}
	return w.Flush()
}

func runLogsShow(cmd *cobra.Command, args []string) error {
	r := newLogReader()
	entries, err := r.Read(args[0])
	if err != nil {
		return fmt.Errorf("reading log %s: %w", args[0], err)
	}
	for _, entry := range entries {
		if logsLevelFlag != "" && entry.Level != logsLevelFlag {
			continue
		}
		if logsJSONFlag {
			b, _ := json.Marshal(entry)
			fmt.Println(string(b))
			continue
		}
		fmt.Print(formatLogEntry(entry))
	}
	return nil
}

func runLogsTail(cmd *cobra.Command, args []string) error {
	r := newLogReader()
	ch, err := r.Tail(logsAgentFlag, logsFollowFlag)
	if err != nil {
		return err
	}
	for entry := range ch {
		if logsJSONFlag {
			b, _ := json.Marshal(entry)
			fmt.Println(string(b))
		} else {
			fmt.Print(formatLogEntry(entry))
		}
	}
	return nil
}

func runLogsQuery(cmd *cobra.Command, args []string) error {
	r := newLogReader()
	opts := logs.QueryOpts{
		AgentName: logsAgentFlag,
		Level:     logsLevelFlag,
		Limit:     logsLimitFlag,
	}
	if logsSinceFlag != "" {
		t, err := parseTimeOrDuration(logsSinceFlag)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		opts.Since = t
	}
	if logsUntilFlag != "" {
		t, err := time.Parse(time.RFC3339, logsUntilFlag)
		if err != nil {
			return fmt.Errorf("invalid --until (must be RFC3339): %w", err)
		}
		opts.Until = t
	}
	entries, err := r.Query(opts)
	if err != nil {
		return fmt.Errorf("querying logs: %w", err)
	}
	for _, entry := range entries {
		if logsJSONFlag {
			b, _ := json.Marshal(entry)
			fmt.Println(string(b))
		} else {
			fmt.Print(formatLogEntry(entry))
		}
	}
	return nil
}

func formatLogEntry(e logs.LogEntry) string {
	ts := e.Timestamp.Format("15:04:05")
	levelStr := formatLogLevel(e.Level)
	line := fmt.Sprintf("%s %s %s", ts, levelStr, strings.TrimRight(e.Message, "\n"))
	if e.ToolName != "" {
		line += fmt.Sprintf(" [tool:%s]", e.ToolName)
	}
	return line + "\n"
}

func formatLogLevel(level string) string {
	switch level {
	case "error":
		return "\033[31mERROR\033[0m"
	case "warn":
		return "\033[33mWARN \033[0m"
	case "tool":
		return "\033[36mTOOL \033[0m"
	case "thinking":
		return "\033[35mTHINK\033[0m"
	case "debug":
		return "\033[90mDEBUG\033[0m"
	default:
		return "\033[32mINFO \033[0m"
	}
}

func formatLogSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}

// parseTimeOrDuration parses a time string as RFC3339 or a duration relative to now.
// Supports Go durations (e.g. "1h", "30m") and day suffixes (e.g. "7d").
func parseTimeOrDuration(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid day duration %q", s)
		}
		return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time or duration %q (use RFC3339 or Go duration like 1h, 30m, 7d)", s)
	}
	return time.Now().Add(-d), nil
}
