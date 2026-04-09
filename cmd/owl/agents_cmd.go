package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/viche-ai/owl/internal/agents"
	"github.com/viche-ai/owl/internal/config"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agent definitions (list, show, validate, import, export, promote, demote, diff)",
}

// ── agents list ───────────────────────────────────────────────────────────────

var (
	agentsListScope string
	agentsListJSON  bool
)

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agent definitions",
	Run: func(cmd *cobra.Command, args []string) {
		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		// Map "all" → "" for Resolver.List
		scopeFilter := agentsListScope
		if scopeFilter == "all" {
			scopeFilter = ""
		}

		defs, err := resolver.List(scopeFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if agentsListJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(defs)
			return
		}

		if len(defs) == 0 {
			fmt.Println("No agents found.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "NAME\tSCOPE\tVERSION\tMODEL\tDESCRIPTION")
		for _, d := range defs {
			version := d.Version
			if version == "" {
				version = "-"
			}
			model := d.DefaultModel
			if model == "" {
				model = "-"
			}
			desc := d.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			if desc == "" {
				desc = "-"
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", d.Name, d.Scope, version, model, desc)
		}
		_ = w.Flush()
	},
}

// ── agents show ───────────────────────────────────────────────────────────────

var (
	agentsShowScope string
	agentsShowJSON  bool
)

var agentsShowCmd = &cobra.Command{
	Use:   "show <agent>",
	Short: "Show full agent definition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(name, agentsShowScope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if agentsShowJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(def)
			return
		}

		fmt.Printf("Name:        %s\n", def.Name)
		fmt.Printf("Scope:       %s\n", def.Scope)
		fmt.Printf("Source:      %s\n", def.SourcePath)
		if def.Version != "" {
			fmt.Printf("Version:     %s\n", def.Version)
		}
		if def.Description != "" {
			fmt.Printf("Description: %s\n", def.Description)
		}
		if def.DefaultModel != "" {
			fmt.Printf("Model:       %s\n", def.DefaultModel)
		}
		if def.Owner != "" {
			fmt.Printf("Owner:       %s\n", def.Owner)
		}
		if len(def.Capabilities) > 0 {
			fmt.Printf("Capabilities: %s\n", strings.Join(def.Capabilities, ", "))
		}
		if len(def.Tags) > 0 {
			fmt.Printf("Tags:        %s\n", strings.Join(def.Tags, ", "))
		}
		if def.AgentsMD != "" {
			fmt.Printf("\n--- AGENTS.md ---\n%s\n", def.AgentsMD)
		}
		if def.RoleMD != "" {
			fmt.Printf("\n--- role.md ---\n%s\n", def.RoleMD)
		}
		if def.GuardrailsMD != "" {
			fmt.Printf("\n--- guardrails.md ---\n%s\n", def.GuardrailsMD)
		}
	},
}

// ── agents validate ───────────────────────────────────────────────────────────

var (
	agentsValidateScope  string
	agentsValidateStrict bool
)

var agentsValidateCmd = &cobra.Command{
	Use:   "validate <agent>",
	Short: "Validate an agent definition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(name, agentsValidateScope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		errs := resolver.Validate(def, agentsValidateStrict)
		if len(errs) == 0 {
			if agentsValidateStrict {
				fmt.Printf("OK: agent %q is valid (strict)\n", name)
			} else {
				fmt.Printf("OK: agent %q is valid\n", name)
			}
			return
		}

		fmt.Fprintf(os.Stderr, "Validation failed for agent %q:\n", name)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e.Error())
		}
		os.Exit(1)
	},
}

// ── agents import ─────────────────────────────────────────────────────────────

var (
	agentsImportScope string
	agentsImportName  string
)

