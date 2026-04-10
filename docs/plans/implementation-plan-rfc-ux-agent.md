# Implementation Plan: Owl UX + Agent Experience Overhaul

**Source RFC:** `docs/RFC-owl-ux-agent-experience.md`
**Date:** 2026-04-09
**Codebase snapshot:** commit `2d7703a` (main)

---

## Codebase Inventory (Current State)

Before detailing each phase, here is a summary of the existing architecture that each phase builds on:

| Component | File(s) | Current State |
|---|---|---|
| CLI entry + hatch command | `cmd/owl/main.go` | Cobra-based. `hatch [desc]` with `--template`, `--model`, `--harness`, etc. Template loaded from `~/.owl/templates/<name>.json` |
| Project commands | `cmd/owl/project.go` | `owl project init`, `agents` (AGENTS.md wizard), `guards`, `templates {list,create,delete}` |
| Config commands | `cmd/owl/config.go` | `owl config show/set-key/set-model/import` |
| Viche commands | `cmd/owl/viche.go` | `owl viche add-registry/set-default/status` |
| Setup wizard | `cmd/owl/setup.go` | Interactive first-run config |
| IPC / RPC API | `internal/ipc/api.go` | `Service` struct with `Agents []*AgentState`, methods: `Hatch`, `Kill`, `CloneAgent`, `SendMessage`, `SetAgentModel`, `SetAgentConfig`, `ListAgents`, `StreamExternalAgent` |
| Agent engine | `internal/engine/engine.go` | `AgentEngine.Run()` — scaffolding, Viche registration, tool setup, prompt assembly (GLOBAL + PROJECT CONTEXT + PROJECT GUARDRAILS + RUNTIME), conversation loop |
| Harness support | `internal/engine/harness.go` | External subprocess for codex/opencode/claude-code |
| Task ledger | `internal/engine/tasks.go` | Per-agent JSONL at `~/.owl/agents/<id>/tasks.jsonl` |
| Global config | `internal/config/config.go` | `~/.owl/config.json` — models, providers, Viche registries, system_prompt |
| Project config | `internal/config/project.go` | `.owl/project.json` — context, guardrails. Walk-up directory search with cache |
| LLM router | `internal/llm/router.go` | `provider/model` format resolution |
| TUI | `internal/tui/tui.go` | BubbleTea, Everforest theme, sidebar + main pane, 500ms poll via RPC |
| Daemon | `cmd/owld/main.go` | Unix socket RPC + HTTP API on :7890 for external agent streaming |

**Key observations:**
- Agent state is **in-memory only** in the daemon. No persistence across daemon restarts.
- Templates are flat JSON files with no layering or resolution precedence.
- No `owl runs`, `owl logs`, `owl agents`, or `owl metrics` command groups exist yet.
- The `Kill` RPC deletes the agent from the slice entirely — no archive/history.
- Logging is unstructured strings appended to `~/.owl/logs/agent-<name>-<ts>.log`.
- No structured telemetry or metrics capture exists.
- The TUI console has hardcoded commands (`hatch`, `kill`, `clone`, `viche`, `clear`, `help`) — no extensibility pattern.

---

## Phase 1: CLI Contract + Naming Migration

**RFC sections:** 5.2 (Hatch Command UX), 5.3 (Naming/Mental Model), 12 (Backwards Compatibility)

### Goal
Replace `--template` with `--agent` as the primary flag, introduce `--dry-run`, `--scope`, `--from-file`, and set up deprecation pathways.

### Files to Modify

1. **`cmd/owl/main.go`**
   - Add new flags: `--agent`, `--from-file`, `--scope`, `--dry-run`
   - Keep `--template` as hidden/deprecated alias for `--agent`
   - Emit deprecation warning when `--template` is used
   - When bare positional `[description...]` is used without `--agent`, emit migration guidance warning
   - Implement `--dry-run`: resolve agent definition + prompt stack, print to stdout, exit without RPC call
   - Implement `--from-file`: read prompt content from a file path, use as description override

2. **`internal/ipc/api.go`**
   - Rename `HatchArgs.Template` field to `HatchArgs.Agent` (string)
   - Add `HatchArgs.Scope` field (string: `"project"`, `"global"`, or empty for auto-resolve)
   - Add `HatchArgs.DryRun` field (bool) — daemon returns resolved config without spawning

3. **`internal/ipc/api.go` — new RPC method**
   - Add `Service.DryRunHatch(args *HatchArgs, reply *DryRunReply)` that returns the resolved agent definition, prompt stack, and validation result without creating an agent

### Implementation Steps

```
1. Add new flag variables in cmd/owl/main.go:
     var agentFlag string      // --agent <name>
     var fromFileFlag string   // --from-file <path>
     var scopeFlag string      // --scope project|global
     var dryRunFlag bool       // --dry-run

2. Register flags on hatchCmd:
     hatchCmd.Flags().StringVar(&agentFlag, "agent", "", "Use a named agent definition")
     hatchCmd.Flags().StringVar(&fromFileFlag, "from-file", "", "Load prompt from file")
     hatchCmd.Flags().StringVar(&scopeFlag, "scope", "", "Scope hint (project|global)")
     hatchCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Show resolved setup without executing")

3. Mark --template as deprecated:
     hatchCmd.Flags().MarkDeprecated("template", "use --agent instead")

4. In hatchCmd.Run:
   a. If --template is set and --agent is not, copy templateFlag → agentFlag, print warning
   b. If bare positional args given without --agent, print migration hint:
      "Tip: For reproducible hatching, define an agent and use: owl hatch --agent <name>"
   c. If --from-file is set, read file contents into desc (error if file not found)
   d. If --dry-run, call Daemon.DryRunHatch instead of Daemon.Hatch, print result, exit

5. Update HatchArgs struct in ipc/api.go:
   - Rename Template → Agent
   - Add Scope string
   - Add DryRun bool

6. Update template loading logic in main.go:
   - Current: loads from ~/.owl/templates/<templateFlag>.json
   - New: when agentFlag is set, resolve agent definition from:
     a. Project scope: .owl/agents/<agentFlag>/AGENTS.md (and agent.yaml if exists)
     b. Global scope: ~/.owl/agents/<agentFlag>/AGENTS.md (and agent.yaml if exists)
     c. Fallback: ~/.owl/templates/<agentFlag>.json (legacy, with deprecation warning)
   - If --scope is provided, restrict search to that scope only
   - Resolution order: project > global > legacy templates

7. Add DryRunHatch RPC method to Service:
   - Resolve agent definition
   - Build prompt stack
   - Validate required fields
   - Return structured response with resolved config

8. Add validation — hatch fails fast if:
   - --agent specified but agent definition not found at any scope
   - Required fields missing in agent definition
   - --scope invalid (not "project" or "global")
```

