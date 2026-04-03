package tools

import (
	"strings"
	"testing"
)

// mockTaskUpdater implements TaskUpdater for testing
type mockTaskUpdater struct {
	lastID     string
	lastStatus string
	tasks      string
}

func (m *mockTaskUpdater) UpdateTaskStatus(id string, status string) string {
	m.lastID = id
	m.lastStatus = status
	return "Task " + id + " updated to " + status
}

func (m *mockTaskUpdater) ListTasks() string {
	return m.tasks
}

func TestTaskUpdateTool(t *testing.T) {
	mock := &mockTaskUpdater{}
	tt := &TaskTools{Updater: mock}

	result := tt.Execute(ToolCall{
		Name: "task_update",
		Args: map[string]interface{}{
			"id":     "t1",
			"status": "completed",
		},
	})

	if mock.lastID != "t1" {
		t.Errorf("expected id t1, got %s", mock.lastID)
	}
	if mock.lastStatus != "completed" {
		t.Errorf("expected status completed, got %s", mock.lastStatus)
	}
	if !strings.Contains(result, "updated") {
		t.Errorf("expected success message, got: %s", result)
	}
}

func TestTaskUpdateMissingFields(t *testing.T) {
	mock := &mockTaskUpdater{}
	tt := &TaskTools{Updater: mock}

	result := tt.Execute(ToolCall{
		Name: "task_update",
		Args: map[string]interface{}{},
	})

	if !strings.Contains(result, "Error") {
		t.Errorf("expected error for missing fields, got: %s", result)
	}
}

func TestTaskToolDefinitions(t *testing.T) {
	tt := &TaskTools{Updater: &mockTaskUpdater{}}
	defs := tt.Definitions()

	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "task_update" {
		t.Errorf("expected task_update, got %s", defs[0].Name)
	}
}

func TestTaskToolUnknown(t *testing.T) {
	tt := &TaskTools{Updater: &mockTaskUpdater{}}
	result := tt.Execute(ToolCall{Name: "unknown", Args: map[string]interface{}{}})

	if !strings.Contains(result, "Unknown tool") {
		t.Errorf("expected unknown tool error, got: %s", result)
	}
}
