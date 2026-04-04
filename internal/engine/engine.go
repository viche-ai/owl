package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viche-ai/owl/internal/config"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/llm"
	"github.com/viche-ai/owl/internal/tools"
	"github.com/viche-ai/owl/internal/viche"
)

type AgentEngine struct {
	State  *ipc.AgentState
	Cfg    *config.Config
	Mu     func(func())
	Router *llm.Router

	logFile     *os.File
	messages    []llm.Message
	provider    llm.Provider
	model       string
	vicheTools  *tools.VicheTools
	systemTools *tools.SystemTools
	taskTools   *tools.TaskTools
	taskLedger  *TaskLedger
	toolDefs    []llm.ToolDef
}

func (e *AgentEngine) Run(args *ipc.HatchArgs, inbox chan ipc.InboundMessage) {
	// Open log file
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".owl", "logs")
	_ = os.MkdirAll(logDir, 0755)

	// Use a temporary name for the log file until we scaffold the real name
	tempName := sanitizeName(args.Description)
	logPath := filepath.Join(logDir, fmt.Sprintf("agent-%s-%d.log", tempName, time.Now().Unix()))
	if f, err := os.Create(logPath); err == nil {
		e.logFile = f
		defer func() { _ = f.Close() }()
	}

	e.appendLog("> Booting agent engine...\n")
	e.appendLog(fmt.Sprintf("> Log: %s\n", logPath))
	time.Sleep(200 * time.Millisecond)

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

	// ── Step 2: Scaffolding (Hatching) ──
	e.setState("hatching")
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

	// Attempt to parse JSON from scaffold response
	type ScaffoldResult struct {
		Name         string   `json:"name"`
		Capabilities []string `json:"capabilities"`
		Plan         any      `json:"plan"`
	}

	var identity ScaffoldResult

	// simple json extraction if wrapped in code blocks
	jsonStr := scaffoldResponse
	if start := strings.Index(jsonStr, "{"); start != -1 {
		if end := strings.LastIndex(jsonStr, "}"); end != -1 {
			jsonStr = jsonStr[start : end+1]
		}
	}

	var planText string
	if err := json.Unmarshal([]byte(jsonStr), &identity); err != nil {
		e.appendLog(fmt.Sprintf("[Warning] Failed to parse scaffolding JSON: %v\n", err))
		// Fallbacks
		identity.Name = sanitizeName(args.Description)
		if len(identity.Name) == 0 {
			identity.Name = "owl-agent"
		}
		identity.Capabilities = []string{"owl-agent"}
		planText = scaffoldResponse // just dump the text
	} else {
		// Plan could be an array of strings or a single string
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

	if len(identity.Capabilities) == 0 {
		identity.Capabilities = []string{"owl-agent"}
	} else {
		// Always ensure 'owl-agent' is present so they can be discovered generally
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

	// Rename if args.Name was explicitly provided
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
			// Do not process messages that are results/acknowledgements to prevent infinite loops
			if msg.Type == "result" {
				// Simply append them to the UI logs so the user sees it, but do NOT
				// feed it into the agent's LLM inbox to trigger a response
				e.appendLog(fmt.Sprintf("\n> [Viche result from %s] %s\n", msg.From, msg.Body))
				return
			}

			// Route incoming Viche messages into the agent's main inbox to ensure sequential processing
			inbox <- ipc.InboundMessage{
				From:    msg.From,
				Content: fmt.Sprintf("[Message from agent %s]: %s", msg.From, msg.Body),
			}
		}

		if err := ch.Connect(); err != nil {
			e.appendLog(fmt.Sprintf("[Warning] WebSocket failed: %v\n", err))
		} else {
			e.appendLog("> Connected via WebSocket.\n")

			// Set up Viche tools for this agent
			e.vicheTools = &tools.VicheTools{Channel: ch, AgentID: agentID}
			for _, def := range e.vicheTools.Definitions() {
				e.toolDefs = append(e.toolDefs, llm.ToolDef{
					Name:        def.Name,
					Description: def.Description,
					Parameters:  def.Parameters,
				})
			}
			e.appendLog(fmt.Sprintf("> %d Viche tools available: viche_discover, viche_send\n", len(e.toolDefs)))
		}
		e.appendLog("\n")
	}

	// ── Step 3b: System tools (shell, file read/write/edit) ──
	e.systemTools = &tools.SystemTools{WorkDir: home + "/dev/viche-ai/viche"}
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
	systemPrompt := fmt.Sprintf(`You are an AI agent. Your identity: %s
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
			if agentID != "" {
				return fmt.Sprintf(" Your Viche ID is %s.", agentID)
			}
			return ""
		}(),
		planText)

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
	if agentID != "" && e.vicheTools != nil && e.vicheTools.Channel != nil {
		e.vicheTools.Channel.Close()
	}
	e.appendLog("\n> Agent stopped.\n")
	e.setState("stopped")
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
	e.setState("flying")
	e.appendLog("\n[Thinking]\n")

	// Register inbound message as a new task
	source := "user"
	if strings.HasPrefix(content, "[Message from agent") {
		parts := strings.SplitN(content, "]", 2)
		if len(parts) > 0 {
			source = strings.TrimPrefix(parts[0], "[Message from agent ")
		}
	}
	task := e.taskLedger.AddTask(truncate(content, 200), source)

	// Inject task context into the message
	taskContext := fmt.Sprintf("[TASK LEDGER]\n%s\n[END TASK LEDGER]\n\n[NEW MESSAGE (task %s)]\n%s", e.taskLedger.ContextSummary(), task.ID, content)

	e.messages = append(e.messages, llm.Message{Role: llm.RoleUser, Content: taskContext})

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
		ctx := context.Background()
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
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, event.ToolCall)
			}
			if event.Usage != nil {
				e.Mu(func() {
					e.State.Ctx = fmt.Sprintf("%dk / 128k", (event.Usage.TotalTokens+500)/1000)
				})
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
			default:
				result = fmt.Sprintf("Unknown tool: %s", call.Name)
			}

			// If debug verbosity is enabled, print full result, else print truncated
			e.logDebug(fmt.Sprintf("> Result: %s\n", result))

			outcome := "Success"
			lowerResult := strings.ToLower(result)
			if strings.Contains(lowerResult, "error") || strings.Contains(lowerResult, "failed") || strings.Contains(lowerResult, "unknown tool") {
				outcome = "Failed"
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

// appendLog writes to both disk and TUI state
func (e *AgentEngine) appendLog(text string) {
	e.Mu(func() { e.State.Logs += text })
	if e.logFile != nil {
		_, _ = e.logFile.WriteString(text)
	}
}

// logVerbose logs based on verbosity level
func (e *AgentEngine) logVerbose(text string) {
	var v string
	e.Mu(func() { v = e.State.Verbosity })

	if e.logFile != nil {
		_, _ = e.logFile.WriteString(text)
	}

	if v == "verbose" || v == "debug" {
		e.Mu(func() { e.State.Logs += text })
	}
}

// logDebug logs based on verbosity level
func (e *AgentEngine) logDebug(text string) {
	var v string
	e.Mu(func() { v = e.State.Verbosity })

	if e.logFile != nil {
		_, _ = e.logFile.WriteString(text)
	}

	if v == "debug" {
		e.Mu(func() { e.State.Logs += text })
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