### Conversion helper: `owl migrate templates`

New file: **`cmd/owl/migrate.go`**

```
1. Add migrateCmd cobra.Command group
2. Add migrateTemplatesCmd:
   - Scan ~/.owl/templates/*.json and .owl/templates/*.json
   - For each template JSON, create:
     <scope>/agents/<name>/AGENTS.md (converted from system_prompt + description)
     <scope>/agents/<name>/agent.yaml (from name, model, thinking, effort, capabilities)
   - Print conversion report
3. Add migrateCheckCmd:
   - Scan for any usage of --template in shell history or scripts (best-effort)
   - Report templates not yet converted
   - Report any agent definitions missing required fields

4. Register: rootCmd.AddCommand(migrateCmd)
```

### Tests

- Unit test: `--template` flag sets agentFlag with deprecation warning on stderr
- Unit test: `--dry-run` returns resolved config without creating agent
- Unit test: `--from-file` reads file content correctly
- Unit test: `--scope project` restricts resolution to project scope only
- Unit test: validation fails fast for missing agent definition
- Integration test: `owl migrate templates` converts a sample template JSON to AGENTS.md format

---

## Phase 2: Agent Definition Format + Resolver

**RFC sections:** 6.1 (Core Direction), 6.2 (Layered Prompt Architecture), 6.3 (Metadata Schema)

### Goal
Implement the AGENTS.md-style agent definition format with optional `agent.yaml` sidecar, and a layered prompt resolver.

### New Package: `internal/agents/`

Create a new package to own agent definition loading, resolution, and validation.

#### File: `internal/agents/definition.go`

```go
// AgentDefinition represents a fully resolved agent identity
type AgentDefinition struct {
    Name             string
    Version          string
    Description      string
    Capabilities     []string
    AllowedWorkspaces []string
    DefaultModel     string
    PromptLayers     []string
    Owner            string
    Tags             []string
    
    // Resolved prompt content
    AgentsMD    string   // Content of AGENTS.md
    RoleMD      string   // Content of role.md (optional)
    GuardrailsMD string  // Content of guardrails.md (optional)
    
    // Source tracking
    Scope       string   // "project" or "global"
    SourcePath  string   // Absolute path to agent directory
}
```

#### File: `internal/agents/resolver.go`

```go
// Resolver handles agent definition lookup with scope precedence
type Resolver struct {
    ProjectDir string  // .owl/agents/ relative to project root
    GlobalDir  string  // ~/.owl/agents/
}

// Resolve finds an agent definition by name with scope precedence
// Resolution order: project > global
func (r *Resolver) Resolve(name string, scopeHint string) (*AgentDefinition, error)

// List returns all available agent definitions across scopes
func (r *Resolver) List(scopeFilter string) ([]AgentDefinition, error)

// Validate checks an agent definition for required fields and guardrail compliance
func (r *Resolver) Validate(def *AgentDefinition, strict bool) []ValidationError
```

#### File: `internal/agents/prompt.go`

Implements the 5-tier prompt resolution stack:

```go
// PromptStack represents the fully resolved prompt with source attribution
type PromptStack struct {
    Layers []PromptLayer
}

type PromptLayer struct {
    Name     string // "runtime-override", "project-agent", "project-policy", "global-agent", "owl-defaults"
    Source   string // file path or "cli-flag"
    Content  string
    Priority int    // 1 (highest) to 5 (lowest)
}

// BuildPromptStack resolves the full prompt for a given agent + runtime overrides
func BuildPromptStack(
    def *AgentDefinition,          // resolved agent definition (may be nil for ad-hoc hatch)
    globalCfg *config.Config,      // ~/.owl/config.json
    projectCfg *config.ProjectConfig, // .owl/project.json
    runtimePrompt string,          // --prompt or --from-file override
) *PromptStack

// Render produces the final system prompt string from the stack
func (ps *PromptStack) Render() string

// Explain produces a human-readable breakdown of each layer and its source
func (ps *PromptStack) Explain() string
```

### Implementation Steps

```
1. Create directory: internal/agents/

2. Implement definition.go:
   - AgentDefinition struct
   - LoadFromDirectory(dirPath string) (*AgentDefinition, error):
     a. Read AGENTS.md from dirPath (required — error if missing)
     b. Read agent.yaml if present, parse into struct fields
     c. Read role.md if present
     d. Read guardrails.md if present
     e. Return populated AgentDefinition

3. Implement resolver.go:
   - Resolver struct with ProjectDir/GlobalDir paths
   - NewResolver(workDir string) *Resolver:
     a. Find project root by walking up looking for .owl/
     b. Set ProjectDir = <project_root>/.owl/agents/
     c. Set GlobalDir = ~/.owl/agents/
   - Resolve(name, scopeHint):
     a. If scopeHint == "project", only check ProjectDir
     b. If scopeHint == "global", only check GlobalDir
     c. Otherwise: check ProjectDir first, then GlobalDir
     d. Call LoadFromDirectory on the found path
     e. Return error if not found in any scope
   - List(scopeFilter):
     a. Scan directories for subdirectories containing AGENTS.md
     b. Load each, tag with scope
   - Validate(def, strict):
     a. Check AGENTS.md non-empty
     b. If strict: require agent.yaml with name, version, description
     c. Check capabilities non-empty
     d. Return []ValidationError

4. Implement prompt.go:
   - BuildPromptStack:
     a. Layer 5 (lowest): Owl system defaults from config.DefaultSystemPrompt()
     b. Layer 4: Global agent base — def.AgentsMD if scope == "global"
     c. Layer 3: Project shared policy — projectCfg.Context + projectCfg.Guardrails
     d. Layer 2: Project agent overlays — def.AgentsMD + def.RoleMD + def.GuardrailsMD if scope == "project"
     e. Layer 1 (highest): Runtime override — runtimePrompt from --prompt/--from-file
   - Render(): concatenate layers in priority order, separated by section headers
   - Explain(): produce formatted output showing each layer with its source file path

5. Implement agent.yaml parsing:
   - Use gopkg.in/yaml.v3 (add dependency)
   - Parse into AgentYAML struct matching RFC schema:
     name, version, description, capabilities[], allowed_workspaces[],
     default_model, prompt_layers[], owner, tags[]

6. Update engine.go prompt assembly (lines 300-360):
   - Replace the current hardcoded prompt builder with BuildPromptStack()
   - Current: manually builds [GLOBAL] + [PROJECT CONTEXT] + [PROJECT GUARDRAILS] + [RUNTIME]
   - New: use agents.BuildPromptStack(def, globalCfg, projectCfg, runtimeOverride).Render()
   - This keeps the same output format but makes it composable and explainable

7. Update cmd/owl/main.go hatch flow:
   - When --agent is provided, use agents.Resolver to find and load definition
   - Pass resolved AgentDefinition through to engine via HatchArgs (add new fields or pass via separate channel)
   - When no --agent, behave as today (ad-hoc hatch with description)
```

