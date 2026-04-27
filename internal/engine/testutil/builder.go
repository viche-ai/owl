package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/viche-ai/owl/internal/config"
	"github.com/viche-ai/owl/internal/engine"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/llm"
	"github.com/viche-ai/owl/internal/viche"
)

type EngineBuilder struct {
	args       *ipc.HatchArgs
	llmScript  [][]llm.StreamEvent
	vicheError error
}

type Fakes struct {
	LLM     *FakeLLM
	Viche   *FakeViche
	Logs    *FakeLogWriter
	Metrics *FakeMetricsRecorder
	Runs    *FakeRunStore
	WorkDir string
	HomeDir string
}

func NewEngineBuilder() *EngineBuilder {
	return &EngineBuilder{
		args: &ipc.HatchArgs{
			Description: "test task",
			ModelID:     "fake/test-model",
			WorkDir:     ".",
		},
	}
}

func (b *EngineBuilder) WithLLMScript(turns ...[]llm.StreamEvent) *EngineBuilder {
	b.llmScript = turns
	return b
}

func (b *EngineBuilder) WithArgs(args *ipc.HatchArgs) *EngineBuilder {
	b.args = args
	return b
}

func (b *EngineBuilder) WithVicheError(err error) *EngineBuilder {
	b.vicheError = err
	return b
}

func (b *EngineBuilder) Build() (*engine.AgentEngine, *Fakes) {
	baseDir, err := os.MkdirTemp("", "owl-engine-test-*")
	if err != nil {
		panic(fmt.Sprintf("make temp dir: %v", err))
	}

	workDir := filepath.Join(baseDir, "work")
	homeDir := filepath.Join(baseDir, "home")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		panic(fmt.Sprintf("make work dir: %v", err))
	}
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		panic(fmt.Sprintf("make home dir: %v", err))
	}

	args := *b.args
	if args.ModelID == "" {
		args.ModelID = "fake/test-model"
	}
	if args.WorkDir == "" || args.WorkDir == "." {
		args.WorkDir = workDir
	}

	llmFake := &FakeLLM{Script: append([][]llm.StreamEvent(nil), b.llmScript...)}
	vicheFake := &FakeViche{
		RegisterError: b.vicheError,
		Inbox:         make(chan viche.InboxMessage, 16),
		BaseURLValue:  "https://fake-viche.local",
	}
	logsFake := &FakeLogWriter{}
	metricsFake := NewFakeMetricsRecorder("run-test-001", "test-agent", args.ModelID, "fake", args.WorkDir)
	runStoreFake := NewFakeRunStore()

	fakes := &Fakes{
		LLM:     llmFake,
		Viche:   vicheFake,
		Logs:    logsFake,
		Metrics: metricsFake,
		Runs:    runStoreFake,
		WorkDir: workDir,
		HomeDir: homeDir,
	}

	resolver := &fakeResolver{
		provider: llmFake,
		model:    "test-model",
	}

	cfg := &config.Config{
		Models: config.ModelsConfig{
			Default: args.ModelID,
		},
	}

	state := &ipc.AgentState{
		ID:      "agent-test-001",
		RunID:   "run-test-001",
		Name:    "test-agent",
		State:   "idle",
		ModelID: args.ModelID,
	}

	var mu sync.Mutex
	eng := &engine.AgentEngine{
		State:    state,
		Cfg:      cfg,
		Router:   resolver,
		RunStore: runStoreFake,
		Clock:    func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		Sleep:    func(ctx context.Context, d time.Duration) error { return nil },
		HomeDir:  func() (string, error) { return homeDir, nil },
		Getwd:    func() (string, error) { return workDir, nil },
		NewLogWriter: func(logDir, runID, agentName, agentID, modelID string) (engine.LogWriter, string, error) {
			return logsFake, filepath.Join(logDir, runID+".jsonl"), nil
		},
		NewMetricsRecorder: func(runID, agentName, modelID, adapter, workspace string) engine.MetricsRecorder {
			metricsFake.Result.RunID = runID
			metricsFake.Result.AgentName = agentName
			metricsFake.Result.Model = modelID
			metricsFake.Result.Adapter = adapter
			metricsFake.Result.Workspace = workspace
			return metricsFake
		},
		NewVicheClient: func(baseURL, token string) engine.VicheClient {
			vicheFake.BaseURLValue = baseURL
			vicheFake.TokenValue = token
			return vicheFake
		},
		NewVicheChannel: func(baseURL, agentID, token string) engine.VicheChannel {
			vicheFake.BaseURLValue = baseURL
			vicheFake.AgentID = agentID
			vicheFake.TokenValue = token
			return vicheFake
		},
		NewTaskLedger: func(agentID string) engine.TaskLedgerStore {
			return engine.NewTaskLedgerAt(filepath.Join(homeDir, ".owl", "agents"), agentID)
		},
		Mu: func(f func()) {
			mu.Lock()
			defer mu.Unlock()
			f()
		},
	}

	return eng, fakes
}

type fakeResolver struct {
	provider engine.LLMProvider
	model    string
}

func (r *fakeResolver) Resolve(modelID string) (llm.Provider, string, error) {
	return r.provider, r.model, nil
}
