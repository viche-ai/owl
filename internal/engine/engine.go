package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viche-ai/owl/internal/agents"
	"github.com/viche-ai/owl/internal/config"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/llm"
	"github.com/viche-ai/owl/internal/logs"
	"github.com/viche-ai/owl/internal/metrics"
	"github.com/viche-ai/owl/internal/runs"
	"github.com/viche-ai/owl/internal/tools"
	"github.com/viche-ai/owl/internal/viche"
)

type AgentEngine struct {
	State       *ipc.AgentState
	Cfg         *config.Config
	Mu          func(func())
	Router      *llm.Router
	RunStore    *runs.Store    // optional; persists state transitions to disk
	MetricStore *metrics.Store // optional; persists run metrics to disk

	logWriter   *logs.Writer
	collector   *metrics.Collector
	runCtx      context.Context // cancellable context for the current run
	messages    []llm.Message
	provider    llm.Provider
	model       string
	vicheTools  *tools.VicheTools
	systemTools *tools.SystemTools
	taskTools   *tools.TaskTools
	metaTools   *tools.MetaTools
	taskLedger  *TaskLedger
	toolDefs    []llm.ToolDef
}

func (e *AgentEngine) Run(ctx context.Context, args *ipc.HatchArgs, inbox chan ipc.InboundMessage) {
	e.runCtx = ctx

	// Open structured log file
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".owl", "logs")

	runID := e.State.RunID
	if runID == "" {
		runID = logs.GenerateRunID(sanitizeName(args.Description))
	}

	var logPath string
	if w, path, err := logs.NewWriter(logDir, runID, e.State.Name, e.State.ID, args.ModelID); err == nil {
		e.logWriter = w
		logPath = path
		defer func() { _ = e.logWriter.Close() }()
	}

	// Update persisted RunRecord with resolved log path
	if e.RunStore != nil {
		if rec, err := e.RunStore.Load(runID); err == nil {
			rec.LogPath = logPath
			rec.State = "hatching"
			_ = e.RunStore.Save(rec)
		}
	}

	// Initialise metrics collector for this run
	adapterName := ""
	if parts := strings.SplitN(args.ModelID, "/", 2); len(parts) == 2 {
		adapterName = parts[0]
	}
	workspaceForMetrics := args.WorkDir
	if workspaceForMetrics == "" {
		workspaceForMetrics, _ = os.Getwd()
	}
	e.collector = metrics.NewCollector(runID, e.State.Name, args.ModelID, adapterName, workspaceForMetrics)

	e.appendLog("> Booting agent engine...\n")
	e.appendLog(fmt.Sprintf("> Log: %s\n", logPath))
	time.Sleep(200 * time.Millisecond)

	if args.Harness != "" {
		e.runHarness(ctx, args, inbox)
		return
	}

	// ── Step 1: Resolve LLM ──
	modelID := args.ModelID
	if modelID == "" {
		modelID = e.Cfg.Models.Default
	}
	if modelID == "" {
		for name, pCfg := range e.Cfg.Models.Providers {
			switch name {
			case "anthropic":
				modelID = "anthropic/claude-sonnet-4-6"
			case "google":
				modelID = "google/gemini-2.5-pro"
			default:
				baseURL := pCfg.BaseURL
				if baseURL == "" {
					baseURL = "https://api.openai.com/v1"
				}
				models, err := llm.DiscoverModels(baseURL, pCfg.APIKey)
				if err == nil && len(models) > 0 {
					modelID = name + "/" + models[0]
				} else {
					modelID = name + "/gpt-4o"
				}
			}
			break
		}
	}

	if modelID == "" {
		e.appendLog("[Error] No AI providers configured.\n")
		e.setState("idle")
		return
	}

	provider, model, err := e.Router.Resolve(modelID)
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] %v\n", err))
		e.setState("idle")
		return
	}
	e.provider = provider
	e.model = model

	e.Mu(func() {
		if e.State.ModelID == "" {
			e.State.ModelID = modelID
		}
	})

	e.appendLog(fmt.Sprintf("> Model: %s/%s\n\n", provider.Name(), model))

	// ── Meta-agent fast path ──
	// Skip scaffolding, Viche, and task ledger. Use fixed identity + meta tools.
	if args.MetaAgent {
		e.runMetaAgent(args, inbox)
		return
	}

	// ── Step 2: Resolve agent definition (before scaffolding) ──
	workDir := args.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	var agentDef *agents.AgentDefinition
	if args.Agent != "" {
		agentResolver := agents.NewResolver(workDir)
		e.appendLog(fmt.Sprintf("> Resolving agent definition %q (scope=%q, workdir=%s)\n", args.Agent, args.Scope, workDir))
		if def, err := agentResolver.Resolve(args.Agent, args.Scope); err == nil {
			agentDef = def
			e.appendLog(fmt.Sprintf("> Loaded agent definition %q from %s scope (%s)\n", def.Name, def.Scope, def.SourcePath))
			if def.AgentsMD != "" {
				e.appendLog(fmt.Sprintf("> AGENTS.md loaded (%d bytes)\n", len(def.AgentsMD)))
			}
			if def.RoleMD != "" {
				e.appendLog(fmt.Sprintf("> role.md loaded (%d bytes)\n", len(def.RoleMD)))
			}
			if def.GuardrailsMD != "" {
				e.appendLog(fmt.Sprintf("> guardrails.md loaded (%d bytes)\n", len(def.GuardrailsMD)))
			}
		} else {
			e.appendLog(fmt.Sprintf("[Warning] Could not load agent definition %q: %v\n", args.Agent, err))
		}
	}

	// ── Step 3: Scaffolding (Hatching) ──
	e.setState("hatching")

	type ScaffoldResult struct {
		Name         string   `json:"name"`
		Capabilities []string `json:"capabilities"`
		Plan         any      `json:"plan"`
	}

	var identity ScaffoldResult
	var planText string

	if agentDef != nil {
		// Agent definition provides identity — skip LLM scaffolding
		e.appendLog(fmt.Sprintf("> Using agent definition %q for identity (skipping LLM scaffold)\n", agentDef.Name))
		identity.Name = agentDef.Name
		identity.Capabilities = agentDef.Capabilities
		if agentDef.Description != "" {
			planText = agentDef.Description
		} else {
			planText = "Agent identity loaded from definition"
		}
	} else {
		// No agent definition — scaffold identity via LLM
		e.appendLog("[Thinking]\nScaffolding agent identity...\n")

		scaffoldPrompt := fmt.Sprintf(`You are an AI agent being initialized.
Your purpose based on user request: %s

You will be connected to the Viche agent network. You need an identity.
Please output a JSON block with the following structure:
{
  "name": "a short, descriptive, hyphenated name (e.g., code-reviewer, search-bot)",
  "capabilities": ["capability1", "capability2"], // list of 1-4 string tags describing your skills. Other agents will use these to discover you on Viche. Example: ["poem-writer", "code-review", "web-search"]
  "plan": "a brief 3-5 bullet point plan of what you will do"
}
Output ONLY valid JSON.`, args.Description)

		e.messages = []llm.Message{
			{Role: llm.RoleSystem, Content: scaffoldPrompt},
			{Role: llm.RoleUser, Content: "Initialize. What is your plan?"},
		}

		scaffoldResponse := e.runWithTools()
		if scaffoldResponse == "" {
			return
		}

		// simple json extraction if wrapped in code blocks
		jsonStr := scaffoldResponse
		if start := strings.Index(jsonStr, "{"); start != -1 {
			if end := strings.LastIndex(jsonStr, "}"); end != -1 {
				jsonStr = jsonStr[start : end+1]
			}
		}

		if err := json.Unmarshal([]byte(jsonStr), &identity); err != nil {
			e.appendLog(fmt.Sprintf("[Warning] Failed to parse scaffolding JSON: %v\n", err))
			identity.Name = sanitizeName(args.Description)
			if len(identity.Name) == 0 {
				identity.Name = "owl-agent"
			}
			identity.Capabilities = []string{"owl-agent"}
			planText = scaffoldResponse
		} else {
			if pArr, ok := identity.Plan.([]interface{}); ok {
				var lines []string
				for _, item := range pArr {
					if s, ok := item.(string); ok {
						lines = append(lines, "- "+s)
					}
				}
				planText = strings.Join(lines, "\n")
			} else if pStr, ok := identity.Plan.(string); ok {
				planText = pStr
			} else {
				planText = fmt.Sprintf("%v", identity.Plan)
			}
		}
	}

	if len(identity.Capabilities) == 0 {
		identity.Capabilities = []string{"owl-agent"}
	} else {
		hasOwl := false
		for _, cap := range identity.Capabilities {
			if cap == "owl-agent" {
				hasOwl = true
				break
			}
		}
		if !hasOwl {
			identity.Capabilities = append(identity.Capabilities, "owl-agent")
		}
	}

	if args.Name != "" {
		identity.Name = args.Name
	}

	e.Mu(func() {
		e.State.Name = identity.Name
		e.State.Role = strings.Join(identity.Capabilities, ", ")
	})

	e.appendLog(fmt.Sprintf("\n> Identity configured:\n  Name: %s\n  Capabilities: %v\n\nPlan:\n%s\n\n",
		identity.Name, identity.Capabilities, planText))

	// ── Step 3: Viche Registration ──
	e.appendLog("> Connecting to Viche network...\n")

	vicheURL, vicheToken := e.Cfg.GetActiveRegistry()
	if args.Registry != "" {
		vicheToken = args.Registry
	}

	vc := viche.NewClient(vicheURL, vicheToken)

	if vc.IsAuthenticated() {
		e.appendLog(fmt.Sprintf("> Registry: %s\n", vc.RegistryLabel()))
	} else {
		e.appendLog("> Registry: public (no account required)\n")
	}

	agentID, err := vc.Register(identity.Name, identity.Capabilities)
	if err != nil {
		e.appendLog(fmt.Sprintf("[Warning] Viche registration failed: %v\n", err))
		e.appendLog("> Continuing without network presence...\n\n")
	} else {
		e.Mu(func() {
			e.State.VicheID = agentID
			e.State.Registry = vc.RegistryLabel()
		})
		e.appendLog(fmt.Sprintf("> Viche ID: %s\n", agentID))

		ch := viche.NewChannel(vc.BaseURL(), agentID, vc.Token())

		ch.OnMessage = func(msg viche.InboxMessage) {
			if msg.Type == "result" {
				e.appendLog(fmt.Sprintf("\n> [Viche result from %s] %s\n", msg.From, msg.Body))
				return
			}
			inbox <- ipc.InboundMessage{
				From:    msg.From,
				Content: fmt.Sprintf("[Message from agent %s]: %s", msg.From, msg.Body),
			}
		}

		if err := ch.Connect(); err != nil {
			e.appendLog(fmt.Sprintf("[Warning] WebSocket failed: %v\n", err))
		} else {
			e.appendLog("> Connected via WebSocket.\n")

			e.vicheTools = &tools.VicheTools{Channel: ch, AgentID: agentID}
			for _, def := range e.vicheTools.Definitions() {
				e.toolDefs = append(e.toolDefs, llm.ToolDef{
					Name:        def.Name,
					Description: def.Description,
					Parameters:  def.Parameters,
				})
			}
			e.appendLog(fmt.Sprintf("> %d Viche tools available: viche_discover, viche_send\n", len(e.vicheTools.Definitions())))
		}
		e.appendLog("\n")
	}

	// ── Step 3b: System tools (shell, file read/write/edit) ──
	e.systemTools = &tools.SystemTools{WorkDir: workDir}
	for _, def := range e.systemTools.Definitions() {
		e.toolDefs = append(e.toolDefs, llm.ToolDef{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}
	e.appendLog(fmt.Sprintf("> %d system tools available: shell_exec, file_read, file_write, file_edit\n", len(e.systemTools.Definitions())))

	// ── Step 3c: Task ledger + tools ──
	e.taskLedger = NewTaskLedger(fmt.Sprintf("%s-%d", sanitizeName(args.Description), time.Now().Unix()))
	e.taskTools = &tools.TaskTools{Updater: e}
	for _, def := range e.taskTools.Definitions() {
		e.toolDefs = append(e.toolDefs, llm.ToolDef{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.Parameters,
		})
	}
	e.appendLog("> Task ledger initialized\n")

	// ── Step 4: Setup main system prompt ──
	projectCfg, _ := config.LoadProjectConfig(workDir)

	promptStack := agents.BuildPromptStack(agentDef, e.Cfg, projectCfg, "")

	var promptBuilder strings.Builder
	promptBuilder.WriteString(promptStack.Render())
	promptBuilder.WriteString("\n\n")

	fmt.Fprintf(&promptBuilder, `[RUNTIME]
You are an AI agent. Your identity: %s
Your purpose: %s
Your capabilities: %v

You are connected to the Viche agent network.%s
You have tools to discover and communicate with other agents:
- viche_discover: Find agents by capability (use '*' for all)
- viche_send: Send a message to another agent by their ID

When you need to interact with other agents, use these tools. Always use the tools rather than trying to simulate communication.

TASK MANAGEMENT:
Each inbound message is automatically registered in your task ledger. Before acting on any message:
1. Check the [TASK LEDGER] section — it shows all your tasks and their status
2. If a message relates to a task you already COMPLETED, acknowledge briefly but do NOT redo the work
3. If it is a new task, work on it and use task_update to mark it "started" then "completed" when done
4. Never repeat completed work, even if asked again

Your initial plan was:
%s
`,
		identity.Name,
		args.Description,
		strings.Join(identity.Capabilities, ", "),
		func() string {
			if e.State.VicheID != "" {
				return fmt.Sprintf(" Your Viche ID is %s.", e.State.VicheID)
			}
			return ""
		}(),
		planText)

	systemPrompt := promptBuilder.String()

	// Record prompt hash for metrics correlation
	if e.collector != nil {
		hash := sha256.Sum256([]byte(systemPrompt))
		e.collector.SetPromptHash(fmt.Sprintf("%x", hash))
		if agentDef != nil {
			e.collector.SetAgentVersion(agentDef.Version)
		}
	}

	e.messages = []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
	}

	e.appendLog("> Hatch complete.\n")
	e.setState("flying")

	// ── Step 5: Kickoff — send the original description as the first task ──
	if !args.Ambient {
		e.appendLog("> Starting work on task...\n")
		e.processMessage(args.Description)
	} else {
		e.appendLog("> Ambient mode active. Waiting for messages...\n")
	}
	e.setState("idle")

	// ── Step 6: Conversation loop ──
	e.appendLog("> Listening for messages...\n")
	for msg := range inbox {
		e.appendLog(fmt.Sprintf("\n> [%s] %s\n", msg.From, msg.Content))
		e.processMessage(msg.Content)
	}

	// ── Cleanup ──
	if e.State.VicheID != "" && e.vicheTools != nil && e.vicheTools.Channel != nil {
		e.vicheTools.Channel.Close()
	}
	e.appendLog("\n> Agent stopped.\n")
	e.setState("stopped")

	// Finalise metrics and persist
	if e.collector != nil {
		status := "completed"
		if ctx.Err() != nil {
			status = "stopped"
		}
		m := e.collector.Finalize(status)
		if e.MetricStore != nil {
			_ = e.MetricStore.Save(m) // best-effort
		}
		// Write per-agent metrics.md if we know the agent name
		if args.Agent != "" {
			writeAgentMetricsMD(args.Agent, args.Scope, m)
		}
	}

	// Persist final state
	if e.RunStore != nil {
		exitReason := "completed"
		if ctx.Err() != nil {
			exitReason = "force-stop"
		}
		if rec, err := e.RunStore.Load(runID); err == nil {
			rec.State = "stopped"
			rec.ExitReason = exitReason
			now := time.Now()
			rec.EndTime = &now
			_ = e.RunStore.Save(rec)
		}
	}
}