### New CLI command: `owl explain <agent>`

Add to **`cmd/owl/main.go`** or new file **`cmd/owl/explain.go`**:

```
1. Create explainCmd:
   - Args: agent name (required)
   - Flags: --scope (optional)
   - Logic:
     a. Create agents.Resolver
     b. Resolve agent by name
     c. Build prompt stack
     d. Print Explain() output showing each layer with source
   - No daemon connection required (pure local resolution)

2. Register: rootCmd.AddCommand(explainCmd)
```

### Directory Structure Created

```
~/.owl/agents/                     # Global scope
  reviewer/
    AGENTS.md                      # Primary definition (required)
    role.md                        # Optional role elaboration
    guardrails.md                  # Optional agent-specific guardrails
    agent.yaml                     # Optional strict metadata sidecar
    metrics.md                     # Generated/maintained (Phase 6)
    CHANGELOG.md                   # Version history (Phase 6)

.owl/agents/                       # Project scope (same structure)
  reviewer/
    AGENTS.md
    ...
```

### Tests

- Unit test: `LoadFromDirectory` correctly parses AGENTS.md + agent.yaml + role.md
- Unit test: `Resolve` returns project scope when both exist (precedence)
- Unit test: `Resolve` falls back to global when no project agent
- Unit test: `Resolve` respects scopeHint
- Unit test: `BuildPromptStack` produces correct 5-layer output
- Unit test: `Validate` catches missing AGENTS.md, empty capabilities in strict mode
- Unit test: `Explain` output contains source file paths for each layer

---

## Phase 3: Meta-Agent Console Scaffold + Validation Workflows

**RFC sections:** 5.1 (Main Console Experience), 11 (Prompt Mutation Policy), 13 (Security/Guardrails)

### Goal
Add a meta-agent that runs in the TUI main console tab, providing guided agent creation, validation, and prompt improvement suggestions.

### Architecture Decision

The meta-agent is an `AgentEngine` instance with a special system prompt and tool set, running as agent index 0 in the daemon. It is always present when the daemon starts.

### Files to Modify

1. **`cmd/owld/main.go`** — Auto-hatch meta-agent on daemon startup
2. **`internal/engine/meta.go`** — New file for meta-agent system prompt and tools
3. **`internal/tools/meta_tools.go`** — New tools: `create_agent_definition`, `validate_agent`, `explain_prompt_stack`, `suggest_improvement`, `apply_patch`
4. **`internal/tui/tui.go`** — Show meta-agent as first tab (console), adapt rendering

### Implementation Steps

```
1. Create internal/engine/meta.go:
   - Define MetaAgentSystemPrompt constant with instructions:
     * "You are the Owl meta-agent. You help users create, validate, and improve agent definitions."
     * Context-aware: knows about workspace, project settings, available agents
     * Guardrail-aware: validates definitions against project guardrails
     * Prompt-quality-aware: can analyze and suggest improvements
     * Default mode: proposes changes as diffs, never applies silently
   - Define MetaAgentTools() returning tool definitions for:
     a. list_agents — list available agent definitions (calls agents.Resolver.List)
     b. create_agent — create a new AGENTS.md + agent.yaml in specified scope
     c. validate_agent — validate agent definition against guardrails
     d. explain_agent — show resolved prompt stack for an agent
     e. suggest_edit — propose a diff/patch to an agent's AGENTS.md
     f. apply_edit — apply a previously proposed edit (requires user confirmation)
     g. read_agent_file — read any file from an agent definition directory
     h. read_project_config — read current project config and guardrails

2. Create internal/tools/meta_tools.go:
   - Implement MetaTools struct with Execute(call) dispatcher
   - Each tool calls into internal/agents/ package for resolution/validation
   - apply_edit implementation:
     a. Generate diff of proposed change
     b. Present to user as formatted patch
     c. Only apply after explicit confirmation (via inbox message "yes"/"approve")
     d. If applied: increment version in agent.yaml, append to CHANGELOG.md

3. Modify cmd/owld/main.go:
   - After service initialization, auto-hatch meta-agent:
     service.Hatch(&ipc.HatchArgs{
       Name:        "owl",
       Description: "Meta-agent: Owl control surface",
       Ambient:     true,  // waits for user input
     }, &reply)
   - Set special flag so engine knows to use meta-agent system prompt + tools

4. Modify internal/ipc/api.go:
   - Add HatchArgs.MetaAgent bool field (internal-only, set by daemon)
   - When MetaAgent is true, engine.Run uses MetaAgentSystemPrompt and MetaAgentTools

5. Modify engine.go:
   - In Run(), after model resolution, check if args.MetaAgent
   - If true: skip normal scaffolding, use meta-agent identity directly
   - Set up meta-agent tools instead of standard tools
   - Inject workspace/project context into system prompt

6. Modify tui.go:
   - Tab 0 (index 0) always shows the meta-agent
   - Label it "owl" or "console" in the sidebar
   - The console input sends messages to agent 0's inbox
   - Remove existing hardcoded console command handling (hatch/kill/clone)
     — these become meta-agent tool calls or are kept as shortcuts

7. Prompt mutation policy (RFC section 11):
   - Default v1 flow in apply_edit tool:
     a. Meta-agent generates proposed edit as unified diff
     b. Display diff in TUI with "[Proposed change]" header
     c. Wait for user confirmation message
     d. On approval: write changes, bump version in agent.yaml, append CHANGELOG entry
     e. On rejection: acknowledge and suggest alternatives
   - CHANGELOG.md format:
     ## v1.1.0 — 2026-04-09
     - Updated guardrails to include migration safety checks
     - Source: meta-agent suggestion based on 3 failed runs

8. Guardrails enforcement (RFC section 13):
   - validate_agent tool checks:
     a. Workspace boundaries: if allowed_workspaces in agent.yaml, verify current workspace matches
     b. Capability checks: ensure capabilities are valid identifiers
     c. Guardrail compliance: agent's AGENTS.md must not contradict project guardrails
     d. Sensitive content: warn if agent definition contains API keys or secrets
   - Meta-agent cannot bypass guardrails or escalate privileges
   - All prompt changes are auditable via CHANGELOG.md
```

