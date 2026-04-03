package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestLedger(t *testing.T) (*TaskLedger, string) {
	t.Helper()
	dir := t.TempDir()
	tl := &TaskLedger{
		filePath: filepath.Join(dir, "tasks.jsonl"),
		tasks:    []Task{},
		nextID:   1,
	}
	return tl, dir
}

func TestAddTask(t *testing.T) {
	tl, _ := setupTestLedger(t)

	task := tl.AddTask("Review PR #42", "agent:abc123")

	if task.ID != "t1" {
		t.Errorf("expected ID t1, got %s", task.ID)
	}
	if task.Status != TaskPending {
		t.Errorf("expected status pending, got %s", task.Status)
	}
	if task.Summary != "Review PR #42" {
		t.Errorf("expected summary 'Review PR #42', got %s", task.Summary)
	}
	if task.Source != "agent:abc123" {
		t.Errorf("expected source 'agent:abc123', got %s", task.Source)
	}
	if task.Created == "" {
		t.Error("expected non-empty Created timestamp")
	}
	if len(tl.tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tl.tasks))
	}
}

func TestAddMultipleTasks(t *testing.T) {
	tl, _ := setupTestLedger(t)

	t1 := tl.AddTask("Task one", "user")
	t2 := tl.AddTask("Task two", "agent:xyz")

	if t1.ID != "t1" {
		t.Errorf("expected t1, got %s", t1.ID)
	}
	if t2.ID != "t2" {
		t.Errorf("expected t2, got %s", t2.ID)
	}
	if len(tl.tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tl.tasks))
	}
}

func TestUpdateTask(t *testing.T) {
	tl, _ := setupTestLedger(t)
	tl.AddTask("Do something", "user")

	tl.UpdateTask("t1", TaskStarted)
	if tl.tasks[0].Status != TaskStarted {
		t.Errorf("expected started, got %s", tl.tasks[0].Status)
	}
	if tl.tasks[0].Updated == "" {
		t.Error("expected Updated to be set")
	}

	tl.UpdateTask("t1", TaskCompleted)
	if tl.tasks[0].Status != TaskCompleted {
		t.Errorf("expected completed, got %s", tl.tasks[0].Status)
	}
	if tl.tasks[0].Completed == "" {
		t.Error("expected Completed to be set")
	}
}

func TestUpdateNonexistentTask(t *testing.T) {
	tl, _ := setupTestLedger(t)
	tl.AddTask("Do something", "user")

	// Should not panic
	tl.UpdateTask("t999", TaskCompleted)

	if tl.tasks[0].Status != TaskPending {
		t.Errorf("original task should still be pending, got %s", tl.tasks[0].Status)
	}
}

func TestContextSummary(t *testing.T) {
	tl, _ := setupTestLedger(t)

	summary := tl.ContextSummary()
	if summary != "No tasks recorded yet." {
		t.Errorf("expected empty summary, got %s", summary)
	}

	tl.AddTask("Review PR", "agent:abc")
	tl.AddTask("Fix bug", "user")
	tl.UpdateTask("t1", TaskCompleted)

	summary = tl.ContextSummary()
	if !strings.Contains(summary, "[completed]") {
		t.Error("expected completed status in summary")
	}
	if !strings.Contains(summary, "[pending]") {
		t.Error("expected pending status in summary")
	}
	if !strings.Contains(summary, "Review PR") {
		t.Error("expected task summary in output")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tasks.jsonl")

	// Create and populate
	tl1 := &TaskLedger{filePath: filePath, tasks: []Task{}, nextID: 1}
	tl1.AddTask("Task A", "user")
	tl1.AddTask("Task B", "agent:x")
	tl1.UpdateTask("t1", TaskCompleted)

	// Reload from disk
	tl2 := &TaskLedger{filePath: filePath, tasks: []Task{}, nextID: 1}
	tl2.load()

	if len(tl2.tasks) != 2 {
		t.Fatalf("expected 2 tasks after reload, got %d", len(tl2.tasks))
	}
	if tl2.tasks[0].Status != TaskCompleted {
		t.Errorf("expected first task completed after reload, got %s", tl2.tasks[0].Status)
	}
	if tl2.tasks[1].Summary != "Task B" {
		t.Errorf("expected second task summary 'Task B', got %s", tl2.tasks[1].Summary)
	}
}

func TestPersistenceFileFormat(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tasks.jsonl")

	tl := &TaskLedger{filePath: filePath, tasks: []Task{}, nextID: 1}
	tl.AddTask("Test task", "user")

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line in JSONL, got %d", len(lines))
	}

	var task Task
	if err := json.Unmarshal([]byte(lines[0]), &task); err != nil {
		t.Fatalf("line is not valid JSON: %v", err)
	}
	if task.ID != "t1" {
		t.Errorf("expected t1, got %s", task.ID)
	}
}

func TestFindSimilar(t *testing.T) {
	tl, _ := setupTestLedger(t)
	tl.AddTask("Review PR #42 for viche-ai/viche", "agent:abc")

	found := tl.FindSimilar("Review PR #42")
	if found == nil {
		t.Error("expected to find similar task")
	}

	found = tl.FindSimilar("something completely different")
	if found != nil {
		t.Error("expected no match for unrelated query")
	}
}

func TestAllStatuses(t *testing.T) {
	tl, _ := setupTestLedger(t)
	tl.AddTask("task", "user")

	statuses := []TaskStatus{TaskStarted, TaskStalled, TaskCancelled, TaskWontDo, TaskCompleted}
	for _, s := range statuses {
		tl.UpdateTask("t1", s)
		if tl.tasks[0].Status != s {
			t.Errorf("expected %s, got %s", s, tl.tasks[0].Status)
		}
	}
}