// UpdateTaskStatus implements tools.TaskUpdater
func (e *AgentEngine) UpdateTaskStatus(id string, status string) string {
	e.taskLedger.UpdateTask(id, TaskStatus(status))
	return fmt.Sprintf("Task %s updated to %s", id, status)
}

// ListTasks implements tools.TaskUpdater
func (e *AgentEngine) ListTasks() string {
	return e.taskLedger.ContextSummary()
}

func (e *AgentEngine) processMessage(content string) {
	if e.collector != nil {
		e.collector.StartActive()
		defer e.collector.StopActive()
	}
	e.setState("flying")
	e.appendLog("\n[Thinking]\n")

	var msgContent string
	if e.taskLedger != nil {
		// Register inbound message as a new task
		source := "user"
		if strings.HasPrefix(content, "[Message from agent") {
			parts := strings.SplitN(content, "]", 2)
			if len(parts) > 0 {
				source = strings.TrimPrefix(parts[0], "[Message from agent ")
			}
		}
		task := e.taskLedger.AddTask(truncate(content, 200), source)
		if e.collector != nil {
			e.collector.RecordTaskCreated()
		}
		msgContent = fmt.Sprintf("[TASK LEDGER]\n%s\n[END TASK LEDGER]\n\n[NEW MESSAGE (task %s)]\n%s",
			e.taskLedger.ContextSummary(), task.ID, content)
	} else {
		msgContent = content
	}

	e.messages = append(e.messages, llm.Message{Role: llm.RoleUser, Content: msgContent})

	response := e.runWithTools()
	if response != "" {
		e.messages = append(e.messages, llm.Message{Role: llm.RoleAssistant, Content: response})
	} else {
		// If response is completely empty, the LLM failed to generate anything (likely API refusal or empty stream)
		e.appendLog("\n[Error] LLM returned an empty response.\n")
	}

	e.setState("idle")
}