### First-Run Experience

When a user opens Owl with no agents configured:
```
Meta-agent greets user:
  "Welcome to Owl. What would you like to build or run?"
  "I can help you:
   - Create a new agent definition
   - Import an existing agent identity
   - Explore available agents
   Type a description of what you need, and I'll help set it up."
```

### Tests

- Unit test: meta-agent tools correctly call agents.Resolver
- Unit test: create_agent produces valid AGENTS.md + agent.yaml
- Unit test: validate_agent catches guardrail mismatches
- Unit test: apply_edit requires confirmation before writing
- Integration test: meta-agent responds to "create a code reviewer agent" with appropriate tool calls

---

## Phase 4: Logs CLI + Structured Query Interface

**RFC sections:** 8.1 (Logs CLI), 8.2 (Meta-Agent Logs Usage)

### Goal
Add `owl logs` command group with structured output, and enable the meta-agent to consume log data for improvement suggestions.

### Current Logging State

- Agent logs written to `~/.owl/logs/agent-<name>-<timestamp>.log` (unstructured text)
- Logs also stored in `AgentState.Logs` string (in-memory, unbounded)
- No structured format, no run IDs, no queryable fields

### New Structured Log Format

Before adding CLI commands, we need structured logging. Each log entry becomes a JSON line.

#### File: `internal/logs/structured.go`

```go
type LogEntry struct {
    Timestamp  time.Time `json:"ts"`
    RunID      string    `json:"run_id"`
    AgentName  string    `json:"agent_name"`
    AgentID    string    `json:"agent_id"`
    Level      string    `json:"level"`     // info, warn, error, debug, tool, thinking
    Message    string    `json:"message"`
    ToolName   string    `json:"tool_name,omitempty"`
    ToolResult string    `json:"tool_result,omitempty"`
    ModelID    string    `json:"model_id,omitempty"`
    TokensIn   int      `json:"tokens_in,omitempty"`
    TokensOut  int      `json:"tokens_out,omitempty"`
}

// Writer wraps an io.Writer with structured JSON line output
type Writer struct {
    file     *os.File
    runID    string
    agentName string
    agentID  string
}

func (w *Writer) Log(level, message string)
func (w *Writer) LogTool(toolName, args, result string)
func (w *Writer) LogUsage(tokensIn, tokensOut int, modelID string)
```

#### File: `internal/logs/reader.go`

```go
// Reader provides query access to log files
type Reader struct {
    LogDir string // ~/.owl/logs/
}

// List returns available log files with metadata
func (r *Reader) List() ([]LogFileMeta, error)

// Read returns all entries from a log file
func (r *Reader) Read(runID string) ([]LogEntry, error)

// Query returns filtered entries
func (r *Reader) Query(opts QueryOpts) ([]LogEntry, error)

// Tail streams new entries from the latest (or specified) log file
func (r *Reader) Tail(agentName string, follow bool) (<-chan LogEntry, error)

type QueryOpts struct {
    AgentName string
    Since     time.Time
    Until     time.Time
    Level     string
    Limit     int
}

type LogFileMeta struct {
    RunID     string
    AgentName string
    StartTime time.Time
    Path      string
    Size      int64
}
```

### Implementation Steps

```
1. Create internal/logs/ package with structured.go and reader.go

2. Generate RunID:
   - In ipc.Service.Hatch(), generate a unique RunID for each agent:
     RunID = fmt.Sprintf("%s-%s", sanitizeName(args.Name), ulid.Make().String())
   - Add RunID field to AgentState
   - Pass RunID to engine.Run() for log file naming

3. Update log file naming:
   - Current: agent-<name>-<timestamp>.log
   - New: <run_id>.jsonl (e.g., reviewer-01JRQX5ABC.jsonl)
   - Keep a .meta.json sidecar with: run_id, agent_name, agent_id, start_time, model_id, status

4. Update engine.go logging:
   - Replace appendLog() with structured Writer
   - Each appendLog call becomes w.Log(level, message)
   - Tool executions become w.LogTool(name, args, result)
   - Usage events become w.LogUsage(in, out, model)
   - For TUI display: continue appending to State.Logs (render from structured entries)

5. Create cmd/owl/logs.go:

   a. owl logs list
      - Scan ~/.owl/logs/ for .meta.json files
      - Print table: RUN_ID | AGENT | STARTED | STATUS | SIZE
      - Flags: --agent <name> (filter), --json (structured output)

   b. owl logs show <run-id>
      - Read <run-id>.jsonl
      - Pretty-print entries with color coding by level
      - Flags: --json (raw JSON lines), --level <level> (filter)

   c. owl logs tail [--agent <name>]
      - Find most recent log for agent (or most recent overall)
      - Stream new lines as they're appended (fsnotify or poll)
      - Flags: --follow/-f (continuous), --json

   d. owl logs query --agent <name> --since <time> --json
      - Use Reader.Query() with structured filters
      - Output JSON lines for machine consumption
      - Flags: --until, --level, --limit

6. Register commands:
   logsCmd.AddCommand(logsListCmd, logsShowCmd, logsTailCmd, logsQueryCmd)
   rootCmd.AddCommand(logsCmd)

7. Redaction support:
   - Add redaction patterns to config (or hardcoded defaults):
     API keys (sk-*, key-*), tokens, passwords
   - Reader applies redaction before output
   - Flag: --no-redact to disable (requires confirmation)

8. Meta-agent log consumption (RFC 8.2):
   - Add meta-agent tool: query_logs(agent_name, since, level)
   - Tool calls logs.Reader.Query() and returns structured results
   - Meta-agent can then:
     a. Detect recurring error patterns
     b. Identify tool failures
     c. Correlate issues with model/provider
     d. Suggest prompt improvements based on failure analysis
```

