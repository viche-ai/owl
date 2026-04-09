package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/viche-ai/owl/internal/logs"
	"github.com/viche-ai/owl/internal/runs"
)

type AgentState struct {
	ID        string
	RunID     string // unique run ID, set at hatch time, used for log file naming
	Name      string
	Role      string
	Ctx       string
	State     string
	Logs      string
	VicheID   string
	Registry  string
	ModelID   string
	Harness   string
	Thinking  bool
	Effort    string
	Verbosity string
}

type Service struct {
	Mu     sync.Mutex
	Agents []*AgentState

	// Channel-based message routing: agentIndex -> channel of inbound messages
	InboxChans map[int]chan InboundMessage

	// CloneArgs stores the original hatch parameters for each agent (used for cloning)
	CloneArgs map[int]*CloneArgs

	// RunStore persists run metadata across daemon restarts
	RunStore *runs.Store

	// CancelFuncs allows force-stopping an in-progress LLM call by RunID
	CancelFuncs map[string]context.CancelFunc
}

type InboundMessage struct {
	From    string // "user" or a viche agent ID
	Content string
}

func NewService() *Service {
	store, _ := runs.NewStore() // best-effort; nil store is handled gracefully
	return &Service{
		Agents:      []*AgentState{},
		InboxChans:  make(map[int]chan InboundMessage),
		CloneArgs:   make(map[int]*CloneArgs),
		RunStore:    store,
		CancelFuncs: make(map[string]context.CancelFunc),
	}
}

type HatchArgs struct {
	Description string
	Registry    string
	ModelID     string
	Agent       string // replaces Template
	Template    string // deprecated: use Agent
	Scope       string // "project", "global", or "" for auto-resolve
	DryRun      bool
	Thinking    bool
	Effort      string
	Name        string
	Ambient     bool
	WorkDir     string
	Harness     string
	HarnessArgs string
	NoNetInject bool
	MetaAgent   bool // internal: use meta-agent system prompt and tools instead of scaffolding
}

type HatchReply struct {
	Success bool
	Message string
}

type DryRunReply struct {
	ResolvedAgent string // name of the resolved agent definition
	Scope         string // "project", "global", or "legacy"
	SourcePath    string // absolute path to agent definition
	PromptStack   string // assembled prompt layers (human-readable summary)
	ModelID       string
	Valid         bool
	Errors        []string
}

type CloneArgs struct {
	Description string
	Registry    string
	ModelID     string
	Thinking    bool
	Effort      string
	Name        string
	Ambient     bool
	WorkDir     string
	Harness     string
	HarnessArgs string
	NoNetInject bool
}

type CloneRequest struct {
	AgentIndex int
}

type CloneResponse struct {
	Success bool
	Message string
	NewID   string
}

// RunEngineHook is set by owld to wire the engine into the RPC service.
// It receives a cancellable context so force-stop can abort in-flight LLM calls.
var RunEngineHook func(ctx context.Context, state *AgentState, mu func(func()), args *HatchArgs, inbox chan InboundMessage)