// runWithTools streams LLM output, executing any tool calls in a loop until the model is done
func (e *AgentEngine) runWithTools() string {
	for {
		ctx := e.runCtx
		if ctx == nil {
			ctx = context.Background()
		}
		var stream <-chan llm.StreamEvent
		var err error

		// Ensure we are using the dynamically set model ID from state, if any.
		var currentModelID string
		e.Mu(func() { currentModelID = e.State.ModelID })
		if currentModelID != "" {
			if provider, model, errResolve := e.Router.Resolve(currentModelID); errResolve == nil {
				e.provider = provider
				e.model = model
			}
		}

		// (Thinking & Effort configs would be passed here if supported by the provider interface)
		// For now we assume provider uses default if not extended, but we would need provider updates to support it.

		e.setState("thinking") // drives TUI animation

		// Inject agent ID so the CLI providers can use it for session persistence
		ctx = context.WithValue(ctx, llm.SessionIDKey, e.State.ID)

		if len(e.toolDefs) > 0 {
			stream, err = e.provider.ChatStreamWithTools(ctx, e.model, e.messages, e.toolDefs)
		} else {
			stream, err = e.provider.ChatStream(ctx, e.model, e.messages)
		}

		if err != nil {
			if ctx.Err() != nil {
				e.appendLog("\n> Agent force-stopped.\n")
				return ""
			}
			e.appendLog(fmt.Sprintf("[Error] LLM failed: %v\n", err))
			e.setState("idle")
			return ""
		}

		e.setState("flying") // transition from thinking to streaming

		var textResponse strings.Builder
		var toolCalls []*llm.ToolCallEvent

		for event := range stream {
			if event.Error != nil {
				e.appendLog(fmt.Sprintf("\n[Error] %v\n", event.Error))
				e.setState("idle")
				return ""
			}
			if event.Delta != "" {
				textResponse.WriteString(event.Delta)
				e.appendLog(event.Delta)
			}
			if event.Reasoning != "" {
				e.appendLog(event.Reasoning)
			}
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, event.ToolCall)
			}
			if event.Usage != nil {
				e.Mu(func() {
					e.State.Ctx = fmt.Sprintf("%dk / 128k", (event.Usage.TotalTokens+500)/1000)
				})
				e.logWriter.LogUsage(event.Usage.PromptTokens, event.Usage.CompletionTokens, e.model)
				if e.collector != nil {
					e.collector.RecordTokenUsage(event.Usage.PromptTokens, event.Usage.CompletionTokens)
				}
			}
			if event.Done {
				break
			}
		}

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			return textResponse.String()
		}

		// Build the assistant message WITH tool_use references
		var toolRefs []llm.ToolCallRef
		for _, tc := range toolCalls {
			toolRefs = append(toolRefs, llm.ToolCallRef{
				ID:               tc.ID,
				Name:             tc.Name,
				Arguments:        tc.Arguments,
				ThoughtSignature: tc.ThoughtSignature,
			})
		}
		e.messages = append(e.messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   textResponse.String(),
			ToolCalls: toolRefs,
		})

		// Execute each tool and append results
		for _, tc := range toolCalls {
			e.logVerbose(fmt.Sprintf("\n> [Tool: %s] %s\n", tc.Name, tc.Arguments))

			call, err := tools.ParseToolCallFromJSON(tc.Name, tc.Arguments)
			if err != nil {
				e.logVerbose(fmt.Sprintf("[Error] Bad tool args: %v\n", err))
				continue
			}

			var result string
			switch call.Name {
			case "viche_discover", "viche_send":
				if e.vicheTools != nil {
					result = e.vicheTools.Execute(call)
				} else {
					result = "Error: Viche tools not available (not connected to network)"
				}
			case "shell_exec", "file_read", "file_write", "file_edit":
				result = e.systemTools.Execute(call)
			case "task_update":
				result = e.taskTools.Execute(call)
			case "list_agents", "create_agent", "validate_agent", "explain_agent",
				"suggest_edit", "apply_edit", "read_agent_file", "read_project_config",
				"query_logs", "query_metrics", "compare_versions", "list_models":
				if e.metaTools != nil {
					result = e.metaTools.Execute(call)
				} else {
					result = "Error: meta tools not available"
				}
			default:
				result = fmt.Sprintf("Unknown tool: %s", call.Name)
			}

			// Write a structured tool entry to the disk log.
			e.logWriter.LogTool(tc.Name, tc.Arguments, result)

			// If debug verbosity is enabled, print full result, else print truncated
			e.logDebug(fmt.Sprintf("> Result: %s\n", result))

			outcome := "Success"
			if strings.HasPrefix(result, "Error") || strings.HasPrefix(result, "Unknown tool:") {
				outcome = "Failed"
			}
			if e.collector != nil {
				success := outcome == "Success"
				e.collector.RecordToolCall(call.Name, success)
				if call.Name == "viche_send" {
					e.collector.RecordHandoff()
				}
				if call.Name == "task_update" {
					if statusVal, ok := call.Args["status"].(string); ok && statusVal == "completed" {
						e.collector.RecordTaskCompleted()
					}
				}
			}
			e.logVerbose(fmt.Sprintf("> Tool execution completed: %s\n", outcome))

			e.messages = append(e.messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}

		// Loop back to let the LLM respond to the tool results
		e.appendLog("\n[Thinking]\n")
	}
}