var agentsImportCmd = &cobra.Command{
	Use:   "import --path <dir|file> [--scope project|global]",
	Short: "Import an agent definition from a path",
	Run: func(cmd *cobra.Command, args []string) {
		importPath, _ := cmd.Flags().GetString("path")
		if importPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --path is required")
			os.Exit(1)
		}

		if err := agents.ValidateImportPath(importPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		var destDir string
		switch agentsImportScope {
		case "global":
			destDir = resolver.GlobalDir
		case "project", "":
			if resolver.ProjectDir == "" {
				fmt.Fprintln(os.Stderr, "Error: no project found (run 'owl project init' first) or use --scope global")
				os.Exit(1)
			}
			destDir = resolver.ProjectDir
		default:
			fmt.Fprintln(os.Stderr, `Error: --scope must be "project" or "global"`)
			os.Exit(1)
		}

		def, suggestions, err := agents.ImportAgent(importPath, destDir, agentsImportName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		scope := agentsImportScope
		if scope == "" {
			scope = "project"
		}
		fmt.Printf("Imported agent %q into %s scope at %s\n", def.Name, scope, def.SourcePath)
		for _, s := range suggestions {
			fmt.Println("  Suggestion:", s)
		}
	},
}

// ── agents export ─────────────────────────────────────────────────────────────

var (
	agentsExportAgent            string
	agentsExportOut              string
	agentsExportScope            string
	agentsExportIncludeGenerated bool
)

var agentsExportCmd = &cobra.Command{
	Use:   "export --agent <name> --out <path>",
	Short: "Export an agent definition to a directory",
	Run: func(cmd *cobra.Command, args []string) {
		if agentsExportAgent == "" {
			fmt.Fprintln(os.Stderr, "Error: --agent is required")
			os.Exit(1)
		}
		if agentsExportOut == "" {
			fmt.Fprintln(os.Stderr, "Error: --out is required")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(agentsExportAgent, agentsExportScope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := agents.ExportAgent(def, agentsExportOut, agentsExportIncludeGenerated); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Exported agent %q to %s\n", def.Name, agentsExportOut)
	},
}

// ── agents promote ────────────────────────────────────────────────────────────

var (
	agentsPromoteAgent string
	agentsPromoteTo    string
	agentsPromoteForce bool
)

var agentsPromoteCmd = &cobra.Command{
	Use:   "promote --agent <name>",
	Short: "Promote a project-scoped agent to global scope",
	Run: func(cmd *cobra.Command, args []string) {
		if agentsPromoteAgent == "" {
			fmt.Fprintln(os.Stderr, "Error: --agent is required")
			os.Exit(1)
		}
		if agentsPromoteTo != "" && agentsPromoteTo != "global" {
			fmt.Fprintln(os.Stderr, `Error: --to must be "global"`)
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(agentsPromoteAgent, "project")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := agents.PromoteAgent(def, resolver.GlobalDir, agentsPromoteForce); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Promoted %s from project to global scope\n", def.Name)
	},
}

// ── agents demote ─────────────────────────────────────────────────────────────

var (
	agentsDemoteAgent string
	agentsDemoteTo    string
	agentsDemoteForce bool
)

var agentsDemoteCmd = &cobra.Command{
	Use:   "demote --agent <name>",
	Short: "Demote a global-scoped agent to project scope",
	Run: func(cmd *cobra.Command, args []string) {
		if agentsDemoteAgent == "" {
			fmt.Fprintln(os.Stderr, "Error: --agent is required")
			os.Exit(1)
		}
		if agentsDemoteTo != "" && agentsDemoteTo != "project" {
			fmt.Fprintln(os.Stderr, `Error: --to must be "project"`)
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		if resolver.ProjectDir == "" {
			fmt.Fprintln(os.Stderr, "Error: no project found (run 'owl project init' first)")
			os.Exit(1)
		}

		def, err := resolver.Resolve(agentsDemoteAgent, "global")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := agents.DemoteAgent(def, resolver.ProjectDir, agentsDemoteForce); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Demoted %s from global to project scope\n", def.Name)
	},
}

// ── agents diff ───────────────────────────────────────────────────────────────

var (
	agentsDiffFrom  string
	agentsDiffTo    string
	agentsDiffJSON  bool
	agentsDiffScope string
)

// DiffLine is one line in a structured diff result.
type DiffLine struct {
	Type    string `json:"type"` // "context", "added", "removed"
	Content string `json:"content"`
}

var agentsDiffCmd = &cobra.Command{
	Use:   "diff <agent>",
	Short: "Show diff between two versions of an agent's AGENTS.md",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(name, agentsDiffScope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		agentsMDPath := filepath.Join(def.SourcePath, "AGENTS.md")
		gitRoot, err := getGitRoot(def.SourcePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: git not available or agent is not in a git repository")
			os.Exit(1)
		}

		relPath, err := filepath.Rel(gitRoot, agentsMDPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot compute relative path: %v\n", err)
			os.Exit(1)
		}

		if agentsDiffFrom == "" && agentsDiffTo == "" {
			listGitVersions(gitRoot, relPath)
			return
		}

		if agentsDiffFrom == "" || agentsDiffTo == "" {
			fmt.Fprintln(os.Stderr, "Error: both --from and --to are required")
			os.Exit(1)
		}

		fromContent, err := getGitFileContent(gitRoot, agentsDiffFrom, relPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot get version %q: %v\n", agentsDiffFrom, err)
			os.Exit(1)
		}

		toContent, err := getGitFileContent(gitRoot, agentsDiffTo, relPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot get version %q: %v\n", agentsDiffTo, err)
			os.Exit(1)
		}

		lines := computeDiff(fromContent, toContent)

		if agentsDiffJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(lines)
			return
		}

		for _, line := range lines {
			switch line.Type {
			case "added":
				fmt.Printf("\033[32m+%s\033[0m\n", line.Content)
			case "removed":
				fmt.Printf("\033[31m-%s\033[0m\n", line.Content)
			default:
				fmt.Printf(" %s\n", line.Content)
			}
		}
	},
}

// ── agents explain (alias) ────────────────────────────────────────────────────

var agentsExplainScope string

var agentsExplainCmd = &cobra.Command{
	Use:   "explain <agent>",
	Short: "Show the resolved prompt stack for an agent definition",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		if agentsExplainScope != "" && agentsExplainScope != "project" && agentsExplainScope != "global" {
			fmt.Fprintln(os.Stderr, `Error: --scope must be "project" or "global"`)
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		resolver := agents.NewResolver(cwd)

		def, err := resolver.Resolve(name, agentsExplainScope)
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

// ── diff helpers ──────────────────────────────────────────────────────────────

func getGitRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getGitFileContent(gitRoot, ref, relPath string) (string, error) {
	out, err := exec.Command("git", "-C", gitRoot, "show", ref+":"+relPath).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func listGitVersions(gitRoot, relPath string) {
	out, err := exec.Command("git", "-C", gitRoot, "log", "--oneline", "--", relPath).Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: cannot retrieve git history")
		os.Exit(1)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		fmt.Println("No git history found for this agent's AGENTS.md")
		return
	}
	fmt.Println("Available versions (use commit hash with --from/--to):")
	fmt.Println(trimmed)
}

// computeDiff produces a line-by-line diff using LCS.
func computeDiff(from, to string) []DiffLine {
	fromLines := splitLines(from)
	toLines := splitLines(to)
	lcs := lcsLines(fromLines, toLines)

	var result []DiffLine
	fi, ti := 0, 0
	for _, common := range lcs {
		for fi < len(fromLines) && fromLines[fi] != common {
			result = append(result, DiffLine{Type: "removed", Content: fromLines[fi]})
			fi++
		}
		for ti < len(toLines) && toLines[ti] != common {
			result = append(result, DiffLine{Type: "added", Content: toLines[ti]})
			ti++
		}
		result = append(result, DiffLine{Type: "context", Content: common})
		fi++
		ti++
	}
	for ; fi < len(fromLines); fi++ {
		result = append(result, DiffLine{Type: "removed", Content: fromLines[fi]})
	}
	for ; ti < len(toLines); ti++ {
		result = append(result, DiffLine{Type: "added", Content: toLines[ti]})
	}
	return result
}

func splitLines(s string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// lcsLines computes the Longest Common Subsequence of two string slices.
func lcsLines(a, b []string) []string {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	result := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, a[i-1])
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

// ── init ──────────────────────────────────────────────────────────────────────

func init() {
	agentsListCmd.Flags().StringVar(&agentsListScope, "scope", "", "Filter by scope (project|global|all)")
	agentsListCmd.Flags().BoolVar(&agentsListJSON, "json", false, "Output as JSON")

	agentsShowCmd.Flags().StringVar(&agentsShowScope, "scope", "", "Restrict to scope (project|global)")
	agentsShowCmd.Flags().BoolVar(&agentsShowJSON, "json", false, "Output as JSON")

	agentsValidateCmd.Flags().StringVar(&agentsValidateScope, "scope", "", "Restrict to scope (project|global)")
	agentsValidateCmd.Flags().BoolVar(&agentsValidateStrict, "strict", false, "Require agent.yaml with all fields populated")

	agentsImportCmd.Flags().String("path", "", "Path to agent directory or AGENTS.md file (required)")
	agentsImportCmd.Flags().StringVar(&agentsImportScope, "scope", "project", "Target scope (project|global)")
	agentsImportCmd.Flags().StringVar(&agentsImportName, "name", "", "Override the agent name")

	agentsExportCmd.Flags().StringVar(&agentsExportAgent, "agent", "", "Agent name (required)")
	agentsExportCmd.Flags().StringVar(&agentsExportOut, "out", "", "Output directory (required)")
	agentsExportCmd.Flags().StringVar(&agentsExportScope, "scope", "", "Restrict resolution to scope (project|global)")
	agentsExportCmd.Flags().BoolVar(&agentsExportIncludeGenerated, "include-generated", false, "Include generated files (metrics.md, CHANGELOG.md)")

	agentsPromoteCmd.Flags().StringVar(&agentsPromoteAgent, "agent", "", "Agent name (required)")
	agentsPromoteCmd.Flags().StringVar(&agentsPromoteTo, "to", "global", "Target scope (must be global)")
	agentsPromoteCmd.Flags().BoolVar(&agentsPromoteForce, "force", false, "Overwrite if agent already exists in global scope")

	agentsDemoteCmd.Flags().StringVar(&agentsDemoteAgent, "agent", "", "Agent name (required)")
	agentsDemoteCmd.Flags().StringVar(&agentsDemoteTo, "to", "project", "Target scope (must be project)")
	agentsDemoteCmd.Flags().BoolVar(&agentsDemoteForce, "force", false, "Overwrite if agent already exists in project scope")

	agentsDiffCmd.Flags().StringVar(&agentsDiffFrom, "from", "", "Starting version (git ref or commit hash)")
	agentsDiffCmd.Flags().StringVar(&agentsDiffTo, "to", "", "Ending version (git ref or commit hash)")
	agentsDiffCmd.Flags().BoolVar(&agentsDiffJSON, "json", false, "Output structured diff as JSON")
	agentsDiffCmd.Flags().StringVar(&agentsDiffScope, "scope", "", "Restrict resolution to scope (project|global)")

	agentsExplainCmd.Flags().StringVar(&agentsExplainScope, "scope", "", "Restrict resolution to scope (project|global)")

	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsShowCmd)
	agentsCmd.AddCommand(agentsValidateCmd)
	agentsCmd.AddCommand(agentsImportCmd)
	agentsCmd.AddCommand(agentsExportCmd)
	agentsCmd.AddCommand(agentsPromoteCmd)
	agentsCmd.AddCommand(agentsDemoteCmd)
	agentsCmd.AddCommand(agentsDiffCmd)
	agentsCmd.AddCommand(agentsExplainCmd)

	rootCmd.AddCommand(agentsCmd)
}