// DryRunHatch resolves the agent definition and prompt stack without spawning an agent.
func (s *Service) DryRunHatch(args *HatchArgs, reply *DryRunReply) error {
	agentName := args.Agent
	if agentName == "" {
		agentName = args.Template
	}

	reply.ResolvedAgent = agentName
	reply.ModelID = args.ModelID

	if agentName == "" {
		reply.Valid = true
		reply.Scope = "none"
		reply.PromptStack = fmt.Sprintf("Description: %s\nModel: %s", args.Description, args.ModelID)
		return nil
	}

	// Resolution order: project > global > legacy templates
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	type candidate struct {
		scope string
		path  string
	}

	var candidates []candidate

	if args.Scope == "" || args.Scope == "project" {
		candidates = append(candidates, candidate{"project", filepath.Join(cwd, ".owl", "agents", agentName)})
	}
	if args.Scope == "" || args.Scope == "global" {
		candidates = append(candidates, candidate{"global", filepath.Join(home, ".owl", "agents", agentName)})
	}
	if args.Scope == "" {
		candidates = append(candidates, candidate{"legacy", filepath.Join(home, ".owl", "templates", agentName+".json")})
	}

	for _, c := range candidates {
		var found bool
		var promptLines []string

		if c.scope == "legacy" {
			if b, err := os.ReadFile(c.path); err == nil {
				found = true
				reply.Scope = "legacy"
				reply.SourcePath = c.path
				promptLines = append(promptLines, fmt.Sprintf("[legacy template] %s", c.path))
				promptLines = append(promptLines, string(b))
			}
		} else {
			agentsMD := filepath.Join(c.path, "AGENTS.md")
			if _, err := os.Stat(agentsMD); err == nil {
				found = true
				reply.Scope = c.scope
				reply.SourcePath = c.path
				if content, err := os.ReadFile(agentsMD); err == nil {
					promptLines = append(promptLines, fmt.Sprintf("[%s] AGENTS.md:", c.scope))
					promptLines = append(promptLines, string(content))
				}
				roleMD := filepath.Join(c.path, "role.md")
				if content, err := os.ReadFile(roleMD); err == nil {
					promptLines = append(promptLines, "[role.md]:")
					promptLines = append(promptLines, string(content))
				}
				guardrailsMD := filepath.Join(c.path, "guardrails.md")
				if content, err := os.ReadFile(guardrailsMD); err == nil {
					promptLines = append(promptLines, "[guardrails.md]:")
					promptLines = append(promptLines, string(content))
				}
			}
		}

		if found {
			reply.PromptStack = strings.Join(promptLines, "\n")
			reply.Valid = true
			return nil
		}
	}

	reply.Valid = false
	reply.Errors = append(reply.Errors, fmt.Sprintf("agent definition %q not found (searched project, global, and legacy scopes)", agentName))
	return nil
}

func (s *Service) Hatch(args *HatchArgs, reply *HatchReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	fmt.Println("=> [Daemon] Received hatch request:", args.Description)

	idx := len(s.Agents)

	name := args.Name
	if name == "" {
		name = args.Description
	}

	runID := logs.GenerateRunID(name)

	newAgent := &AgentState{
		ID:        fmt.Sprintf("%d", idx+1),
		RunID:     runID,
		Name:      name,
		Role:      "auto",
		Ctx:       "0 / 128k",
		State:     "hatching",
		Logs:      "",
		ModelID:   args.ModelID,
		Harness:   args.Harness,
		Thinking:  args.Thinking,
		Effort:    args.Effort,
		Verbosity: "verbose", // default verbosity
	}

	s.Agents = append(s.Agents, newAgent)

	// Create an inbox channel for this agent
	inbox := make(chan InboundMessage, 64)
	s.InboxChans[idx] = inbox

	// Store clone args for later use by CloneAgent
	s.CloneArgs[idx] = &CloneArgs{
		Description: args.Description,
		Registry:    args.Registry,
		ModelID:     args.ModelID,
		Thinking:    args.Thinking,
		Effort:      args.Effort,
		Name:        args.Name,
		Ambient:     args.Ambient,
		WorkDir:     args.WorkDir,
		Harness:     args.Harness,
		HarnessArgs: args.HarnessArgs,
		NoNetInject: args.NoNetInject,
	}

	// Persist initial RunRecord
	if s.RunStore != nil {
		workDir := args.WorkDir
		if workDir == "" {
			workDir, _ = os.Getwd()
		}
		rec := &runs.RunRecord{
			RunID:     runID,
			AgentName: name,
			AgentDef:  args.Agent,
			ModelID:   args.ModelID,
			Harness:   args.Harness,
			State:     "hatching",
			StartTime: time.Now(),
			WorkDir:   workDir,
		}
		_ = s.RunStore.Save(rec) // best-effort
	}

	if RunEngineHook != nil {
		ctx, cancel := context.WithCancel(context.Background())
		s.CancelFuncs[runID] = cancel
		go RunEngineHook(ctx, newAgent, func(f func()) {
			s.Mu.Lock()
			defer s.Mu.Unlock()
			f()
		}, args, inbox)
	}

	reply.Success = true
	reply.Message = "Egg hatched for: " + args.Description
	return nil
}

