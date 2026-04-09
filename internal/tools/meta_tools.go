package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viche-ai/owl/internal/agents"
	"github.com/viche-ai/owl/internal/config"
	"github.com/viche-ai/owl/internal/logs"
	"github.com/viche-ai/owl/internal/metrics"
)

// MetaTools implements the Owl meta-agent tool set for managing agent definitions.
type MetaTools struct {
	WorkDir string
}

// Execute dispatches a tool call to the appropriate meta tool handler.
func (t *MetaTools) Execute(call ToolCall) string {
	switch call.Name {
	case "list_agents":
		return t.listAgents(call)
	case "create_agent":
		return t.createAgent(call)
	case "validate_agent":
		return t.validateAgent(call)
	case "explain_agent":
		return t.explainAgent(call)
	case "suggest_edit":
		return t.suggestEdit(call)
	case "apply_edit":
		return t.applyEdit(call)
	case "read_agent_file":
		return t.readAgentFile(call)
	case "read_project_config":
		return t.readProjectConfig(call)
	case "query_logs":
		return t.queryLogs(call)
	case "query_metrics":
		return t.queryMetrics(call)
	case "compare_versions":
		return t.compareVersions(call)
	default:
		return fmt.Sprintf("Unknown meta tool: %s", call.Name)
	}
}

