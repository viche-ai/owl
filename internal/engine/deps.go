package engine

import (
	"context"
	"os"
	"time"

	"github.com/viche-ai/owl/internal/llm"
	"github.com/viche-ai/owl/internal/logs"
	"github.com/viche-ai/owl/internal/metrics"
	"github.com/viche-ai/owl/internal/runs"
	"github.com/viche-ai/owl/internal/tools"
	"github.com/viche-ai/owl/internal/viche"
)

type LLMProvider interface {
	ChatStream(ctx context.Context, model string, messages []llm.Message) (<-chan llm.StreamEvent, error)
	ChatStreamWithTools(ctx context.Context, model string, messages []llm.Message, toolDefs []llm.ToolDef) (<-chan llm.StreamEvent, error)
	Name() string
}

type ModelResolver interface {
	Resolve(modelID string) (llm.Provider, string, error)
}

type VicheClient interface {
	IsAuthenticated() bool
	RegistryLabel() string
	Register(name string, capabilities []string) (string, error)
	BaseURL() string
	Token() string
}

type VicheChannel interface {
	tools.VicheChannel
	Connect() error
	SetOnMessage(func(viche.InboxMessage))
}

type LogWriter interface {
	Log(level, message string)
	LogTool(toolName, args, result string)
	LogUsage(tokensIn, tokensOut int, modelID string)
	Close() error
}

type MetricsRecorder interface {
	SetPromptHash(hash string)
	SetAgentVersion(version string)
	StartActive()
	StopActive()
	RecordToolCall(name string, success bool)
	RecordTokenUsage(input, output int)
	RecordHandoff()
	RecordTaskCreated()
	RecordTaskCompleted()
	Snapshot() metrics.RunMetrics
	Finalize(status string) *metrics.RunMetrics
}

type RunStore interface {
	Load(runID string) (*runs.RunRecord, error)
	Save(rec *runs.RunRecord) error
}

type MetricsStore interface {
	Save(m *metrics.RunMetrics) error
}

type TaskLedgerStore interface {
	AddTask(summary, source string) *Task
	UpdateTask(id string, status TaskStatus)
	ContextSummary() string
}

type Clock func() time.Time

type SleepFunc func(ctx context.Context, d time.Duration) error

type HomeDirFunc func() (string, error)

type GetwdFunc func() (string, error)

type LogWriterFactory func(logDir, runID, agentName, agentID, modelID string) (LogWriter, string, error)

type MetricsFactory func(runID, agentName, modelID, adapter, workspace string) MetricsRecorder

type VicheClientFactory func(baseURL, token string) VicheClient

type VicheChannelFactory func(baseURL, agentID, token string) VicheChannel

type TaskLedgerFactory func(agentID string) TaskLedgerStore

type noopLogWriter struct{}

func (noopLogWriter) Log(level, message string)                        {}
func (noopLogWriter) LogTool(toolName, args, result string)            {}
func (noopLogWriter) LogUsage(tokensIn, tokensOut int, modelID string) {}
func (noopLogWriter) Close() error                                     { return nil }

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func defaultLogWriterFactory(logDir, runID, agentName, agentID, modelID string) (LogWriter, string, error) {
	return logs.NewWriter(logDir, runID, agentName, agentID, modelID)
}

func defaultMetricsFactory(runID, agentName, modelID, adapter, workspace string) MetricsRecorder {
	return metrics.NewCollector(runID, agentName, modelID, adapter, workspace)
}

func defaultVicheClientFactory(baseURL, token string) VicheClient {
	return viche.NewClient(baseURL, token)
}

func defaultVicheChannelFactory(baseURL, agentID, token string) VicheChannel {
	return viche.NewChannel(baseURL, agentID, token)
}

func defaultTaskLedgerFactory(agentID string) TaskLedgerStore {
	return NewTaskLedger(agentID)
}

func (e *AgentEngine) initDeps() {
	if e.Mu == nil {
		e.Mu = func(f func()) { f() }
	}
	if e.Clock == nil {
		e.Clock = time.Now
	}
	if e.Sleep == nil {
		e.Sleep = sleepWithContext
	}
	if e.HomeDir == nil {
		e.HomeDir = os.UserHomeDir
	}
	if e.Getwd == nil {
		e.Getwd = os.Getwd
	}
	if e.NewLogWriter == nil {
		e.NewLogWriter = defaultLogWriterFactory
	}
	if e.NewMetricsRecorder == nil {
		e.NewMetricsRecorder = defaultMetricsFactory
	}
	if e.NewVicheClient == nil {
		e.NewVicheClient = defaultVicheClientFactory
	}
	if e.NewVicheChannel == nil {
		e.NewVicheChannel = defaultVicheChannelFactory
	}
	if e.NewTaskLedger == nil {
		e.NewTaskLedger = defaultTaskLedgerFactory
	}
	if e.logWriter == nil {
		e.logWriter = noopLogWriter{}
	}
}