func (s *Service) CloneAgent(req *CloneRequest, res *CloneResponse) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if req.AgentIndex < 0 || req.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index %d out of range", req.AgentIndex)
	}

	cloneArgs, ok := s.CloneArgs[req.AgentIndex]
	if !ok {
		return fmt.Errorf("no clone args found for agent index %d", req.AgentIndex)
	}

	idx := len(s.Agents)

	cloneName := cloneArgs.Name
	if cloneName == "" {
		cloneName = cloneArgs.Description
	}
	if cloneName == "" {
		cloneName = s.Agents[req.AgentIndex].Name + "-clone"
	} else {
		cloneName = cloneName + "-clone"
	}

	cloneRunID := logs.GenerateRunID(cloneName)

	newAgent := &AgentState{
		ID:        fmt.Sprintf("%d", idx+1),
		RunID:     cloneRunID,
		Name:      cloneName,
		Role:      "auto",
		Ctx:       "0 / 128k",
		State:     "hatching",
		Logs:      "",
		ModelID:   cloneArgs.ModelID,
		Harness:   cloneArgs.Harness,
		Thinking:  cloneArgs.Thinking,
		Effort:    cloneArgs.Effort,
		Verbosity: "verbose",
	}

	s.Agents = append(s.Agents, newAgent)

	inbox := make(chan InboundMessage, 64)
	s.InboxChans[idx] = inbox

	s.CloneArgs[idx] = &CloneArgs{
		Description: cloneArgs.Description,
		Registry:    cloneArgs.Registry,
		ModelID:     cloneArgs.ModelID,
		Thinking:    cloneArgs.Thinking,
		Effort:      cloneArgs.Effort,
		Name:        cloneName,
		Ambient:     cloneArgs.Ambient,
		WorkDir:     cloneArgs.WorkDir,
		Harness:     cloneArgs.Harness,
		HarnessArgs: cloneArgs.HarnessArgs,
		NoNetInject: cloneArgs.NoNetInject,
	}

	// Persist initial RunRecord for clone
	if s.RunStore != nil {
		workDir := cloneArgs.WorkDir
		if workDir == "" {
			workDir, _ = os.Getwd()
		}
		rec := &runs.RunRecord{
			RunID:     cloneRunID,
			AgentName: cloneName,
			AgentDef:  "",
			ModelID:   cloneArgs.ModelID,
			Harness:   cloneArgs.Harness,
			State:     "hatching",
			StartTime: time.Now(),
			WorkDir:   workDir,
		}
		_ = s.RunStore.Save(rec) // best-effort
	}

	if RunEngineHook != nil {
		hatchArgs := &HatchArgs{
			Description: cloneArgs.Description,
			Registry:    cloneArgs.Registry,
			ModelID:     cloneArgs.ModelID,
			Thinking:    cloneArgs.Thinking,
			Effort:      cloneArgs.Effort,
			Name:        cloneName,
			Ambient:     cloneArgs.Ambient,
			WorkDir:     cloneArgs.WorkDir,
			Harness:     cloneArgs.Harness,
			HarnessArgs: cloneArgs.HarnessArgs,
			NoNetInject: cloneArgs.NoNetInject,
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.CancelFuncs[cloneRunID] = cancel
		go RunEngineHook(ctx, newAgent, func(f func()) {
			s.Mu.Lock()
			defer s.Mu.Unlock()
			f()
		}, hatchArgs, inbox)
	}

	res.Success = true
	res.Message = "Cloned agent: " + s.Agents[req.AgentIndex].Name
	res.NewID = newAgent.ID
	return nil
}

// SendMessage delivers a user message to an agent's inbox
type SendMessageArgs struct {
	AgentIndex int
	Content    string
}

type SendMessageReply struct {
	Success bool
}

func (s *Service) SendMessage(args *SendMessageArgs, reply *SendMessageReply) error {
	s.Mu.Lock()
	ch, ok := s.InboxChans[args.AgentIndex]
	s.Mu.Unlock()

	if !ok {
		return fmt.Errorf("no inbox for agent index %d", args.AgentIndex)
	}

	ch <- InboundMessage{From: "user", Content: args.Content}
	reply.Success = true
	return nil
}

type SetModelArgs struct {
	AgentIndex int
	ModelID    string
}

type SetModelReply struct {
	Success bool
	Message string
}

