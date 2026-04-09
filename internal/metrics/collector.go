package metrics

import (
	"sync"
	"time"
)

// RunMetrics captures telemetry for a single run.
type RunMetrics struct {
	RunID          string     `json:"run_id"`
	AgentName      string     `json:"agent_name"`
	AgentVersion   string     `json:"agent_version,omitempty"`
	PromptHash     string     `json:"prompt_hash"`
	Model          string     `json:"model"`
	Adapter        string     `json:"adapter"`
	Workspace      string     `json:"workspace"`
	StartTS        time.Time  `json:"start_ts"`
	EndTS          *time.Time `json:"end_ts,omitempty"`
	DurationMS     int64      `json:"duration_ms,omitempty"`
	Status         string     `json:"status"` // running, completed, failed, stopped
	RetryCount     int        `json:"retry_count"`
	BlockedCount   int        `json:"blocked_count"`
	HandoffCount   int        `json:"handoff_count"`
	TokenInput     int        `json:"token_input"`
	TokenOutput    int        `json:"token_output"`
	EstimatedCost  float64    `json:"estimated_cost,omitempty"`
	ToolCallCount  int        `json:"tool_call_count"`
	ToolFailCount  int        `json:"tool_fail_count"`
	TasksCreated   int        `json:"tasks_created"`
	TasksCompleted int        `json:"tasks_completed"`
}

// Collector accumulates metrics during a run.
type Collector struct {
	mu      sync.Mutex
	metrics RunMetrics
}

// NewCollector initialises a Collector for the given run.
func NewCollector(runID, agentName, model, adapter, workspace string) *Collector {
	return &Collector{
		metrics: RunMetrics{
			RunID:     runID,
			AgentName: agentName,
			Model:     model,
			Adapter:   adapter,
			Workspace: workspace,
			StartTS:   time.Now(),
			Status:    "running",
		},
	}
}

// SetPromptHash stores the SHA-256 hex digest of the resolved system prompt.
func (c *Collector) SetPromptHash(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.PromptHash = hash
}

// SetAgentVersion records the agent definition version (from agent.yaml).
func (c *Collector) SetAgentVersion(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.AgentVersion = version
}

// RecordToolCall increments call counts; success=false counts as a tool failure.
func (c *Collector) RecordToolCall(name string, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.ToolCallCount++
	if !success {
		c.metrics.ToolFailCount++
	}
}

// RecordTokenUsage adds token counts from a single LLM response.
func (c *Collector) RecordTokenUsage(input, output int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.TokenInput += input
	c.metrics.TokenOutput += output
}

// RecordRetry records that the engine retried an LLM call.
func (c *Collector) RecordRetry() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.RetryCount++
}

// RecordBlocked records that the agent was blocked or stalled.
func (c *Collector) RecordBlocked() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.BlockedCount++
}

// RecordHandoff records a viche_send tool call (agent-to-agent handoff).
func (c *Collector) RecordHandoff() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.HandoffCount++
}

// RecordTaskCreated records a new task added to the task ledger.
func (c *Collector) RecordTaskCreated() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.TasksCreated++
}

// RecordTaskCompleted records a task marked completed in the task ledger.
func (c *Collector) RecordTaskCompleted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics.TasksCompleted++
}

// Snapshot returns the current metrics state without finalising.
func (c *Collector) Snapshot() RunMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	snap := c.metrics
	return snap
}

// Finalize closes the run, computes duration, cost, and returns the final metrics.
func (c *Collector) Finalize(status string) *RunMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	c.metrics.EndTS = &now
	c.metrics.DurationMS = now.Sub(c.metrics.StartTS).Milliseconds()
	c.metrics.Status = status
	c.metrics.EstimatedCost = EstimateCost(c.metrics.Model, c.metrics.TokenInput, c.metrics.TokenOutput)
	copy := c.metrics
	return &copy
}