### Stable Schema Contract

The JSON log schema must be versioned:
```json
{
  "schema_version": "1",
  "ts": "2026-04-09T10:30:00Z",
  "run_id": "reviewer-01JRQX5ABC",
  "agent_name": "reviewer",
  "level": "info",
  "message": "Starting work on task..."
}
```
Document the schema in `docs/design/06-log-schema.md`.

### Dependencies

- Add `github.com/oklog/ulid/v2` for RunID generation (or use `crypto/rand` + timestamp for a simpler ULID-like ID)
- Optional: `github.com/fsnotify/fsnotify` for `logs tail --follow`

### Tests

- Unit test: `Writer` produces valid JSONL
- Unit test: `Reader.Query` filters by agent, time range, level
- Unit test: `Reader.List` finds all log files with correct metadata
- Unit test: Redaction removes API key patterns
- Integration test: `owl logs list` after hatching an agent shows the run
- Integration test: `owl logs show <run-id>` produces colored output

---

## Phase 5: Run Controls (Stop/Remove/Inspect)

**RFC sections:** 9.1 (CLI Controls), 9.2 (TUI Controls), 9.3 (Safety Semantics)

### Goal
Add `owl runs` command group with list/stop/remove/inspect, update TUI with matching controls, and implement graceful stop + archive semantics.

### Current State Analysis

- `Kill` RPC closes inbox channel and removes agent from slice entirely — no history preserved
- No concept of "archived" or "completed" runs
- Agent indices shift after kill (map rebuild) — fragile for external references
- No `inspect` capability beyond reading the TUI log pane

### Architecture Changes

#### Run ID-based addressing
Replace index-based agent addressing with stable RunID (introduced in Phase 4).

#### Agent state persistence
Stopped/archived agents need to persist for inspection after removal from active list.

### Files to Modify

1. **`internal/ipc/api.go`** — Major changes to agent state management
2. **`cmd/owl/runs.go`** — New file for runs CLI commands
3. **`internal/tui/tui.go`** — TUI controls for stop/remove/inspect
4. **`internal/runs/store.go`** — New package for run state persistence

### New Package: `internal/runs/`

#### File: `internal/runs/store.go`

```go
// RunRecord persists essential run metadata to disk
type RunRecord struct {
    RunID       string    `json:"run_id"`
    AgentName   string    `json:"agent_name"`
    AgentDef    string    `json:"agent_def,omitempty"`  // agent definition name if used
    ModelID     string    `json:"model_id"`
    Harness     string    `json:"harness,omitempty"`
    State       string    `json:"state"`       // hatching, flying, idle, stopped, archived, error
    StartTime   time.Time `json:"start_time"`
    EndTime     *time.Time `json:"end_time,omitempty"`
    ExitReason  string    `json:"exit_reason,omitempty"`  // "user-stop", "force-stop", "completed", "error"
    LogPath     string    `json:"log_path"`
    WorkDir     string    `json:"work_dir"`
}

// Store manages run records on disk
type Store struct {
    Dir string // ~/.owl/runs/
}

func (s *Store) Save(rec *RunRecord) error      // write/update <run_id>.json
func (s *Store) Load(runID string) (*RunRecord, error)
func (s *Store) List() ([]RunRecord, error)      // scan all .json files
func (s *Store) Archive(runID string) error       // set state=archived, set end_time
func (s *Store) Delete(runID string) error        // hard delete .json file
```

### Implementation Steps

```
1. Create internal/runs/ package with store.go

2. Update ipc.Service for RunID-based operations:

   a. Change Kill → Stop:
      - New method: Service.StopAgent(args *StopArgs, reply *StopReply)
      - StopArgs: { RunID string, Force bool }
      - Graceful stop (default): close inbox channel, let engine clean up, set state="stopped"
      - Force stop: immediately kill, set state="stopped", exit_reason="force-stop"
      - Do NOT remove from Agents slice — keep for inspection
      - Persist RunRecord to ~/.owl/runs/<run_id>.json

   b. New method: Service.RemoveAgent(args *RemoveArgs, reply *RemoveReply)
      - RemoveArgs: { RunID string, Archive bool }
      - If Archive (default=true): set state="archived" in RunRecord, remove from active Agents slice
      - If !Archive: hard delete RunRecord + log files (require --force confirmation)

   c. New method: Service.InspectAgent(args *InspectArgs, reply *InspectReply)
      - InspectArgs: { RunID string }
      - InspectReply: full AgentState + RunRecord + recent log entries
      - Works for both active and archived runs

   d. Update ListAgents to include RunID in AgentState
      - Add RunID field to AgentState struct
      - Also add a ListRuns RPC that includes archived runs

3. Update engine.go:
   - On Run() start: create RunRecord with state="hatching", save to store
   - On state transitions: update RunRecord
   - On completion/stop: set end_time, save final state
   - Support cancellation context: pass ctx to Run(), cancel on Stop

4. Create cmd/owl/runs.go:

   a. owl runs list
      - Call Daemon.ListRuns RPC (active + archived)
      - Print table: RUN_ID | AGENT | STATE | STARTED | DURATION | MODEL
      - Flags: --all (include archived), --state <state> (filter), --json

   b. owl runs stop <run-id> [--force]
      - Call Daemon.StopAgent RPC
      - Default: graceful stop
      - --force: prompt for confirmation in interactive mode, then force stop
      - Print confirmation: "Stopped: reviewer-01JRQX5ABC"

   c. owl runs remove <run-id> [--archive]
      - Default behavior: archive (--archive is default, --no-archive for hard delete)
      - Hard delete requires --force flag
      - Print confirmation

   d. owl runs inspect <run-id>
      - Call Daemon.InspectAgent RPC
      - Print: agent metadata, current state, recent logs, task ledger summary
      - Flags: --json, --logs (show full logs)

5. Register: rootCmd.AddCommand(runsCmd)

6. Update TUI controls (tui.go):
   - Add keyboard shortcuts on agent tabs:
     's' — stop selected agent (graceful)
     'S' (shift+s) — force stop (with confirmation dialog)
     'x' — remove/archive
     'i' — inspect (show detailed view in main pane)
   - Add confirmation dialog component for force stop and hard delete
   - Show archived agents in sidebar with dimmed/grey styling
   - Add replay summary: when inspecting a stopped run, show key events timeline

7. Safety semantics:
   - Graceful stop: close inbox, send SIGTERM to harness subprocess, allow 5s cleanup
   - Force stop: immediate kill, SIGKILL to harness
   - Remove defaults to archive (state="archived", kept on disk)
   - Hard delete requires explicit --no-archive + confirmation
   - All stop/remove actions logged to an audit trail file:
     ~/.owl/audit.jsonl with entries: { ts, action, run_id, user, force }

8. Backwards compatibility:
   - Keep Kill RPC working but deprecated (calls Stop + Remove internally)
   - TUI 'kill' console command maps to stop + remove (archive)
```