func sanitizeName(name string) string {
	s := strings.ReplaceAll(name, " ", "-")
	s = strings.ToLower(s)
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

// appendLog writes to both structured disk log and TUI state.
func (e *AgentEngine) appendLog(text string) {
	e.Mu(func() { e.State.Logs += text })
	e.logWriter.Log(inferLevel(text), text)
}

// logVerbose always writes to disk; only updates TUI state at verbose/debug verbosity.
func (e *AgentEngine) logVerbose(text string) {
	var v string
	e.Mu(func() { v = e.State.Verbosity })

	e.logWriter.Log("debug", text)

	if v == "verbose" || v == "debug" {
		e.Mu(func() { e.State.Logs += text })
	}
}

// logDebug always writes to disk; only updates TUI state at debug verbosity.
func (e *AgentEngine) logDebug(text string) {
	var v string
	e.Mu(func() { v = e.State.Verbosity })

	e.logWriter.Log("debug", text)

	if v == "debug" {
		e.Mu(func() { e.State.Logs += text })
	}
}

// inferLevel derives a structured log level from message content prefixes.
func inferLevel(text string) string {
	switch {
	case strings.Contains(text, "[Error]"):
		return "error"
	case strings.Contains(text, "[Warning]"):
		return "warn"
	case strings.Contains(text, "[Thinking]"):
		return "thinking"
	default:
		return "info"
	}
}

func (e *AgentEngine) setState(state string) {
	e.Mu(func() { e.State.State = state })
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// writeAgentMetricsMD writes a human-readable metrics summary to the agent's directory.
func writeAgentMetricsMD(agentName, scope string, m *metrics.RunMetrics) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	var agentDir string
	switch scope {
	case "global", "":
		agentDir = filepath.Join(home, ".owl", "agents", agentName)
	case "project":
		cwd, _ := os.Getwd()
		agentDir = filepath.Join(cwd, ".owl", "agents", agentName)
	default:
		agentDir = filepath.Join(home, ".owl", "agents", agentName)
	}

	if _, err := os.Stat(agentDir); err != nil {
		return // agent directory doesn't exist yet
	}

	dur := ""
	if m.DurationMS > 0 {
		dur = fmt.Sprintf("%.1fs", float64(m.DurationMS)/1000)
	}
	activeDur := ""
	if m.ActiveDurationMS > 0 {
		activeDur = fmt.Sprintf(" (active: %.1fs)", float64(m.ActiveDurationMS)/1000)
	}
	cost := ""
	if m.EstimatedCost > 0 {
		cost = fmt.Sprintf(" (~$%.4f)", m.EstimatedCost)
	}

	content := fmt.Sprintf(`# Metrics: %s

**Last run:** %s
**Run ID:** %s
**Status:** %s
**Model:** %s
**Duration:** %s%s%s

## Token Usage
- Input: %d
- Output: %d

## Tool Usage
- Total calls: %d
- Failed calls: %d

## Task Ledger
- Tasks created: %d
- Tasks completed: %d

## Handoffs
- Viche handoffs: %d
`,
		agentName,
		m.StartTS.Format("2006-01-02 15:04:05"),
		m.RunID,
		m.Status,
		m.Model,
		dur,
		activeDur,
		cost,
		m.TokenInput,
		m.TokenOutput,
		m.ToolCallCount,
		m.ToolFailCount,
		m.TasksCreated,
		m.TasksCompleted,
		m.HandoffCount,
	)

	_ = os.WriteFile(filepath.Join(agentDir, "metrics.md"), []byte(content), 0644)
}
