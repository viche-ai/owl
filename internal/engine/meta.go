package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/llm"
	"github.com/viche-ai/owl/internal/tools"
)

// MetaAgentSystemPrompt is the fixed system prompt for the Owl meta-agent (always agent index 0).
const MetaAgentSystemPrompt = `You are the Owl meta-agent, displayed as "owl" in the sidebar.
You are the primary user interface for the Owl AI agent platform.

Your role:
1. Help users create, validate, and improve agent definitions
2. Show the resolved prompt stack for any agent (explain_agent)
3. Propose and apply edits to agent definition files with user confirmation
4. List available agents and project configuration

Tools available to you:
- list_agents: list all agent definitions across scopes
- create_agent: create AGENTS.md + agent.yaml for a new agent
- validate_agent: check an agent definition for structural errors
- explain_agent: show the resolved prompt stack for an agent
- suggest_edit: propose a file change as a unified diff (does NOT write yet)
- apply_edit: write the confirmed change, bump patch version, append CHANGELOG
- read_agent_file: read any file in an agent definition directory
- read_project_config: read the project context and guardrails
- shell_exec, file_read, file_write, file_edit: general file operations

WORKING PRINCIPLES:
- Propose file changes with suggest_edit first; only call apply_edit after user confirms
- When creating a new agent, always write both AGENTS.md and agent.yaml
- Validate definitions against project guardrails before creating
- Be concise — describe what you are doing without unnecessary commentary

FIRST-RUN:
If no agent definitions exist or the user asks what Owl can do, respond:
  "Welcome to Owl. What would you like to build or run?
   I can help you:
   - Create a new agent definition
   - Import an existing agent identity
   - Explore available agents
   Type a description of what you need, and I'll help set it up."

PROMPT MUTATION POLICY:
1. Call suggest_edit to show the diff with a [Proposed change] header
2. Wait for the user to reply with "yes", "apply", or "approve"
3. Call apply_edit to write the file, bump patch version, append CHANGELOG.md
4. On rejection: acknowledge and suggest alternatives`

// MetaAgentToolDefs returns the tool definitions for the Owl meta-agent.
func MetaAgentToolDefs() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Name:        "list_agents",
			Description: "List all available agent definitions across project and global scopes",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "Filter by scope: 'project', 'global', or empty for all",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "create_agent",
			Description: "Create a new agent definition with AGENTS.md and agent.yaml in the specified scope",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Agent name (hyphenated, e.g. code-reviewer)",
					},
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "Where to create the agent: 'project' or 'global'",
					},
					"agents_md": map[string]interface{}{
						"type":        "string",
						"description": "Content of AGENTS.md (the agent's system prompt / identity)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "One-line description of what the agent does",
					},
					"capabilities": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Capability tags (e.g. ['code-review', 'refactoring'])",
					},
					"default_model": map[string]interface{}{
						"type":        "string",
						"description": "Default LLM model ID (e.g. 'anthropic/claude-sonnet-4-6')",
					},
				},
				"required": []string{"name", "scope", "agents_md", "description"},
			},
		},
		{
			Name:        "validate_agent",
			Description: "Validate an agent definition against project guardrails and structural requirements",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":   map[string]interface{}{"type": "string", "description": "Agent name"},
					"scope":  map[string]interface{}{"type": "string", "description": "Scope: 'project', 'global', or '' for auto-resolve"},
					"strict": map[string]interface{}{"type": "boolean", "description": "Require agent.yaml with name, version, description"},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "explain_agent",
			Description: "Show the resolved prompt stack for an agent with source attribution for each layer",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":  map[string]interface{}{"type": "string", "description": "Agent name"},
					"scope": map[string]interface{}{"type": "string", "description": "Scope: 'project', 'global', or '' for auto-resolve"},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "suggest_edit",
			Description: "Propose an edit to an agent definition file as a unified diff. Does NOT apply the change — waits for user confirmation.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string", "description": "Agent name"},
					"scope":       map[string]interface{}{"type": "string", "description": "Scope: 'project', 'global', or '' for auto-resolve"},
					"file":        map[string]interface{}{"type": "string", "description": "File: 'AGENTS.md', 'agent.yaml', 'role.md', or 'guardrails.md'"},
					"new_content": map[string]interface{}{"type": "string", "description": "Proposed new full content for the file"},
					"reason":      map[string]interface{}{"type": "string", "description": "Why this change is suggested"},
				},
				"required": []string{"name", "file", "new_content", "reason"},
			},
		},
		{
			Name:        "apply_edit",
			Description: "Apply a confirmed edit to an agent definition file. Bumps the patch version in agent.yaml and appends a CHANGELOG.md entry.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":           map[string]interface{}{"type": "string", "description": "Agent name"},
					"scope":          map[string]interface{}{"type": "string", "description": "Scope: 'project' or 'global'"},
					"file":           map[string]interface{}{"type": "string", "description": "File to write"},
					"new_content":    map[string]interface{}{"type": "string", "description": "New content to write"},
					"change_summary": map[string]interface{}{"type": "string", "description": "One-line summary for CHANGELOG"},
				},
				"required": []string{"name", "scope", "file", "new_content", "change_summary"},
			},
		},
		{
			Name:        "read_agent_file",
			Description: "Read a file from an agent definition directory",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":  map[string]interface{}{"type": "string", "description": "Agent name"},
					"scope": map[string]interface{}{"type": "string", "description": "Scope: 'project', 'global', or '' for auto-resolve"},
					"file":  map[string]interface{}{"type": "string", "description": "Filename to read (e.g. 'AGENTS.md', 'agent.yaml')"},
				},
				"required": []string{"name", "file"},
			},
		},
		{
			Name:        "read_project_config",
			Description: "Read the current project configuration including context and guardrails",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []string{},
			},
		},
	}
}