### Tests

- Unit test: `StopAgent` graceful — inbox closed, state transitions to stopped
- Unit test: `StopAgent` force — immediate termination
- Unit test: `RemoveAgent` archive — removed from active list, persisted to disk
- Unit test: `RemoveAgent` hard delete — files removed
- Unit test: `InspectAgent` works for both active and archived runs
- Unit test: `RunStore.List` returns all records sorted by start time
- Integration test: `owl runs list` shows active and stopped runs
- Integration test: `owl runs stop` + `owl runs inspect` shows stopped state

---

## Phase 6: Metrics Capture + Prompt Improvement Loop

**RFC sections:** 10 (Metrics/Continuous Improvement), 14 (Observability Requirements)

### Goal
Capture per-run metrics, persist them, expose via CLI and to the meta-agent for prompt improvement recommendations.

### New Package: `internal/metrics/`

#### File: `internal/metrics/collector.go`

```go
// RunMetrics captures telemetry for a single run
type RunMetrics struct {
    RunID          string        `json:"run_id"`
    AgentName      string        `json:"agent_name"`
    AgentVersion   string        `json:"agent_version,omitempty"`
    PromptHash     string        `json:"prompt_hash"`       // SHA-256 of resolved system prompt
    Model          string        `json:"model"`
    Adapter        string        `json:"adapter"`           // provider name
    Workspace      string        `json:"workspace"`
    StartTS        time.Time     `json:"start_ts"`
    EndTS          *time.Time    `json:"end_ts,omitempty"`
    DurationMS     int64         `json:"duration_ms,omitempty"`
    Status         string        `json:"status"`            // running, completed, failed, stopped
    RetryCount     int           `json:"retry_count"`
    BlockedCount   int           `json:"blocked_count"`     // times agent was blocked/stalled
    HandoffCount   int           `json:"handoff_count"`     // viche_send calls
    TokenInput     int           `json:"token_input"`
    TokenOutput    int           `json:"token_output"`
    EstimatedCost  float64       `json:"estimated_cost,omitempty"`
    ToolCallCount  int           `json:"tool_call_count"`
    ToolFailCount  int           `json:"tool_fail_count"`
    TasksCreated   int           `json:"tasks_created"`
    TasksCompleted int           `json:"tasks_completed"`
}

// Collector accumulates metrics during a run
type Collector struct {
    metrics RunMetrics
    mu      sync.Mutex
}

func NewCollector(runID, agentName, model, adapter, workspace string) *Collector
func (c *Collector) RecordToolCall(name string, success bool)
func (c *Collector) RecordTokenUsage(input, output int)
func (c *Collector) RecordRetry()
func (c *Collector) RecordBlocked()
func (c *Collector) RecordHandoff()
func (c *Collector) RecordTaskCreated()
func (c *Collector) RecordTaskCompleted()
func (c *Collector) Finalize(status string) *RunMetrics
func (c *Collector) Snapshot() RunMetrics  // current state without finalizing
```

#### File: `internal/metrics/store.go`

```go
// Store persists metrics to disk
type Store struct {
    Dir string // ~/.owl/metrics/
}

func (s *Store) Save(m *RunMetrics) error
func (s *Store) Load(runID string) (*RunMetrics, error)
func (s *Store) List(opts ListOpts) ([]RunMetrics, error)
func (s *Store) Aggregate(agentName string) (*AgentMetricsSummary, error)

type ListOpts struct {
    AgentName string
    Since     time.Time
    Status    string
    Limit     int
}

type AgentMetricsSummary struct {
    AgentName       string
    TotalRuns       int
    SuccessRate     float64
    AvgDurationMS   int64
    AvgTokensIn     int
    AvgTokensOut    int
    TotalCost       float64
    TopFailureModes []FailureMode
    PromptVersions  []PromptVersionStats
}

type FailureMode struct {
    Pattern    string
    Count      int
    LastSeen   time.Time
}

type PromptVersionStats struct {
    PromptHash  string
    RunCount    int
    SuccessRate float64
}
```

### Implementation Steps

