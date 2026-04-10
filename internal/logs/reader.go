package logs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reader provides query access to .jsonl log files in a directory.
type Reader struct {
	LogDir string // e.g. ~/.owl/logs/
}

// LogFileMeta contains metadata about a single log run, sourced from .meta.json sidecars.
type LogFileMeta struct {
	RunID     string    `json:"run_id"`
	AgentName string    `json:"agent_name"`
	StartTime time.Time `json:"start_time"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
}

// QueryOpts defines filters for log queries.
type QueryOpts struct {
	AgentName string
	Since     time.Time
	Until     time.Time
	Level     string
	Limit     int
}

// List returns metadata for all log runs found in the log directory.
// It discovers runs by scanning for .meta.json sidecar files.
func (r *Reader) List() ([]LogFileMeta, error) {
	entries, err := os.ReadDir(r.LogDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []LogFileMeta
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".meta.json") {
			continue
		}
		metaPath := filepath.Join(r.LogDir, name)
		b, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta RunMeta
		if err := json.Unmarshal(b, &meta); err != nil {
			continue
		}
		logPath := filepath.Join(r.LogDir, meta.RunID+".jsonl")
		var size int64
		if info, err := os.Stat(logPath); err == nil {
			size = info.Size()
		}
		metas = append(metas, LogFileMeta{
			RunID:     meta.RunID,
			AgentName: meta.AgentName,
			StartTime: meta.StartTime,
			Path:      logPath,
			Size:      size,
		})
	}
	return metas, nil
}

// Read returns all entries from the log identified by runID.
// Sensitive values in messages are redacted before return.
func (r *Reader) Read(runID string) ([]LogEntry, error) {
	logPath := filepath.Join(r.LogDir, runID+".jsonl")
	return readJSONL(logPath)
}

// Query returns filtered log entries matching the given options across all runs.
func (r *Reader) Query(opts QueryOpts) ([]LogEntry, error) {
	metas, err := r.List()
	if err != nil {
		return nil, err
	}
	var results []LogEntry
	for _, meta := range metas {
		if opts.AgentName != "" && meta.AgentName != opts.AgentName {
			continue
		}
		entries, err := readJSONL(meta.Path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !opts.Since.IsZero() && entry.Timestamp.Before(opts.Since) {
				continue
			}
			if !opts.Until.IsZero() && entry.Timestamp.After(opts.Until) {
				continue
			}
			if opts.Level != "" && entry.Level != opts.Level {
				continue
			}
			results = append(results, entry)
			if opts.Limit > 0 && len(results) >= opts.Limit {
				return results, nil
			}
		}
	}
	return results, nil
}

// Tail streams log entries from the most recent log for the given agent (or most recent
// overall if agentName is empty). If follow is true, it polls for new entries after reaching EOF.
func (r *Reader) Tail(agentName string, follow bool) (<-chan LogEntry, error) {
	metas, err := r.List()
	if err != nil {
		return nil, err
	}
	var target LogFileMeta
	for _, meta := range metas {
		if agentName != "" && meta.AgentName != agentName {
			continue
		}
		if target.RunID == "" || meta.StartTime.After(target.StartTime) {
			target = meta
		}
	}
	if target.RunID == "" {
		if agentName != "" {
			return nil, fmt.Errorf("no log found for agent %q", agentName)
		}
		return nil, fmt.Errorf("no logs found")
	}

	ch := make(chan LogEntry, 64)
	go func() {
		defer close(ch)
		f, err := os.Open(target.Path)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()

		reader := bufio.NewReader(f)
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				trimmed := strings.TrimRight(line, "\n\r")
				if trimmed != "" {
					var entry LogEntry
					if jsonErr := json.Unmarshal([]byte(trimmed), &entry); jsonErr == nil {
						ch <- redactEntry(entry)
					}
				}
			}
			if err == io.EOF {
				if !follow {
					return
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if err != nil {
				return
			}
		}
	}()
	return ch, nil
}

// redactPatterns is the list of string prefixes that signal sensitive values.
var redactPatterns = []string{"sk-", "key-", "token=", "password=", "Bearer "}

func readJSONL(path string) ([]LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			entries = append(entries, redactEntry(entry))
		}
	}
	return entries, scanner.Err()
}

func redactEntry(entry LogEntry) LogEntry {
	entry.Message = redactString(entry.Message)
	if entry.ToolResult != "" {
		entry.ToolResult = redactString(entry.ToolResult)
	}
	return entry
}

// redactString replaces the value portion after any recognized sensitive prefix with [REDACTED].
func redactString(s string) string {
	for _, pattern := range redactPatterns {
		for {
			idx := strings.Index(s, pattern)
			if idx == -1 {
				break
			}
			// Advance past the prefix to find where the sensitive value ends.
			end := idx + len(pattern)
			for end < len(s) && s[end] != ' ' && s[end] != '\n' && s[end] != '"' && s[end] != '\t' {
				end++
			}
			s = s[:idx+len(pattern)] + "[REDACTED]" + s[end:]
		}
	}
	return s
}