func (s *Service) SetAgentModel(args *SetModelArgs, reply *SetModelReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if args.AgentIndex < 0 || args.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index out of range")
	}

	s.Agents[args.AgentIndex].ModelID = args.ModelID
	reply.Success = true
	reply.Message = fmt.Sprintf("✓ Model set to %s", args.ModelID)
	s.Agents[args.AgentIndex].Logs += fmt.Sprintf("\n> %s\n", reply.Message)
	return nil
}

type SetConfigArgs struct {
	AgentIndex int
	Thinking   *bool
	Effort     *string
	Verbosity  *string
}

type SetConfigReply struct {
	Success bool
	Message string
}

func (s *Service) SetAgentConfig(args *SetConfigArgs, reply *SetConfigReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if args.AgentIndex < 0 || args.AgentIndex >= len(s.Agents) {
		return fmt.Errorf("agent index out of range")
	}

	agent := s.Agents[args.AgentIndex]
	if args.Thinking != nil {
		agent.Thinking = *args.Thinking
	}
	if args.Effort != nil {
		agent.Effort = *args.Effort
	}
	if args.Verbosity != nil {
		agent.Verbosity = *args.Verbosity
	}

	reply.Success = true
	reply.Message = "✓ Configuration updated"
	agent.Logs += fmt.Sprintf("\n> %s\n", reply.Message)
	return nil
}

type ListArgs struct{}

type ListReply struct {
	Agents []AgentState
}

type ExternalStreamEvent struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	State     string `json:"state"`
	LogLine   string `json:"log_line"`
}

type StreamExternalReply struct {
	Success bool
	Message string
}

func (s *Service) StreamExternalAgent(event *ExternalStreamEvent, reply *StreamExternalReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	var agent *AgentState

	for _, ag := range s.Agents {
		if ag.ID == event.AgentID {
			agent = ag
			break
		}
	}

	if agent == nil {
		agent = &AgentState{
			ID:        event.AgentID,
			Name:      event.AgentName,
			Role:      "external",
			State:     "flying",
			ModelID:   "external",
			Harness:   "external",
			Verbosity: "verbose",
		}
		s.Agents = append(s.Agents, agent)
		s.InboxChans[len(s.Agents)-1] = make(chan InboundMessage, 64)
	}

	if event.State != "" {
		agent.State = event.State
	}

	if event.LogLine != "" {
		agent.Logs += event.LogLine + "\n"
	}

	reply.Success = true
	reply.Message = "Event received"
	return nil
}

func (s *Service) ListAgents(args *ListArgs, reply *ListReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	reply.Agents = make([]AgentState, len(s.Agents))
	for i, ag := range s.Agents {
		reply.Agents[i] = *ag
	}
	return nil
}

// ── Run Controls ─────────────────────────────────────────────────────────────

// StopArgs identifies a run to stop by RunID. Force=true does an immediate kill.
type StopArgs struct {
	RunID string
	Force bool
}

type StopReply struct {
	Success bool
	Message string
}

// StopAgent gracefully (or forcefully) stops a running agent without removing it.
// The agent remains in the Agents slice for inspection; its state becomes "stopped".
func (s *Service) StopAgent(args *StopArgs, reply *StopReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	idx := s.indexByRunID(args.RunID)
	if idx < 0 {
		return fmt.Errorf("run %q not found in active agents", args.RunID)
	}

	agent := s.Agents[idx]
	name := agent.Name

	exitReason := "user-stop"
	if args.Force {
		exitReason = "force-stop"
	}

	// Cancel the in-flight LLM context (force-stop or graceful — both benefit)
	if cancel, ok := s.CancelFuncs[args.RunID]; ok {
		cancel()
		delete(s.CancelFuncs, args.RunID)
	}

	// Close inbox channel — this exits the agent's conversation loop
	if ch, ok := s.InboxChans[idx]; ok {
		close(ch)
		delete(s.InboxChans, idx)
	}

	agent.State = "stopped"

	// Update persisted RunRecord
	if s.RunStore != nil {
		if rec, err := s.RunStore.Load(args.RunID); err == nil {
			rec.State = "stopped"
			rec.ExitReason = exitReason
			now := time.Now()
			rec.EndTime = &now
			_ = s.RunStore.Save(rec)
		}
	}

	s.appendAudit(args.RunID, "stop", args.Force)

	reply.Success = true
	reply.Message = fmt.Sprintf("✓ Stopped: %s (%s)", name, args.RunID)
	return nil
}