func (t *MetaTools) listAgents(call ToolCall) string {
	scope, _ := call.Args["scope"].(string)
	resolver := agents.NewResolver(t.WorkDir)
	defs, err := resolver.List(scope)
	if err != nil {
		return fmt.Sprintf("Error listing agents: %v", err)
	}
	if len(defs) == 0 {
		return "No agent definitions found."
	}
	var lines []string
	for _, d := range defs {
		line := fmt.Sprintf("[%s] %s", d.Scope, d.Name)
		if d.Description != "" {
			line += " — " + d.Description
		}
		if len(d.Capabilities) > 0 {
			line += fmt.Sprintf(" (%s)", strings.Join(d.Capabilities, ", "))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (t *MetaTools) createAgent(call ToolCall) string {
	name, _ := call.Args["name"].(string)
	scope, _ := call.Args["scope"].(string)
	agentsMD, _ := call.Args["agents_md"].(string)
	description, _ := call.Args["description"].(string)
	capRaw, _ := call.Args["capabilities"].([]interface{})
	defaultModel, _ := call.Args["default_model"].(string)

	if name == "" || scope == "" || agentsMD == "" || description == "" {
		return "Error: name, scope, agents_md, and description are required"
	}
	if scope != "project" && scope != "global" {
		return "Error: scope must be 'project' or 'global'"
	}

	agentDir, err := t.resolveNewAgentDir(name, scope)
	if err != nil {
		return fmt.Sprintf("Error resolving agent directory: %v", err)
	}
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Sprintf("Error creating agent directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(agentDir, "AGENTS.md"), []byte(agentsMD), 0644); err != nil {
		return fmt.Sprintf("Error writing AGENTS.md: %v", err)
	}

	var caps []string
	for _, c := range capRaw {
		if s, ok := c.(string); ok {
			caps = append(caps, s)
		}
	}
	yamlContent := buildAgentYAML(name, "1.0.0", description, caps, defaultModel)
	if err := os.WriteFile(filepath.Join(agentDir, "agent.yaml"), []byte(yamlContent), 0644); err != nil {
		return fmt.Sprintf("Error writing agent.yaml: %v", err)
	}

	return fmt.Sprintf("Created agent %q in %s scope\n  Path: %s\n  Files: AGENTS.md, agent.yaml", name, scope, agentDir)
}

func (t *MetaTools) validateAgent(call ToolCall) string {
	name, _ := call.Args["name"].(string)
	scope, _ := call.Args["scope"].(string)
	strict, _ := call.Args["strict"].(bool)

	if name == "" {
		return "Error: name is required"
	}

	resolver := agents.NewResolver(t.WorkDir)
	def, err := resolver.Resolve(name, scope)
	if err != nil {
		return fmt.Sprintf("Error: agent %q not found: %v", name, err)
	}

	errs := resolver.Validate(def, strict)
	if len(errs) == 0 {
		return fmt.Sprintf("Agent %q [%s] is valid.", name, def.Scope)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Validation errors for %q [%s]:", name, def.Scope))
	for _, e := range errs {
		lines = append(lines, "  - "+e.Message)
	}
	return strings.Join(lines, "\n")
}

func (t *MetaTools) explainAgent(call ToolCall) string {
	name, _ := call.Args["name"].(string)
	scope, _ := call.Args["scope"].(string)

	if name == "" {
		return "Error: name is required"
	}

	resolver := agents.NewResolver(t.WorkDir)
	def, err := resolver.Resolve(name, scope)
	if err != nil {
		return fmt.Sprintf("Error: agent %q not found: %v", name, err)
	}

	globalCfg, _ := config.Load()
	projectCfg, _ := config.LoadProjectConfig(t.WorkDir)
	stack := agents.BuildPromptStack(def, globalCfg, projectCfg, "")
	return stack.Explain()
}

func (t *MetaTools) suggestEdit(call ToolCall) string {
	name, _ := call.Args["name"].(string)
	scope, _ := call.Args["scope"].(string)
	file, _ := call.Args["file"].(string)
	newContent, _ := call.Args["new_content"].(string)
	reason, _ := call.Args["reason"].(string)

	if name == "" || file == "" || newContent == "" {
		return "Error: name, file, and new_content are required"
	}

	resolver := agents.NewResolver(t.WorkDir)
	def, err := resolver.Resolve(name, scope)
	if err != nil {
		return fmt.Sprintf("Error: agent %q not found: %v", name, err)
	}

	filePath := filepath.Join(def.SourcePath, file)
	var oldContent string
	if data, readErr := os.ReadFile(filePath); readErr == nil {
		oldContent = string(data)
	}

	diff := buildUnifiedDiff(file, oldContent, newContent)
	return fmt.Sprintf("[Proposed change]\nAgent: %s [%s]\nFile: %s\nReason: %s\n\n%s\n\nReply with 'yes', 'apply', or 'approve' to apply this change, or 'no' to reject.",
		name, def.Scope, file, reason, diff)
}

func (t *MetaTools) applyEdit(call ToolCall) string {
	name, _ := call.Args["name"].(string)
	scope, _ := call.Args["scope"].(string)
	file, _ := call.Args["file"].(string)
	newContent, _ := call.Args["new_content"].(string)
	changeSummary, _ := call.Args["change_summary"].(string)

	if name == "" || scope == "" || file == "" || newContent == "" {
		return "Error: name, scope, file, and new_content are required"
	}

	resolver := agents.NewResolver(t.WorkDir)
	def, err := resolver.Resolve(name, scope)
	if err != nil {
		return fmt.Sprintf("Error: agent %q not found: %v", name, err)
	}

	filePath := filepath.Join(def.SourcePath, file)
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("Error writing %s: %v", file, err)
	}

	// Bump patch version in agent.yaml when changing other files
	if file != "agent.yaml" {
		bumpAgentVersion(def.SourcePath)
	}
	appendChangelog(def.SourcePath, changeSummary)

	return fmt.Sprintf("Applied change to %s/%s\nCHANGELOG updated.", name, file)
}

func (t *MetaTools) readAgentFile(call ToolCall) string {
	name, _ := call.Args["name"].(string)
	scope, _ := call.Args["scope"].(string)
	file, _ := call.Args["file"].(string)

	if name == "" || file == "" {
		return "Error: name and file are required"
	}

	resolver := agents.NewResolver(t.WorkDir)
	def, err := resolver.Resolve(name, scope)
	if err != nil {
		return fmt.Sprintf("Error: agent %q not found: %v", name, err)
	}

	data, err := os.ReadFile(filepath.Join(def.SourcePath, file))
	if err != nil {
		return fmt.Sprintf("Error reading %s: %v", file, err)
	}
	return string(data)
}

func (t *MetaTools) readProjectConfig(call ToolCall) string {
	projectCfg, err := config.LoadProjectConfig(t.WorkDir)
	if err != nil || projectCfg == nil {
		return "No project config found. Run 'owl project init' to create one."
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Project directory: %s", t.WorkDir))
	if projectCfg.Context != "" {
		parts = append(parts, fmt.Sprintf("\nContext:\n%s", projectCfg.Context))
	}
	if len(projectCfg.Guardrails) > 0 {
		parts = append(parts, fmt.Sprintf("\nGuardrails:\n- %s", strings.Join(projectCfg.Guardrails, "\n- ")))
	}
	if len(parts) == 1 {
		parts = append(parts, "(no context or guardrails configured)")
	}
	return strings.Join(parts, "\n")
}

func (t *MetaTools) queryLogs(call ToolCall) string {
	agentName, _ := call.Args["agent_name"].(string)
	level, _ := call.Args["level"].(string)
	sinceStr, _ := call.Args["since"].(string)
	limitFloat, _ := call.Args["limit"].(float64)
	limit := int(limitFloat)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("Error: could not determine home directory: %v", err)
	}
	reader := &logs.Reader{LogDir: filepath.Join(home, ".owl", "logs")}

	opts := logs.QueryOpts{
		AgentName: agentName,
		Level:     level,
		Limit:     limit,
	}
	if sinceStr != "" {
		d, err := time.ParseDuration(sinceStr)
		if err != nil {
			return fmt.Sprintf("Error: invalid 'since' duration %q (use Go duration like '1h', '30m'): %v", sinceStr, err)
		}
		opts.Since = time.Now().Add(-d)
	}

	entries, err := reader.Query(opts)
	if err != nil {
		return fmt.Sprintf("Error querying logs: %v", err)
	}
	if len(entries) == 0 {
		return "No log entries found matching the criteria."
	}

	var lines []string
	for _, e := range entries {
		line := fmt.Sprintf("[%s] [%s] [%s] %s",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.AgentName,
			e.Level,
			strings.TrimRight(e.Message, "\n"),
		)
		if e.ToolName != "" {
			line += fmt.Sprintf(" (tool: %s)", e.ToolName)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (t *MetaTools) queryMetrics(call ToolCall) string {
	agentName, _ := call.Args["agent_name"].(string)
	sinceStr, _ := call.Args["since"].(string)

	store, err := metrics.NewStore()
	if err != nil {
		return fmt.Sprintf("Error: could not open metrics store: %v", err)
	}

	var since time.Time
	if sinceStr != "" {
		d, err := time.ParseDuration(sinceStr)
		if err != nil {
			return fmt.Sprintf("Error: invalid 'since' duration %q (use Go duration like '1h', '24h'): %v", sinceStr, err)
		}
		since = time.Now().Add(-d)
	}

	result, err := store.QueryMetrics(agentName, since)
	if err != nil {
		return fmt.Sprintf("Error querying metrics: %v", err)
	}
	return result
}

func (t *MetaTools) compareVersions(call ToolCall) string {
	agentName, _ := call.Args["agent_name"].(string)
	if agentName == "" {
		return "Error: agent_name is required"
	}

	store, err := metrics.NewStore()
	if err != nil {
		return fmt.Sprintf("Error: could not open metrics store: %v", err)
	}

	result, err := store.CompareVersions(agentName)
	if err != nil {
		return fmt.Sprintf("Error comparing versions: %v", err)
	}
	return result
}

// resolveNewAgentDir returns the target directory path for a new agent definition.
func (t *MetaTools) resolveNewAgentDir(name, scope string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch scope {
	case "global":
		return filepath.Join(home, ".owl", "agents", name), nil
	case "project":
		// Walk up from WorkDir to find the project root (.owl directory)
		dir := t.WorkDir
		for {
			if _, err := os.Stat(filepath.Join(dir, ".owl")); err == nil {
				return filepath.Join(dir, ".owl", "agents", name), nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		// Fallback: create in WorkDir
		return filepath.Join(t.WorkDir, ".owl", "agents", name), nil
	default:
		return "", fmt.Errorf("unknown scope: %s", scope)
	}
}

// buildAgentYAML generates a minimal agent.yaml.
func buildAgentYAML(name, version, description string, capabilities []string, defaultModel string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "name: %s\n", name)
	fmt.Fprintf(&sb, "version: %s\n", version)
	fmt.Fprintf(&sb, "description: %s\n", description)
	if len(capabilities) > 0 {
		sb.WriteString("capabilities:\n")
		for _, c := range capabilities {
			fmt.Fprintf(&sb, "  - %s\n", c)
		}
	} else {
		sb.WriteString("capabilities: []\n")
	}
	if defaultModel != "" {
		fmt.Fprintf(&sb, "default_model: %s\n", defaultModel)
	}
	return sb.String()
}

// buildUnifiedDiff produces a simple unified diff between old and new content.
func buildUnifiedDiff(filename, oldContent, newContent string) string {
	if oldContent == newContent {
		return "(no changes)"
	}

	oldLines := strings.Split(strings.TrimRight(oldContent, "\n"), "\n")
	newLines := strings.Split(strings.TrimRight(newContent, "\n"), "\n")

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n", filename)
	fmt.Fprintf(&sb, "+++ b/%s\n", filename)
	sb.WriteString("@@ changes @@\n")

	// Build a simple Myers-style diff using line comparison
	// For readability, show a context of removed then added lines per changed block
	i, j := 0, 0
	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			// Unchanged — skip context lines for brevity
			i++
			j++
			continue
		}
		// Find the end of the changed block
		oi, nj := i, j
		for i < len(oldLines) && (j >= len(newLines) || oldLines[i] != newLines[j]) {
			i++
		}
		for j < len(newLines) && (i >= len(oldLines) || oldLines[i] != newLines[j]) {
			j++
		}
		fmt.Fprintf(&sb, "@@ -%d +%d @@\n", oi+1, nj+1)
		for k := oi; k < i; k++ {
			fmt.Fprintf(&sb, "-%s\n", oldLines[k])
		}
		for k := nj; k < j; k++ {
			fmt.Fprintf(&sb, "+%s\n", newLines[k])
		}
	}

	return sb.String()
}

// bumpAgentVersion increments the patch version in agent.yaml.
func bumpAgentVersion(agentDir string) {
	yamlPath := filepath.Join(agentDir, "agent.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "version:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				ver := strings.TrimSpace(parts[1])
				lines[i] = "version: " + bumpPatchVersion(ver)
				break
			}
		}
	}
	_ = os.WriteFile(yamlPath, []byte(strings.Join(lines, "\n")), 0644)
}

func bumpPatchVersion(ver string) string {
	parts := strings.Split(ver, ".")
	if len(parts) == 3 {
		var patch int
		_, _ = fmt.Sscanf(parts[2], "%d", &patch)
		patch++
		return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch)
	}
	return ver + ".1"
}

// appendChangelog prepends an entry to CHANGELOG.md in the agent directory.
func appendChangelog(agentDir, changeSummary string) {
	changelogPath := filepath.Join(agentDir, "CHANGELOG.md")
	var existing string
	if data, err := os.ReadFile(changelogPath); err == nil {
		existing = string(data)
	}
	date := time.Now().Format("2006-01-02")
	entry := fmt.Sprintf("## %s\n- %s\n\n", date, changeSummary)
	_ = os.WriteFile(changelogPath, []byte(entry+existing), 0644)
}
