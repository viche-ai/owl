package runs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/viche-ai/owl/internal/runs"
)

func newTestStore(t *testing.T) *runs.Store {
	t.Helper()
	dir := t.TempDir()
	return &runs.Store{Dir: dir}
}

func sampleRecord(runID string) *runs.RunRecord {
	return &runs.RunRecord{
		RunID:     runID,
		AgentName: "test-agent",
		ModelID:   "anthropic/claude-sonnet-4-6",
		State:     "flying",
		StartTime: time.Now().UTC().Truncate(time.Second),
		WorkDir:   "/tmp",
	}
}

func TestSaveAndLoad(t *testing.T) {
	store := newTestStore(t)
	rec := sampleRecord("run-abc123")

	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("run-abc123")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.RunID != rec.RunID {
		t.Errorf("RunID mismatch: got %q want %q", loaded.RunID, rec.RunID)
	}
	if loaded.AgentName != rec.AgentName {
		t.Errorf("AgentName mismatch: got %q want %q", loaded.AgentName, rec.AgentName)
	}
}

func TestList(t *testing.T) {
	store := newTestStore(t)

	for _, id := range []string{"run-1", "run-2", "run-3"} {
		rec := sampleRecord(id)
		rec.StartTime = time.Now().Add(time.Duration(len(id)) * time.Second)
		if err := store.Save(rec); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}

	records, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}
}

func TestListEmpty(t *testing.T) {
	store := newTestStore(t)
	records, err := store.List()
	if err != nil {
		t.Fatalf("List on empty dir: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestArchive(t *testing.T) {
	store := newTestStore(t)
	rec := sampleRecord("run-archive-test")
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Archive("run-archive-test"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	loaded, err := store.Load("run-archive-test")
	if err != nil {
		t.Fatalf("Load after archive: %v", err)
	}
	if loaded.State != "archived" {
		t.Errorf("expected state=archived, got %q", loaded.State)
	}
	if loaded.EndTime == nil {
		t.Error("expected EndTime to be set after archive")
	}
}

func TestArchiveNotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.Archive("nonexistent-run")
	if err == nil {
		t.Error("expected error when archiving nonexistent run")
	}
}

func TestDelete(t *testing.T) {
	store := newTestStore(t)
	rec := sampleRecord("run-delete-test")
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete("run-delete-test"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	path := filepath.Join(store.Dir, "run-delete-test.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestListSortedByStartTime(t *testing.T) {
	store := newTestStore(t)
	base := time.Now()

	ids := []string{"run-c", "run-a", "run-b"}
	times := []time.Time{
		base.Add(3 * time.Second),
		base.Add(1 * time.Second),
		base.Add(2 * time.Second),
	}

	for i, id := range ids {
		rec := sampleRecord(id)
		rec.StartTime = times[i]
		if err := store.Save(rec); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}

	records, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Should be sorted: run-a (t+1s), run-b (t+2s), run-c (t+3s)
	expected := []string{"run-a", "run-b", "run-c"}
	for i, r := range records {
		if r.RunID != expected[i] {
			t.Errorf("position %d: got %q want %q", i, r.RunID, expected[i])
		}
	}
}