// RemoveArgs identifies a run to remove by RunID.
// Archive=true (default) keeps the record on disk with state="archived".
// Archive=false + Force=true hard-deletes the RunRecord and log files.
type RemoveArgs struct {
	RunID   string
	Archive bool // true = archive; false + Force = hard delete
	Force   bool // required for hard delete
}

type RemoveReply struct {
	Success bool
	Message string
}

// RemoveAgent removes an agent from the active slice and optionally archives or deletes its record.
func (s *Service) RemoveAgent(args *RemoveArgs, reply *RemoveReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	idx := s.indexByRunID(args.RunID)
	if idx >= 0 {
		// Still active — stop it first (graceful)
		if cancel, ok := s.CancelFuncs[args.RunID]; ok {
			cancel()
			delete(s.CancelFuncs, args.RunID)
		}
		if ch, ok := s.InboxChans[idx]; ok {
			close(ch)
			delete(s.InboxChans, idx)
		}
		// Remove from active slice
		s.Agents = append(s.Agents[:idx], s.Agents[idx+1:]...)
		s.rebuildMaps(idx)
	}

	if s.RunStore != nil {
		if args.Archive || !args.Force {
			// Default: archive
			_ = s.RunStore.Archive(args.RunID)
		} else {
			// Hard delete — requires explicit Force + !Archive
			_ = s.RunStore.Delete(args.RunID)
		}
	}

	action := "archive"
	if !args.Archive && args.Force {
		action = "delete"
	}
	s.appendAudit(args.RunID, action, args.Force)

	reply.Success = true
	reply.Message = fmt.Sprintf("✓ Removed: %s", args.RunID)
	return nil
}

// InspectArgs identifies a run to inspect by RunID.
type InspectArgs struct {
	RunID    string
	FullLogs bool // if true, include last 100 log lines
}

type InspectReply struct {
	Found      bool
	RunRecord  runs.RunRecord
	AgentState *AgentState // non-nil if the agent is still active in memory
	RecentLogs string      // recent log entries from the JSONL file
}

// InspectAgent returns full metadata and recent logs for a run (active or archived).
func (s *Service) InspectAgent(args *InspectArgs, reply *InspectReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Check active agents first
	idx := s.indexByRunID(args.RunID)
	if idx >= 0 {
		ag := s.Agents[idx]
		reply.Found = true
		reply.AgentState = ag

		if s.RunStore != nil {
			if rec, err := s.RunStore.Load(args.RunID); err == nil {
				reply.RunRecord = *rec
			} else {
				// Synthesise a record from in-memory state
				reply.RunRecord = runs.RunRecord{
					RunID:     ag.RunID,
					AgentName: ag.Name,
					ModelID:   ag.ModelID,
					Harness:   ag.Harness,
					State:     ag.State,
					StartTime: time.Now(),
				}
			}
		}
	} else if s.RunStore != nil {
		// Try archived / stopped records
		if rec, err := s.RunStore.Load(args.RunID); err == nil {
			reply.Found = true
			reply.RunRecord = *rec
		}
	}

	if !reply.Found {
		return fmt.Errorf("run %q not found", args.RunID)
	}

	// Read recent log entries from disk
	reply.RecentLogs = s.readRecentLogs(args.RunID, args.FullLogs)
	return nil
}

// ListRunsArgs controls what the ListRuns RPC returns.
type ListRunsArgs struct {
	All         bool   // include archived runs (default: active only)
	StateFilter string // filter by state ("" = no filter)
}

type ListRunsReply struct {
	Records []runs.RunRecord
}

