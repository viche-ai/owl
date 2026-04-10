package logs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion is the current log schema version.
const SchemaVersion = "1"

// LogEntry represents a single structured log line written to a .jsonl file.
type LogEntry struct {
	SchemaVersion string    `json:"schema_version"`
	Timestamp     time.Time `json:"ts"`
	RunID         string    `json:"run_id"`
	AgentName     string    `json:"agent_name"`
	AgentID       string    `json:"agent_id"`
	Level         string    `json:"level"` // info, warn, error, debug, tool, thinking
	Message       string    `json:"message"`
	ToolName      string    `json:"tool_name,omitempty"`
	ToolResult    string    `json:"tool_result,omitempty"`
	ModelID       string    `json:"model_id,omitempty"`
	TokensIn      int       `json:"tokens_in,omitempty"`
	TokensOut     int       `json:"tokens_out,omitempty"`
}

// RunMeta is persisted as a .meta.json sidecar alongside the .jsonl log file.
type RunMeta struct {
	RunID     string    `json:"run_id"`
	AgentName string    `json:"agent_name"`
	AgentID   string    `json:"agent_id"`
	StartTime time.Time `json:"start_time"`
	ModelID   string    `json:"model_id,omitempty"`
	Status    string    `json:"status"` // running, stopped, error
}

// Writer writes structured JSON log lines to a .jsonl file.
type Writer struct {
	file      *os.File
	runID     string
	agentName string
	agentID   string
}

// NewWriter creates a structured log Writer for the given runID.
// It also writes a .meta.json sidecar with run metadata.
// Returns the writer, the log file path, and any error.
func NewWriter(logDir, runID, agentName, agentID, modelID string) (*Writer, string, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, "", fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(logDir, runID+".jsonl")
	f, err := os.Create(logPath)
	if err != nil {
		return nil, logPath, fmt.Errorf("create log file: %w", err)
	}
	meta := RunMeta{
		RunID:     runID,
		AgentName: agentName,
		AgentID:   agentID,
		StartTime: time.Now().UTC(),
		ModelID:   modelID,
		Status:    "running",
	}
	if b, err := json.Marshal(meta); err == nil {
		_ = os.WriteFile(filepath.Join(logDir, runID+".meta.json"), b, 0644)
	}
	return &Writer{
		file:      f,
		runID:     runID,
		agentName: agentName,
		agentID:   agentID,
	}, logPath, nil
}

// Close closes the underlying log file.
func (w *Writer) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	return w.file.Close()
}

// Log writes a structured log entry at the given level.
func (w *Writer) Log(level, message string) {
	if w == nil || w.file == nil {
		return
	}
	entry := LogEntry{
		SchemaVersion: SchemaVersion,
		Timestamp:     time.Now().UTC(),
		RunID:         w.runID,
		AgentName:     w.agentName,
		AgentID:       w.agentID,
		Level:         level,
		Message:       message,
	}
	if b, err := json.Marshal(entry); err == nil {
		_, _ = w.file.WriteString(string(b) + "\n")
	}
}

// LogTool writes a structured tool execution entry.
func (w *Writer) LogTool(toolName, args, result string) {
	if w == nil || w.file == nil {
		return
	}
	entry := LogEntry{
		SchemaVersion: SchemaVersion,
		Timestamp:     time.Now().UTC(),
		RunID:         w.runID,
		AgentName:     w.agentName,
		AgentID:       w.agentID,
		Level:         "tool",
		Message:       fmt.Sprintf("[Tool: %s] %s", toolName, args),
		ToolName:      toolName,
		ToolResult:    result,
	}
	if b, err := json.Marshal(entry); err == nil {
		_, _ = w.file.WriteString(string(b) + "\n")
	}
}

// LogUsage writes a token usage entry.
func (w *Writer) LogUsage(tokensIn, tokensOut int, modelID string) {
	if w == nil || w.file == nil {
		return
	}
	entry := LogEntry{
		SchemaVersion: SchemaVersion,
		Timestamp:     time.Now().UTC(),
		RunID:         w.runID,
		AgentName:     w.agentName,
		AgentID:       w.agentID,
		Level:         "info",
		Message:       fmt.Sprintf("tokens: in=%d out=%d model=%s", tokensIn, tokensOut, modelID),
		ModelID:       modelID,
		TokensIn:      tokensIn,
		TokensOut:     tokensOut,
	}
	if b, err := json.Marshal(entry); err == nil {
		_, _ = w.file.WriteString(string(b) + "\n")
	}
}

// GenerateRunID creates a unique run ID for an agent.
// Format: <sanitized-name>-<unix-ms>-<4-byte-hex>
// Example: code-reviewer-1744201200000-a3b4c5d6
func GenerateRunID(agentName string) string {
	sanitized := sanitizeIDPart(agentName)
	ts := time.Now().UnixMilli()
	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	return fmt.Sprintf("%s-%d-%s", sanitized, ts, hex.EncodeToString(rnd))
}

// sanitizeIDPart lowercases the name, replaces spaces with hyphens, strips
// non-alphanumeric characters, and truncates to 20 characters.
func sanitizeIDPart(name string) string {
	s := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 20 {
		result = result[:20]
	}
	if result == "" {
		result = "agent"
	}
	return result
}
