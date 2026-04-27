package engine_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/viche-ai/owl/internal/engine/testutil"
	"github.com/viche-ai/owl/internal/ipc"
	"github.com/viche-ai/owl/internal/llm"
	"github.com/viche-ai/owl/internal/runs"
)

func TestRunHappyPathNative(t *testing.T) {
	builder := testutil.NewEngineBuilder().WithLLMScript(
		[]llm.StreamEvent{{Delta: `{"name":"helper","capabilities":["analysis"],"plan":"- inspect\n- respond"}`}},
		[]llm.StreamEvent{{Delta: "Finished the task."}},
	)
	eng, fakes := builder.Build()

	fakes.Runs.Records["run-test-001"] = &runs.RunRecord{RunID: "run-test-001", State: "starting"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inbox := make(chan ipc.InboundMessage)
	close(inbox)

	eng.Run(ctx, &ipc.HatchArgs{
		Description: "finish the task",
		ModelID:     "fake/test-model",
		WorkDir:     fakes.WorkDir,
	}, inbox)

	if eng.State.State != "stopped" {
		t.Fatalf("expected final state stopped, got %q", eng.State.State)
	}
	if eng.State.Name != "helper" {
		t.Fatalf("expected scaffolded name helper, got %q", eng.State.Name)
	}
	if !fakes.Metrics.Finalized || fakes.Metrics.FinalStatus != "completed" {
		t.Fatalf("expected metrics finalized as completed, got finalized=%v status=%q", fakes.Metrics.Finalized, fakes.Metrics.FinalStatus)
	}
	if !fakes.Logs.Closed {
		t.Fatal("expected log writer to be closed")
	}
	logText := joinedLogMessages(fakes.Logs.Events)
	for _, want := range []string{
		"> Booting agent engine...",
		"> Identity configured:",
		"> Hatch complete.",
		"> Agent stopped.",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("logs missing %q:\n%s", want, logText)
		}
	}
	rec, err := fakes.Runs.Load("run-test-001")
	if err != nil {
		t.Fatalf("load run record: %v", err)
	}
	if rec.State != "stopped" {
		t.Fatalf("expected persisted run state stopped, got %q", rec.State)
	}
	if rec.LogPath == "" {
		t.Fatal("expected log path to be persisted")
	}
}

func TestRunMetaAgentMode(t *testing.T) {
	eng, fakes := testutil.NewEngineBuilder().Build()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inbox := make(chan ipc.InboundMessage)
	close(inbox)

	eng.Run(ctx, &ipc.HatchArgs{
		MetaAgent: true,
		WorkDir:   fakes.WorkDir,
	}, inbox)

	if got := len(fakes.LLM.Calls); got != 0 {
		t.Fatalf("expected no LLM calls in meta-agent mode, got %d", got)
	}
	if eng.State.Name != "owl" {
		t.Fatalf("expected meta-agent name owl, got %q", eng.State.Name)
	}
	if eng.State.State != "stopped" {
		t.Fatalf("expected final state stopped, got %q", eng.State.State)
	}
	if !strings.Contains(joinedLogMessages(fakes.Logs.Events), "> Ready. Waiting for messages...") {
		t.Fatalf("expected meta-agent ready log, got:\n%s", joinedLogMessages(fakes.Logs.Events))
	}
}

func TestRunAmbientModeSkipsInitialMessage(t *testing.T) {
	builder := testutil.NewEngineBuilder().WithLLMScript(
		[]llm.StreamEvent{{Delta: `{"name":"ambient","capabilities":["listener"],"plan":"stay ready"}`}},
	)
	eng, fakes := builder.Build()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inbox := make(chan ipc.InboundMessage)
	close(inbox)

	eng.Run(ctx, &ipc.HatchArgs{
		Ambient: true,
		ModelID: "fake/test-model",
		WorkDir: fakes.WorkDir,
	}, inbox)

	if got := len(fakes.LLM.Calls); got != 1 {
		t.Fatalf("expected only scaffolding LLM call in ambient mode, got %d", got)
	}
	if fakes.Metrics.TasksCreated != 0 {
		t.Fatalf("expected no tasks to be created in ambient mode startup, got %d", fakes.Metrics.TasksCreated)
	}
	logText := joinedLogMessages(fakes.Logs.Events)
	if !strings.Contains(logText, "> Ambient mode active. Waiting for messages...") {
		t.Fatalf("expected ambient mode log, got:\n%s", logText)
	}
}

func TestRunWithRunStorePersistsStoppedState(t *testing.T) {
	builder := testutil.NewEngineBuilder().WithLLMScript(
		[]llm.StreamEvent{{Delta: `{"name":"store-check","capabilities":["ops"],"plan":"keep state"}`}},
		[]llm.StreamEvent{{Delta: "done"}},
	)
	eng, fakes := builder.Build()
	fakes.Runs.Records["run-test-001"] = &runs.RunRecord{
		RunID:   "run-test-001",
		State:   "queued",
		WorkDir: fakes.WorkDir,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inbox := make(chan ipc.InboundMessage)
	close(inbox)

	eng.Run(ctx, &ipc.HatchArgs{
		Description: "persist my state",
		ModelID:     "fake/test-model",
		WorkDir:     fakes.WorkDir,
	}, inbox)

	rec, err := fakes.Runs.Load("run-test-001")
	if err != nil {
		t.Fatalf("load run record: %v", err)
	}
	if rec.State != "stopped" {
		t.Fatalf("expected stopped state, got %q", rec.State)
	}
	if rec.ExitReason != "completed" {
		t.Fatalf("expected exit reason completed, got %q", rec.ExitReason)
	}
	if rec.EndTime == nil {
		t.Fatal("expected end time to be written")
	}
}

func TestRunVicheUnavailable(t *testing.T) {
	builder := testutil.NewEngineBuilder().
		WithVicheError(errors.New("registry offline")).
		WithLLMScript(
			[]llm.StreamEvent{{Delta: `{"name":"offline","capabilities":["solo"],"plan":"fallback local"}`}},
			[]llm.StreamEvent{{Delta: "Completed without network."}},
		)
	eng, fakes := builder.Build()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inbox := make(chan ipc.InboundMessage)
	close(inbox)

	eng.Run(ctx, &ipc.HatchArgs{
		Description: "work offline",
		ModelID:     "fake/test-model",
		WorkDir:     fakes.WorkDir,
	}, inbox)

	if got := len(fakes.Viche.RegisterCalls); got != 1 {
		t.Fatalf("expected one Viche registration attempt, got %d", got)
	}
	if len(fakes.LLM.Calls) < 2 {
		t.Fatalf("expected a work turn after scaffolding, got %d calls", len(fakes.LLM.Calls))
	}
	for _, tool := range fakes.LLM.Calls[1].Tools {
		if tool.Name == "viche_discover" || tool.Name == "viche_send" {
			t.Fatalf("did not expect Viche tools after registration failure: %+v", fakes.LLM.Calls[1].Tools)
		}
	}
	if !strings.Contains(joinedLogMessages(fakes.Logs.Events), "Continuing without network presence") {
		t.Fatalf("expected offline fallback log, got:\n%s", joinedLogMessages(fakes.Logs.Events))
	}
}

func joinedLogMessages(events []testutil.FakeLogEvent) string {
	var b strings.Builder
	for _, event := range events {
		b.WriteString(event.Message)
		b.WriteString("\n")
	}
	return b.String()
}
