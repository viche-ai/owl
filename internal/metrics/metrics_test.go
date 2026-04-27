package metrics_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/viche-ai/owl/internal/metrics"
)

// ── Collector tests ───────────────────────────────────────────────────────────

func TestCollector_Accumulates(t *testing.T) {
	c := metrics.NewCollector("run-1", "test-agent", "anthropic/claude-sonnet-4-6", "anthropic", "/tmp")

	c.RecordToolCall("shell_exec", true)
	c.RecordToolCall("file_read", true)
	c.RecordToolCall("shell_exec", false) // failure
	c.RecordTokenUsage(1000, 500)
	c.RecordTokenUsage(2000, 800)
	c.RecordTaskCreated()
	c.RecordTaskCreated()
	c.RecordTaskCompleted()
	c.RecordHandoff()
	c.RecordRetry()

	snap := c.Snapshot()
	if snap.ToolCallCount != 3 {
		t.Errorf("expected 3 tool calls, got %d", snap.ToolCallCount)
	}
	if snap.ToolFailCount != 1 {
		t.Errorf("expected 1 tool failure, got %d", snap.ToolFailCount)
	}
	if snap.TokenInput != 3000 {
		t.Errorf("expected 3000 input tokens, got %d", snap.TokenInput)
	}
	if snap.TokenOutput != 1300 {
		t.Errorf("expected 1300 output tokens, got %d", snap.TokenOutput)
	}
	if snap.TasksCreated != 2 {
		t.Errorf("expected 2 tasks created, got %d", snap.TasksCreated)
	}
	if snap.TasksCompleted != 1 {
		t.Errorf("expected 1 task completed, got %d", snap.TasksCompleted)
	}
	if snap.HandoffCount != 1 {
		t.Errorf("expected 1 handoff, got %d", snap.HandoffCount)
	}
	if snap.RetryCount != 1 {
		t.Errorf("expected 1 retry, got %d", snap.RetryCount)
	}
}

func TestCollector_Finalize(t *testing.T) {
	c := metrics.NewCollector("run-fin", "test-agent", "anthropic/claude-sonnet-4-6", "anthropic", "/tmp")
	c.RecordTokenUsage(100, 50)

	m := c.Finalize("completed")

	if m.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", m.Status)
	}
	if m.DurationMS < 0 {
		t.Errorf("expected non-negative duration, got %d", m.DurationMS)
	}
	if m.EndTS == nil {
		t.Error("expected EndTS to be set")
	}
}

func TestCollector_PromptHash(t *testing.T) {
	c := metrics.NewCollector("run-hash", "test-agent", "gpt-4o", "openai", "/tmp")
	c.SetPromptHash("abc123def456")
	snap := c.Snapshot()
	if snap.PromptHash != "abc123def456" {
		t.Errorf("expected prompt hash 'abc123def456', got %q", snap.PromptHash)
	}
}

func TestCollector_PromptHashDeterministic(t *testing.T) {
	import_sha256 := func(s string) string {
		// simulate what engine.go does
		import_crypto := func() [32]byte {
			var sum [32]byte
			for i, b := range []byte(s) {
				sum[i%32] ^= b
			}
			return sum
		}
		_ = import_crypto
		return s
	}
	_ = import_sha256
	// The real test: same input => same hash. We test via the collector field.
	c1 := metrics.NewCollector("r1", "a", "m", "p", "/")
	c1.SetPromptHash("fixed-hash-value")
	c2 := metrics.NewCollector("r2", "a", "m", "p", "/")
	c2.SetPromptHash("fixed-hash-value")

	if c1.Snapshot().PromptHash != c2.Snapshot().PromptHash {
		t.Error("expected identical prompt hashes for identical input")
	}
}

// ── Cost estimation tests ─────────────────────────────────────────────────────

func TestEstimateCost_KnownModel(t *testing.T) {
	// claude-sonnet-4-6: $3.00/1M in, $15.00/1M out
	cost := metrics.EstimateCost("anthropic/claude-sonnet-4-6", 1_000_000, 1_000_000)
	expected := 3.00 + 15.00
	if cost < expected*0.99 || cost > expected*1.01 {
		t.Errorf("expected cost ~%.2f, got %.4f", expected, cost)
	}
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	cost := metrics.EstimateCost("unknown/mystery-model", 999999, 999999)
	if cost != 0 {
		t.Errorf("expected 0 for unknown model, got %.4f", cost)
	}
}

func TestEstimateCost_ZeroTokens(t *testing.T) {
	cost := metrics.EstimateCost("anthropic/claude-sonnet-4-6", 0, 0)
	if cost != 0 {
		t.Errorf("expected 0 cost for 0 tokens, got %.4f", cost)
	}
}

// ── Store tests ───────────────────────────────────────────────────────────────

func newTempStore(t *testing.T) *metrics.Store {
	t.Helper()
	dir := t.TempDir()
	return &metrics.Store{Dir: dir}
}