```
1. Create internal/metrics/ package

2. Integrate Collector into engine.go:
   - In Run(), create Collector at start
   - After each tool execution: c.RecordToolCall(name, success)
   - On usage events from stream: c.RecordTokenUsage(in, out)
   - On task creation: c.RecordTaskCreated()
   - On task completion: c.RecordTaskCompleted()
   - On viche_send: c.RecordHandoff()
   - On run end: c.Finalize(status), save via store

3. Compute prompt hash:
   - After BuildPromptStack().Render(), SHA-256 hash the result
   - Store as metrics.PromptHash
   - Enables correlating success/failure with specific prompt versions

4. Cost estimation:
   - Add a simple cost table in internal/metrics/cost.go:
     per-model pricing (input/output per 1M tokens)
   - Compute estimated cost from token counts
   - Note: approximate, clearly labeled as estimate

5. Create cmd/owl/metrics.go:

   a. owl metrics show <agent|run-id>
      - If argument looks like a run ID: show single run metrics
      - If argument is an agent name: show aggregate summary
      - Print: duration, tokens, cost, success rate, tool usage breakdown
      - Flags: --json, --since <time>

   b. owl recommend --agent <name>
      - Calls meta-agent with structured prompt:
        "Analyze metrics for agent <name> and suggest improvements"
      - Meta-agent uses query_logs and query_metrics tools
      - Returns: top failure modes, proposed edits ranked by expected impact
      - Default: display suggestions, do not auto-apply

6. Register: rootCmd.AddCommand(metricsCmd)

7. Add meta-agent tools for metrics:
   - query_metrics(agent_name, since) — returns metrics summary
   - compare_versions(agent_name) — compares metrics across prompt versions
   - These enable the meta-agent to:
     a. "What changed?" analysis: diff metrics between prompt versions
     b. Top failure modes: aggregate error patterns
     c. Proposed edits: ranked by expected impact based on failure frequency

8. Per-agent metrics.md (generated):
   - After each run, update <scope>/agents/<name>/metrics.md with:
     * Last run summary
     * Aggregate stats
     * Top issues
   - This file is human-readable and git-trackable

9. Observability requirements (RFC section 14):
   - Run timeline: derive from structured logs (Phase 4)
   - State transitions: log each setState() call with timestamp
   - Stop/remove audit trail: from Phase 5 audit.jsonl
   - Prompt version: prompt_hash in metrics
   - Linked logs: run_id connects metrics to logs
```

### Storage Layout

```
~/.owl/metrics/
  <run-id>.json              # Per-run metrics
~/.owl/agents/<name>/
  metrics.md                 # Human-readable aggregate (auto-generated)
```

### Tests

- Unit test: Collector correctly accumulates tool calls, tokens, retries
- Unit test: Finalize computes duration and sets status
- Unit test: Store.Aggregate computes correct success rate and averages
- Unit test: Cost estimation produces reasonable values for known models
- Unit test: PromptHash is deterministic for same input
- Integration test: After running an agent, `owl metrics show <run-id>` displays metrics

---

## Phase 7: Import/Export/Promote/Demote Identity Flows

**RFC sections:** 7.1 (Scope Rules), 7.2 (Import Flows), 15 (Command Surface)

### Goal
Add `owl agents` command group with full lifecycle management: list, show, validate, import, export, promote, demote, diff.

### File: `cmd/owl/agents_cmd.go` (new file)

### Implementation Steps

```
1. Create cmd/owl/agents_cmd.go with agentsCmdGroup:

   a. owl agents list [--scope project|global|all]
      - Use agents.Resolver.List(scopeFilter)
      - Print table: NAME | SCOPE | VERSION | MODEL | DESCRIPTION
      - Default: --scope all
      - Flags: --json

   b. owl agents show <agent>
      - Use agents.Resolver.Resolve(name, "")
      - Print full agent definition: AGENTS.md content, agent.yaml fields, source path
      - Flags: --scope, --json

   c. owl agents validate <agent> [--strict]
      - Use agents.Resolver.Validate(def, strict)
      - Print validation results: OK or list of errors/warnings
      - --strict: requires agent.yaml with all fields populated
      - Exit code 1 if validation fails (useful for CI)

   d. owl agents import --path <dir|file> [--scope project|global]
      - If path is a directory: expect AGENTS.md inside it, copy entire directory
      - If path is a single file: treat as AGENTS.md, create agent directory
      - Target: <scope>/agents/<name>/ (name derived from AGENTS.md heading or filename)
      - Validate structure after import, auto-suggest fixes:
        * Missing agent.yaml → generate skeleton from AGENTS.md content
        * Missing name → prompt user
      - Flags: --name <override_name>

   e. owl agents export --agent <name> --out <path>
      - Copy agent directory to specified output path
      - Include all files: AGENTS.md, agent.yaml, role.md, guardrails.md
      - Exclude generated files: metrics.md, CHANGELOG.md (unless --include-generated)
      - Create a portable bundle that can be imported elsewhere

   f. owl agents promote --agent <name> --to global
      - Copy agent definition from project scope to global scope
      - If agent already exists in global: error (use --force to overwrite)
      - Validate after promotion
      - Print: "Promoted reviewer from project to global scope"

   g. owl agents demote --agent <name> --to project
      - Copy agent definition from global scope to project scope
      - Same override/validation semantics as promote
      - Print: "Demoted reviewer from global to project scope"

   h. owl agents diff <agent> --from <v1> --to <v2>
      - Compare two versions of an agent's AGENTS.md
      - Versions identified by: git history, CHANGELOG.md entries, or explicit version tags
      - If git available: use git log on the agent's AGENTS.md file
      - Output: unified diff with color highlighting
      - Flags: --json (structured diff)

2. Register commands:
   agentsCmdGroup.AddCommand(listCmd, showCmd, validateCmd, importCmd, exportCmd, promoteCmd, demoteCmd, diffCmd)
   rootCmd.AddCommand(agentsCmdGroup)

3. Import validation logic (in internal/agents/import.go):
   - ValidateImportPath(path):
     a. Check path exists
     b. If directory: check AGENTS.md exists inside
     c. If file: check it's valid markdown
   - SuggestFixes(def):
     a. Missing agent.yaml → generate from parsed AGENTS.md headings
     b. Missing capabilities → suggest based on AGENTS.md content
     c. Missing version → default to "1.0.0"
   - AutoGenerateYAML(agentsMD):
     a. Parse markdown headings for name, role, capabilities
     b. Generate minimal agent.yaml

4. Promote/Demote implementation:
   - Copy directory contents
   - Update agent.yaml scope field if present
   - Validate at destination
   - Do not delete source (promote/demote is copy, not move)
   - User can delete source manually if desired

5. Also wire up: owl explain <agent> (from Phase 2)
   - This was designed in Phase 2 but register it in the agents group:
     owl agents explain <name> — alias for owl explain <agent>
```