// metaAgentWorkContext builds a workspace summary to inject into the meta-agent's system prompt.
func metaAgentWorkContext(workDir string) string {
	home, _ := os.UserHomeDir()
	agentsGlobalDir := filepath.Join(home, ".owl", "agents")

	var parts []string
	parts = append(parts, fmt.Sprintf("Working directory: %s", workDir))

	if entries, err := os.ReadDir(agentsGlobalDir); err == nil {
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		if len(names) > 0 {
			parts = append(parts, fmt.Sprintf("Global agents (%s): %s", agentsGlobalDir, strings.Join(names, ", ")))
		} else {
			parts = append(parts, "Global agents: none")
		}
	} else {
		parts = append(parts, "Global agents: none")
	}

	agentsProjectDir := filepath.Join(workDir, ".owl", "agents")
	if entries, err := os.ReadDir(agentsProjectDir); err == nil {
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		if len(names) > 0 {
			parts = append(parts, fmt.Sprintf("Project agents (%s): %s", agentsProjectDir, strings.Join(names, ", ")))
		} else {
			parts = append(parts, "Project agents: none")
		}
	} else {
		parts = append(parts, "Project agents: none")
	}

	return strings.Join(parts, "\n")
}

// runMetaAgent runs the Owl meta-agent lifecycle. It skips normal agent scaffolding
// (no LLM identity call, no Viche registration, no task ledger) and uses a fixed
// identity with meta-agent-specific tools.
func (e *AgentEngine) runMetaAgent(args *ipc.HatchArgs, inbox chan ipc.InboundMessage) {
	workDir := args.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	e.appendLog("> Initializing Owl meta-agent...\n")

	e.Mu(func() {
		e.State.Name = "owl"
		e.State.Role = "meta-agent"
	})

	// System tools: shell_exec, file_read, file_write, file_edit
	e.systemTools = &tools.SystemTools{WorkDir: workDir}
	for _, def := range e.systemTools.Definitions() {
		e.toolDefs = append(e.toolDefs, llm.ToolDef{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}

	// Meta-agent tools: list_agents, create_agent, validate_agent, etc.
	e.metaTools = &tools.MetaTools{WorkDir: workDir}
	for _, def := range MetaAgentToolDefs() {
		e.toolDefs = append(e.toolDefs, llm.ToolDef{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}

	workContext := metaAgentWorkContext(workDir)
	systemPrompt := MetaAgentSystemPrompt + "\n\n[WORKSPACE]\n" + workContext + "\n[END WORKSPACE]"

	e.messages = []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
	}

	e.appendLog("> Ready. Waiting for messages...\n")
	e.setState("idle")

	for msg := range inbox {
		e.appendLog(fmt.Sprintf("\n> [%s] %s\n", msg.From, msg.Content))
		e.processMessage(msg.Content)
	}

	e.appendLog("\n> Meta-agent stopped.\n")
	e.setState("stopped")
}
