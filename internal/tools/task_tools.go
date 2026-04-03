package tools

import (
	"fmt"
)

// TaskUpdater is an interface the engine implements to let tools update task state
type TaskUpdater interface {
	UpdateTaskStatus(id string, status string) string
	ListTasks() string
}

type TaskTools struct {
	Updater TaskUpdater
}

func (tt *TaskTools) Definitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "task_update",
			Description: "Update the status of a task in your task ledger. Use this to track your work progress.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type":        "string",
						"description": "Task ID (e.g. 't1')",
					},
					"status": map[string]interface{}{
						"type":        "string",
						"description": "New status: 'started', 'completed', 'stalled', 'cancelled', 'wont_do'",
						"enum":        []string{"started", "completed", "stalled", "cancelled", "wont_do"},
					},
				},
				"required": []string{"id", "status"},
			},
		},
	}
}

func (tt *TaskTools) Execute(call ToolCall) string {
	switch call.Name {
	case "task_update":
		id, _ := call.Args["id"].(string)
		status, _ := call.Args["status"].(string)
		if id == "" || status == "" {
			return "Error: 'id' and 'status' are required"
		}
		return tt.Updater.UpdateTaskStatus(id, status)
	default:
		return fmt.Sprintf("Unknown tool: %s", call.Name)
	}
}