// ListRuns returns run records. Active agents are always included; archived runs
// are included when All=true or when AllRuns flag is set.
func (s *Service) ListRuns(args *ListRunsArgs, reply *ListRunsReply) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Build active run IDs set
	activeRunIDs := make(map[string]bool)
	for _, ag := range s.Agents {
		activeRunIDs[ag.RunID] = true
	}

	// Synthesise records for active agents that may not be persisted yet
	var activeRecords []runs.RunRecord
	for _, ag := range s.Agents {
		var rec runs.RunRecord
		if s.RunStore != nil {
			if r, err := s.RunStore.Load(ag.RunID); err == nil {
				r.State = ag.State // reflect in-memory state
				rec = *r
			} else {
				rec = runs.RunRecord{
					RunID:     ag.RunID,
					AgentName: ag.Name,
					ModelID:   ag.ModelID,
					Harness:   ag.Harness,
					State:     ag.State,
					StartTime: time.Now(),
				}
			}
		}
		if args.StateFilter == "" || rec.State == args.StateFilter {
			activeRecords = append(activeRecords, rec)
		}
	}

	if !args.All {
		reply.Records = activeRecords
		return nil
	}

	// Include archived / stopped records from disk
	var allRecords []runs.RunRecord
	if s.RunStore != nil {
		if stored, err := s.RunStore.List(); err == nil {
			for _, r := range stored {
				if activeRunIDs[r.RunID] {
					continue // already added from in-memory state above
				}
				if args.StateFilter == "" || r.State == args.StateFilter {
					allRecords = append(allRecords, r)
				}
			}
		}
	}
	reply.Records = append(activeRecords, allRecords...)
	return nil
}

// ── Deprecated: Kill ─────────────────────────────────────────────────────────

type KillArgs struct {
	AgentIndex int
}

type KillReply struct {
	Success bool
	Message string
}

// Kill is deprecated — it calls StopAgent + RemoveAgent internally.
// Kept for backwards compatibility with older TUI and CLI consumers.
func (s *Service) Kill(args *KillArgs, reply *KillReply) error {
	s.Mu.Lock()
	if args.AgentIndex < 0 || args.AgentIndex >= len(s.Agents) {
		s.Mu.Unlock()
		return fmt.Errorf("agent index %d out of range", args.AgentIndex)
	}
	runID := s.Agents[args.AgentIndex].RunID
	name := s.Agents[args.AgentIndex].Name
	s.Mu.Unlock()

	// Stop (graceful)
	var stopReply StopReply
	_ = s.StopAgent(&StopArgs{RunID: runID, Force: false}, &stopReply)

	// Remove (archive by default)
	var removeReply RemoveReply
	_ = s.RemoveAgent(&RemoveArgs{RunID: runID, Archive: true}, &removeReply)

	reply.Success = true
	reply.Message = "✓ Killed: " + name
	return nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// indexByRunID returns the index in s.Agents for the given RunID, or -1.
// Caller must hold s.Mu.
func (s *Service) indexByRunID(runID string) int {
	for i, ag := range s.Agents {
		if ag.RunID == runID {
			return i
		}
	}
	return -1
}

// rebuildMaps corrects InboxChans and CloneArgs indices after removing index removedIdx.
// Caller must hold s.Mu.
func (s *Service) rebuildMaps(removedIdx int) {
	newInbox := make(map[int]chan InboundMessage)
	for i, ch := range s.InboxChans {
		if i > removedIdx {
			newInbox[i-1] = ch
		} else {
			newInbox[i] = ch
		}
	}
	s.InboxChans = newInbox

	newClone := make(map[int]*CloneArgs)
	for i, ca := range s.CloneArgs {
		if i > removedIdx {
			newClone[i-1] = ca
		} else {
			newClone[i] = ca
		}
	}
	s.CloneArgs = newClone
}

// appendAudit writes a single-line audit entry to ~/.owl/audit.jsonl.
func (s *Service) appendAudit(runID, action string, force bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	entry := map[string]interface{}{
		"ts":     time.Now().UTC().Format(time.RFC3339),
		"action": action,
		"run_id": runID,
		"force":  force,
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	auditPath := filepath.Join(home, ".owl", "audit.jsonl")
	f, err := os.OpenFile(auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s\n", b)
}

// readRecentLogs returns recent log lines from the agent's JSONL log file.
func (s *Service) readRecentLogs(runID string, full bool) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	logPath := filepath.Join(home, ".owl", "logs", runID+".jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	limit := 20
	if full {
		limit = 100
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	var out strings.Builder
	for _, line := range lines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			ts, _ := entry["ts"].(string)
			level, _ := entry["level"].(string)
			msg, _ := entry["message"].(string)
			_, _ = fmt.Fprintf(&out, "[%s] %s: %s\n", ts, level, msg)
		} else {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}
