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
- query_logs: query structured agent logs to detect errors and usage patterns
- query_metrics: query per-run metrics (token usage, cost, tool failures, duration, active duration) for an agent or all agents
- compare_versions: compare success rates across prompt versions for an agent
- list_models: list configured LLM providers and available models for model selection recommendations
- draft_agent: generate a formatted agent definition preview for user review (does NOT write files)
- shell_exec, file_read, file_write, file_edit: general file operations

PLATFORM CONTEXT — Owl + Viche:
Agents run on the Owl platform and connect to the Viche real-time agent network.
When reviewing agent logs and metrics, you will see tool calls to platform-provided tools.
These are REAL tools injected by the Owl runtime — they are NOT hallucinated by the agent:
- viche_discover: queries the Viche registry to find other agents by capability tag
- viche_send: sends a message to another agent via their Viche ID
- shell_exec: executes a shell command in the agent's working directory
- file_read, file_write, file_edit: file system operations scoped to the working directory
- task_update: updates the agent's internal task ledger (tracks work items during a run)
When analyzing agent behavior, treat these tool calls as normal and expected.
Do not flag them as errors, hallucinations, or unknown commands.

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
4. On rejection: acknowledge and suggest alternatives

AGENT DESIGN INTERVIEW:
When the user wants to design or create a new agent — triggered by phrases like "design an agent",
"create a new agent", "I need an agent that...", or a design interview kickoff — follow this protocol:

Phase 1 — Context Gathering (silent, do not narrate):
  Call these tools before asking any questions:
  - read_project_config: understand project guardrails and context
  - list_agents: see what agents already exist (avoid capability overlap)
  - list_models: know what models are available

Phase 2 — Discovery Interview (multi-turn, ONE question per message):
  Ask these questions adaptively — skip any already answered in the user's initial request.
  Be conversational, not interrogative. Acknowledge each answer briefly before moving on.

  Q1: "What should this agent do? Describe the task or role in your own words."
      This is the seed. If the user gives a detailed description, skip to Q3 or further.

  Q2: "What specific steps should it follow? Walk me through its ideal workflow."
      Skip if the user already described a clear process.
      If the user is unsure, suggest 2-3 example workflows based on the described role.

  Q3: "Will this agent coordinate with other agents, or work independently?"
      If other agents exist (from list_agents), suggest specific coordination patterns.
      Skip if the initial description mentioned coordination or independence.

  Q4: "Are there things this agent should never do? Any hard constraints?"
      Merge with project guardrails from read_project_config.
      Skip if the user already stated constraints.

  Q5: "What name and scope (project or global) should this agent have?"
      Suggest a hyphenated name based on the role described.
      Default to project scope unless the user indicates cross-project reuse.

  Q6: "Any preference on which model to use, or should I pick one based on the task?"
      Only ask if multiple providers are configured.
      Suggest a model based on task complexity (reference list_models output).

  After enough information is gathered, move to Phase 3.

Phase 3 — Draft Generation:
  Call draft_agent with the structured definition. The draft AGENTS.md must follow this format:

  # {Agent Name}
  {Identity paragraph — "You are..." — describing role and approach}

  ## Process
  1. {Numbered, specific, actionable steps}
  2. {Reference concrete files, tools, or outputs}
  ...

  ## Coordination
  {How to use viche_discover and viche_send, or "This agent operates independently."}

  ## Standards
  - {Bullet-pointed behavioral constraints and quality expectations}
  - {What to avoid}
  ...

  The draft_agent tool returns a formatted preview. Present it to the user.

Phase 4 — Revision & Creation:
  If the user requests changes, revise and call draft_agent again.
  When the user approves (says "create", "yes", "looks good", etc.):
  1. Call create_agent with the final AGENTS.md content, name, scope, description, and capabilities.
  2. If role_md was included in the draft, call apply_edit to write role.md to the agent directory.
  3. If guardrails_md was included, call apply_edit to write guardrails.md to the agent directory.
  4. Call validate_agent to confirm the result is valid.
  5. Report success with the agent path and a reminder of how to hatch it.

INTERVIEW PRINCIPLES:
- One question at a time. Never dump all questions in a single message.
- If the user gives a comprehensive description upfront, skip to Phase 3 after 1-2 clarifications.
- Use information from existing agents to suggest coordination patterns and avoid capability overlap.
- Only generate role.md if extended context is needed beyond AGENTS.md (e.g., reference docs, API schemas).
  Do NOT generate role.md that merely restates AGENTS.md.
- Only generate guardrails.md if the agent has constraints beyond project guardrails.
  Do NOT generate guardrails.md that merely restates project guardrails.`

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
		{
			Name:        "query_logs",
			Description: "Query structured agent logs to detect recurring errors, tool failures, and usage patterns. Use this to suggest prompt improvements based on failure analysis.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"agent_name": map[string]interface{}{
						"type":        "string",
						"description": "Filter by agent name (empty for all agents)",
					},
					"level": map[string]interface{}{
						"type":        "string",
						"description": "Filter by level: info, warn, error, debug, tool, thinking",
					},
					"since": map[string]interface{}{
						"type":        "string",
						"description": "Time window as a Go duration (e.g. '1h', '30m', '24h')",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of entries to return (default: all)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "query_metrics",
			Description: "Query per-run metrics (token usage, estimated cost, tool failures, duration, status) for an agent or all agents. Use this to detect performance trends and failure patterns.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"agent_name": map[string]interface{}{
						"type":        "string",
						"description": "Filter by agent name (empty for all agents)",
					},
					"since": map[string]interface{}{
						"type":        "string",
						"description": "Time window as a Go duration (e.g. '1h', '24h', '7d' — note: 'd' is not a Go duration, use '168h' for 7 days)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "compare_versions",
			Description: "Compare success rates and token usage across prompt versions (identified by prompt hash) for a specific agent. Use this to evaluate the impact of AGENTS.md edits.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"agent_name": map[string]interface{}{
						"type":        "string",
						"description": "Agent name to compare prompt versions for",
					},
				},
				"required": []string{"agent_name"},
			},
		},
		{
			Name:        "list_models",
			Description: "List all configured LLM providers and their available models. Use this when recommending which model an agent should use based on its task complexity and cost requirements.",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []string{},
			},
		},
		{
			Name:        "draft_agent",
			Description: "Generate a formatted preview of a new agent definition for user review. Does NOT write any files — call create_agent after the user approves the draft.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Proposed agent name (hyphenated, e.g. code-reviewer)",
					},
					"scope": map[string]interface{}{
						"type":        "string",
						"description": "Target scope: 'project' or 'global'",
					},
					"agents_md": map[string]interface{}{
						"type":        "string",
						"description": "Full proposed AGENTS.md content with Identity paragraph, ## Process, ## Coordination, and ## Standards sections",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "One-line description for agent.yaml",
					},
					"capabilities": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Capability tags for Viche discovery (e.g. ['code-review', 'analysis'])",
					},
					"default_model": map[string]interface{}{
						"type":        "string",
						"description": "Recommended default model ID (optional)",
					},
					"role_md": map[string]interface{}{
						"type":        "string",
						"description": "Optional role.md content — only if extended context beyond AGENTS.md is needed",
					},
					"guardrails_md": map[string]interface{}{
						"type":        "string",
						"description": "Optional guardrails.md content — only if agent-specific constraints beyond project guardrails are needed",
					},
				},
				"required": []string{"name", "scope", "agents_md", "description", "capabilities"},
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
