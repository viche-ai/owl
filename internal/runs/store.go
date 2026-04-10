package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunRecord persists essential run metadata to disk.
type RunRecord struct {
	RunID      string     `json:"run_id"`
	AgentName  string     `json:"agent_name"`
	AgentDef   string     `json:"agent_def,omitempty"` // agent definition name if used
	ModelID    string     `json:"model_id"`
	Harness    string     `json:"harness,omitempty"`
	State      string     `json:"state"` // hatching, flying, idle, stopped, archived, error
	StartTime  time.Time  `json:"start_time"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	ExitReason string     `json:"exit_reason,omitempty"` // "user-stop", "force-stop", "completed", "error"
	LogPath    string     `json:"log_path"`
	WorkDir    string     `json:"work_dir"`
}

// Store manages run records on disk at ~/.owl/runs/.
type Store struct {
	Dir string
}

// NewStore creates a Store backed by ~/.owl/runs/, creating the directory if needed.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".owl", "runs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create runs dir: %w", err)
	}
	return &Store{Dir: dir}, nil
}

// Save writes or updates a RunRecord to <run_id>.json.
func (s *Store) Save(rec *RunRecord) error {
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.Dir, rec.RunID+".json")
	return os.WriteFile(path, b, 0600)
}

// Load reads a RunRecord by run ID.
func (s *Store) Load(runID string) (*RunRecord, error) {
	path := filepath.Join(s.Dir, runID+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rec RunRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// List returns all run records sorted by start time (oldest first).
func (s *Store) List() ([]RunRecord, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []RunRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			continue
		}
		var rec RunRecord
		if err := json.Unmarshal(b, &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].StartTime.Before(records[j].StartTime)
	})
	return records, nil
}

// Archive sets the record's state to "archived" and stamps an end time if not already set.
func (s *Store) Archive(runID string) error {
	rec, err := s.Load(runID)
	if err != nil {
		return fmt.Errorf("run %q not found: %w", runID, err)
	}
	rec.State = "archived"
	now := time.Now()
	if rec.EndTime == nil {
		rec.EndTime = &now
	}
	return s.Save(rec)
}

// Delete hard-deletes a run record from disk.
func (s *Store) Delete(runID string) error {
	path := filepath.Join(s.Dir, runID+".json")
	return os.Remove(path)
}
