package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	mu       sync.RWMutex
	tasks    []Task
	nextID   int
}

func NewTaskLedger(agentID string) *TaskLedger {
	home, _ := os.UserHomeDir()
	return NewTaskLedgerAt(filepath.Join(home, ".owl", "agents"), agentID)
}

func NewTaskLedgerAt(baseDir, agentID string) *TaskLedger {
	dir := filepath.Join(baseDir, agentID)
	_ = os.MkdirAll(dir, 0755)

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
	tl.mu.Lock()
	defer tl.mu.Unlock()

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

func (tl *TaskLedger) saveLocked() error {
	var lines []string
	for _, t := range tl.tasks {
		b, _ := json.Marshal(t)
		lines = append(lines, string(b))
	}
	return os.WriteFile(tl.filePath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func (tl *TaskLedger) AddTask(summary, source string) *Task {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	t := Task{
		ID:      fmt.Sprintf("t%d", tl.nextID),
		Status:  TaskPending,
		Summary: summary,
		Source:  source,
		Created: time.Now().UTC().Format(time.RFC3339),
	}
	tl.nextID++
	tl.tasks = append(tl.tasks, t)
	_ = tl.saveLocked()
	return &t
}

func (tl *TaskLedger) UpdateTask(id string, status TaskStatus) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	for i := range tl.tasks {
		if tl.tasks[i].ID == id {
			tl.tasks[i].Status = status
			tl.tasks[i].Updated = time.Now().UTC().Format(time.RFC3339)
			if status == TaskCompleted {
				tl.tasks[i].Completed = time.Now().UTC().Format(time.RFC3339)
			}
			_ = tl.saveLocked()
			return
		}
	}
}

// ContextSummary returns a compact summary of all tasks for injection into the LLM context
func (tl *TaskLedger) ContextSummary() string {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	if len(tl.tasks) == 0 {
		return "No tasks recorded yet."
	}

	counts := map[TaskStatus]int{}
	for _, t := range tl.tasks {
		counts[t.Status]++
	}

	var lines []string
	lines = append(lines, fmt.Sprintf(
		"Status counts: pending=%d started=%d completed=%d stalled=%d cancelled=%d wont_do=%d",
		counts[TaskPending],
		counts[TaskStarted],
		counts[TaskCompleted],
		counts[TaskStalled],
		counts[TaskCancelled],
		counts[TaskWontDo],
	))
	for _, t := range tl.tasks {
		lines = append(lines, fmt.Sprintf("- [%s] %s (id: %s, from: %s)", t.Status, t.Summary, t.ID, t.Source))
	}
	return strings.Join(lines, "\n")
}

// FindSimilar checks if a task with similar content already exists
func (tl *TaskLedger) FindSimilar(summary string) *Task {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

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
