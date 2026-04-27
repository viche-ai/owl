package testutil

import (
	"fmt"
	"sync"

	"github.com/viche-ai/owl/internal/runs"
)

type FakeRunStore struct {
	mu      sync.Mutex
	Records map[string]*runs.RunRecord
}

func NewFakeRunStore() *FakeRunStore {
	return &FakeRunStore{
		Records: make(map[string]*runs.RunRecord),
	}
}

func (f *FakeRunStore) Load(runID string) (*runs.RunRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.Records[runID]
	if !ok {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	copy := *rec
	return &copy, nil
}

func (f *FakeRunStore) Save(rec *runs.RunRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *rec
	f.Records[rec.RunID] = &copy
	return nil
}