### Full Command Surface Verification (RFC Section 15)

Cross-reference all proposed commands against implementation:

```
## 15.1 Agents
owl agents list [--scope ...]           → Phase 7, agents_cmd.go
owl agents show <agent>                 → Phase 7, agents_cmd.go
owl agents validate <agent> [--strict]  → Phase 7, agents_cmd.go
owl agents import ...                   → Phase 7, agents_cmd.go
owl agents export ...                   → Phase 7, agents_cmd.go
owl agents promote ...                  → Phase 7, agents_cmd.go
owl agents demote ...                   → Phase 7, agents_cmd.go
owl agents diff ...                     → Phase 7, agents_cmd.go
owl explain <agent>                     → Phase 2, explain.go

## 15.2 Hatch / Runs
owl hatch --agent <name> [...]          → Phase 1, main.go
owl runs list                           → Phase 5, runs.go
owl runs inspect <run-id>               → Phase 5, runs.go
owl runs stop <run-id> [--force]        → Phase 5, runs.go
owl runs remove <run-id> [--archive]    → Phase 5, runs.go

## 15.3 Logs / Metrics
owl logs list                           → Phase 4, logs.go
owl logs show <run-id>                  → Phase 4, logs.go
owl logs tail ...                       → Phase 4, logs.go
owl logs query ... --json               → Phase 4, logs.go
owl metrics show <agent|run-id>         → Phase 6, metrics.go
owl recommend --agent <name>            → Phase 6, metrics.go

## Migration
owl migrate templates                   → Phase 1, migrate.go
owl migrate check                       → Phase 1, migrate.go
```

### Tests

- Unit test: `owl agents list` returns agents from both scopes with correct labels
- Unit test: `owl agents import` from directory creates correct structure
- Unit test: `owl agents import` from single file creates agent directory with generated agent.yaml
- Unit test: `owl agents promote` copies to global scope correctly
- Unit test: `owl agents validate --strict` fails for missing agent.yaml fields
- Unit test: `owl agents export` excludes generated files by default
- Integration test: Full round-trip: create → export → import in different scope → validate

---

## Cross-Cutting Concerns

### Dependency Additions

| Dependency | Phase | Purpose |
|---|---|---|
| `gopkg.in/yaml.v3` | 2 | Parse agent.yaml |
| `github.com/oklog/ulid/v2` | 4 | Generate RunIDs (or use stdlib crypto/rand) |
| `github.com/fsnotify/fsnotify` | 4 | `logs tail --follow` (optional, can poll) |
| `crypto/sha256` | 6 | Prompt hash (stdlib, no dep) |

### IPC API Summary of New Methods

| Method | Phase | Purpose |
|---|---|---|
| `Daemon.DryRunHatch` | 1 | Resolve and validate without executing |
| `Daemon.StopAgent` | 5 | Graceful/force stop by RunID |
| `Daemon.RemoveAgent` | 5 | Archive/delete by RunID |
| `Daemon.InspectAgent` | 5 | Detailed agent + run info |
| `Daemon.ListRuns` | 5 | Active + archived runs |

### New Packages

| Package | Phase | Purpose |
|---|---|---|
| `internal/agents/` | 2 | Agent definition, resolution, validation |
| `internal/logs/` | 4 | Structured logging, reader, query |
| `internal/runs/` | 5 | Run state persistence |
| `internal/metrics/` | 6 | Metrics collection and aggregation |

### New CLI Files

| File | Phase | Commands |
|---|---|---|
| `cmd/owl/migrate.go` | 1 | `owl migrate templates`, `owl migrate check` |
| `cmd/owl/explain.go` | 2 | `owl explain <agent>` |
| `cmd/owl/logs.go` | 4 | `owl logs list/show/tail/query` |
| `cmd/owl/runs.go` | 5 | `owl runs list/stop/remove/inspect` |
| `cmd/owl/metrics.go` | 6 | `owl metrics show`, `owl recommend` |
| `cmd/owl/agents_cmd.go` | 7 | `owl agents list/show/validate/import/export/promote/demote/diff` |

### Phase Dependencies

```
Phase 1 (CLI contract)
  └─→ Phase 2 (Agent definitions) — requires --agent flag
       └─→ Phase 3 (Meta-agent) — requires agent resolver + definitions
       └─→ Phase 7 (Import/export) — requires agent resolver
  
Phase 4 (Logs CLI) — independent, can run in parallel with Phase 2
  └─→ Phase 6 (Metrics) — builds on structured logging

Phase 5 (Run controls) — independent, can run in parallel with Phase 2
  └─→ Phase 6 (Metrics) — needs RunID from run store

Phase 6 (Metrics) — requires Phase 4 + Phase 5
  └─→ Phase 3 (Meta-agent, full) — meta-agent log/metrics consumption needs Phase 6
```

**Recommended execution order for maximum parallelism:**
1. Phase 1 (CLI contract) — foundation, do first
2. Phase 2 + Phase 4 + Phase 5 — in parallel
3. Phase 3 (meta-agent scaffold) — after Phase 2
4. Phase 6 (metrics) — after Phase 4 + 5
5. Phase 7 (import/export) — after Phase 2
6. Phase 3 (meta-agent full) — after Phase 6

---

## Open Questions (from RFC) — Recommended Answers

1. **Should `agent.yaml` be required for marketplace/imported agents?**
   Recommendation: Required for imported agents, optional for local-only. Import validates and auto-generates if missing.

2. **Minimum approval model for meta-agent patch application?**
   Recommendation: Always require explicit "approve" message in v1. Display diff, wait for confirmation.

3. **Should prompt versions be semantic or monotonic?**
   Recommendation: Semantic (`v1.2.0`) in agent.yaml for human clarity. Prompt hash (SHA-256) for machine correlation. Both stored.

4. **Redaction defaults?**
   Recommendation: Redact patterns matching `sk-*`, `key-*`, `token-*`, `password`, `secret` by default. Show `[REDACTED]` with original length hint. `--no-redact` flag available.

5. **Multiple environment profiles?**
   Recommendation: Defer to future RFC. Current scope is project/global. Environment profiles add complexity without clear immediate need.