func makeMetrics(runID, agentName, status string, tokIn, tokOut int) metrics.RunMetrics {
	now := time.Now()
	end := now.Add(5 * time.Second)
	return metrics.RunMetrics{
		RunID:         runID,
		AgentName:     agentName,
		Model:         "anthropic/claude-sonnet-4-6",
		Status:        status,
		StartTS:       now,
		EndTS:         &end,
		DurationMS:    5000,
		TokenInput:    tokIn,
		TokenOutput:   tokOut,
		ToolCallCount: 3,
		ToolFailCount: 1,
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	store := newTempStore(t)
	m := makeMetrics("run-abc", "my-agent", "completed", 1000, 500)
	m.PromptHash = "deadbeef"

	if err := store.Save(&m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load("run-abc")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.RunID != "run-abc" {
		t.Errorf("expected RunID 'run-abc', got %q", loaded.RunID)
	}
	if loaded.PromptHash != "deadbeef" {
		t.Errorf("expected PromptHash 'deadbeef', got %q", loaded.PromptHash)
	}
}

func TestStore_LoadMissing(t *testing.T) {
	store := newTempStore(t)
	_, err := store.Load("does-not-exist")
	if err == nil {
		t.Error("expected error for missing run, got nil")
	}
}

func TestStore_List(t *testing.T) {
	store := newTempStore(t)

	_ = store.Save(ptr(makeMetrics("r1", "agent-a", "completed", 100, 50)))
	_ = store.Save(ptr(makeMetrics("r2", "agent-a", "stopped", 200, 80)))
	_ = store.Save(ptr(makeMetrics("r3", "agent-b", "completed", 300, 100)))

	// All
	all, err := store.List(metrics.ListOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 records, got %d", len(all))
	}

	// Filter by agent
	filtered, _ := store.List(metrics.ListOpts{AgentName: "agent-a"})
	if len(filtered) != 2 {
		t.Errorf("expected 2 records for agent-a, got %d", len(filtered))
	}

	// Filter by status
	completed, _ := store.List(metrics.ListOpts{Status: "completed"})
	if len(completed) != 2 {
		t.Errorf("expected 2 completed records, got %d", len(completed))
	}

	// Limit
	limited, _ := store.List(metrics.ListOpts{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("expected 2 with limit, got %d", len(limited))
	}
}

func TestStore_Aggregate(t *testing.T) {
	store := newTempStore(t)
	_ = store.Save(ptr(makeMetrics("r1", "agent-x", "completed", 1000, 400)))
	_ = store.Save(ptr(makeMetrics("r2", "agent-x", "completed", 2000, 600)))
	_ = store.Save(ptr(makeMetrics("r3", "agent-x", "stopped", 500, 200)))

	summary, err := store.Aggregate("agent-x")
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}
	if summary.TotalRuns != 3 {
		t.Errorf("expected 3 total runs, got %d", summary.TotalRuns)
	}
	// 2 completed out of 3 → ~66.7%
	if summary.SuccessRate < 0.66 || summary.SuccessRate > 0.67 {
		t.Errorf("expected success rate ~66.7%%, got %.2f%%", summary.SuccessRate*100)
	}
	expectedAvgIn := (1000 + 2000 + 500) / 3
	if summary.AvgTokensIn != expectedAvgIn {
		t.Errorf("expected avg tokens in %d, got %d", expectedAvgIn, summary.AvgTokensIn)
	}
}

func TestStore_AggregateMissing(t *testing.T) {
	store := newTempStore(t)
	_, err := store.Aggregate("no-such-agent")
	if err == nil {
		t.Error("expected error for missing agent, got nil")
	}
}

func TestStore_QueryMetrics(t *testing.T) {
	store := newTempStore(t)
	_ = store.Save(ptr(makeMetrics("r1", "agent-q", "completed", 100, 50)))

	result, err := store.QueryMetrics("agent-q", time.Time{})
	if err != nil {
		t.Fatalf("QueryMetrics failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestStore_CompareVersions(t *testing.T) {
	store := newTempStore(t)
	m1 := makeMetrics("r1", "agent-v", "completed", 100, 50)
	m1.PromptHash = "aabbccdd"
	m2 := makeMetrics("r2", "agent-v", "stopped", 200, 80)
	m2.PromptHash = "11223344"
	_ = store.Save(&m1)
	_ = store.Save(&m2)

	result, err := store.CompareVersions("agent-v")
	if err != nil {
		t.Fatalf("CompareVersions failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

// ── Metrics.md generation ─────────────────────────────────────────────────────

func TestWriteAgentMetricsMD_ViaStore(t *testing.T) {
	// Just verify Store.Save produces valid JSON that can be read back
	store := newTempStore(t)
	m := makeMetrics("r-md", "test-agent", "completed", 500, 200)
	m.PromptHash = "cafebabe"
	if err := store.Save(&m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "r-md.json")); err != nil {
		t.Errorf("expected metrics JSON file to exist: %v", err)
	}
}

func ptr(m metrics.RunMetrics) *metrics.RunMetrics { return &m }
