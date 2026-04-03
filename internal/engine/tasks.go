package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskStarted   TaskStatus = "started"
	TaskCompleted TaskStatus = "completed"
	TaskStalled   TaskStatus = "stalled"
	TaskCancelled TaskStatus = "cancelled"
	TaskWontDo    TaskStatus = "wont_do"
)

type Task struct {
	ID        string     `json:"id"`
	Status    TaskStatus `json:"status"`
	Summary   string     `json:"summary"`
	Source    string     `json:"source"`
	Created   string     `json:"created"`
	Updated   string     `json:"updated,omitempty"`
	Completed string     `json:"completed,omitempty"`
}

type TaskLedger struct {
	filePath string
	tasks    []Task
	nextID   int
}

func NewTaskLedger(agentID string) *TaskLedger {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".owl", "agents", agentID)
	os.MkdirAll(dir, 0755)

	tl := &TaskLedger{
		filePath: filepath.Join(dir, "tasks.jsonl"),
		tasks:    []Task{},
		nextID:   1,
	}

	// Load existing tasks if file exists
	tl.load()
	return tl
}

func (tl *TaskLedger) load() {
	data, err := os.ReadFile(tl.filePath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var t Task
		if err := json.Unmarshal([]byte(line), &t); err == nil {
			tl.tasks = append(tl.tasks, t)
			tl.nextID++
		}
	}
}

func (tl *TaskLedger) save() error {
	var lines []string
	for _, t := range tl.tasks {
		b, _ := json.Marshal(t)
		lines = append(lines, string(b))
	}
	return os.WriteFile(tl.filePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func (tl *TaskLedger) AddTask(summary, source string) *Task {
	t := Task{
		ID:      fmt.Sprintf("t%d", tl.nextID),
		Status:  TaskPending,
		Summary: summary,
		Source:  source,
		Created: time.Now().UTC().Format(time.RFC3339),
	}
	tl.nextID++
	tl.tasks = append(tl.tasks, t)
	tl.save()
	return &t
}

func (tl *TaskLedger) UpdateTask(id string, status TaskStatus) {
	for i := range tl.tasks {
		if tl.tasks[i].ID == id {
			tl.tasks[i].Status = status
			tl.tasks[i].Updated = time.Now().UTC().Format(time.RFC3339)
			if status == TaskCompleted {
				tl.tasks[i].Completed = time.Now().UTC().Format(time.RFC3339)
			}
			tl.save()
			return
		}
	}
}

// ContextSummary returns a compact summary of all tasks for injection into the LLM context
func (tl *TaskLedger) ContextSummary() string {
	if len(tl.tasks) == 0 {
		return "No tasks recorded yet."
	}

	var lines []string
	for _, t := range tl.tasks {
		lines = append(lines, fmt.Sprintf("- [%s] %s (id: %s, from: %s)", t.Status, t.Summary, t.ID, t.Source))
	}
	return strings.Join(lines, "\n")
}

// FindSimilar checks if a task with similar content already exists
func (tl *TaskLedger) FindSimilar(summary string) *Task {
	// Simple substring match — the LLM will do the real dedup reasoning
	lower := strings.ToLower(summary)
	for i := range tl.tasks {
		if strings.Contains(strings.ToLower(tl.tasks[i].Summary), lower) ||
			strings.Contains(lower, strings.ToLower(tl.tasks[i].Summary)) {
			return &tl.tasks[i]
		}
	}
	return nil
}
