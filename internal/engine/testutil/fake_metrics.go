package testutil

import (
	"sync"
	"time"

	"github.com/viche-ai/owl/internal/metrics"
)

type FakeMetricsRecorder struct {
	mu sync.Mutex

	Result         *metrics.RunMetrics
	Finalized      bool
	FinalStatus    string
	ActiveStarts   int
	ActiveStops    int
	ToolCalls      []FakeToolCall
	TokenInputs    int
	TokenOutputs   int
	Handoffs       int
	TasksCreated   int
	TasksCompleted int
	PromptHash     string
	AgentVersion   string
}

type FakeToolCall struct {
	Name    string
	Success bool
}

func NewFakeMetricsRecorder(runID, agentName, modelID, adapter, workspace string) *FakeMetricsRecorder {
	return &FakeMetricsRecorder{
		Result: &metrics.RunMetrics{
			RunID:     runID,
			AgentName: agentName,
			Model:     modelID,
			Adapter:   adapter,
			Workspace: workspace,
			StartTS:   time.Unix(0, 0).UTC(),
			Status:    "running",
		},
	}
}

func (f *FakeMetricsRecorder) SetPromptHash(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.PromptHash = hash
	f.Result.PromptHash = hash
}

func (f *FakeMetricsRecorder) SetAgentVersion(version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.AgentVersion = version
	f.Result.AgentVersion = version
}

func (f *FakeMetricsRecorder) StartActive() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ActiveStarts++
}

func (f *FakeMetricsRecorder) StopActive() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ActiveStops++
}

func (f *FakeMetricsRecorder) RecordToolCall(name string, success bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ToolCalls = append(f.ToolCalls, FakeToolCall{Name: name, Success: success})
	f.Result.ToolCallCount++
	if !success {
		f.Result.ToolFailCount++
	}
}

func (f *FakeMetricsRecorder) RecordTokenUsage(input, output int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.TokenInputs += input
	f.TokenOutputs += output
	f.Result.TokenInput += input
	f.Result.TokenOutput += output
}

func (f *FakeMetricsRecorder) RecordHandoff() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Handoffs++
	f.Result.HandoffCount++
}

func (f *FakeMetricsRecorder) RecordTaskCreated() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.TasksCreated++
	f.Result.TasksCreated++
}

func (f *FakeMetricsRecorder) RecordTaskCompleted() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.TasksCompleted++
	f.Result.TasksCompleted++
}

func (f *FakeMetricsRecorder) Snapshot() metrics.RunMetrics {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.Result
}

func (f *FakeMetricsRecorder) Finalize(status string) *metrics.RunMetrics {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Finalized = true
	f.FinalStatus = status
	copy := *f.Result
	copy.Status = status
	now := time.Unix(1, 0).UTC()
	copy.EndTS = &now
	f.Result = &copy
	return &copy
}
