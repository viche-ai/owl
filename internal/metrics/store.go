package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store persists RunMetrics to ~/.owl/metrics/<run-id>.json.
type Store struct {
	Dir string
}

// NewStore returns a Store rooted at ~/.owl/metrics/.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".owl", "metrics")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// Save writes a RunMetrics record as JSON to disk, overwriting any existing file.
func (s *Store) Save(m *RunMetrics) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, m.RunID+".json"), b, 0644)
}

// Load reads a RunMetrics record by run ID.
func (s *Store) Load(runID string) (*RunMetrics, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, runID+".json"))
	if err != nil {
		return nil, err
	}
	var m RunMetrics
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListOpts filters results for Store.List.
type ListOpts struct {
	AgentName string
	Since     time.Time
	Status    string
	Limit     int
}

// List returns metrics records matching the given options, newest first.
func (s *Store) List(opts ListOpts) ([]RunMetrics, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []RunMetrics
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			continue
		}
		var m RunMetrics
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		if opts.AgentName != "" && m.AgentName != opts.AgentName {
			continue
		}
		if opts.Status != "" && m.Status != opts.Status {
			continue
		}
		if !opts.Since.IsZero() && m.StartTS.Before(opts.Since) {
			continue
		}
		out = append(out, m)
	}

	// newest first
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTS.After(out[j].StartTS)
	})

	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// FailureMode summarises a recurring failure pattern.
type FailureMode struct {
	Pattern  string    `json:"pattern"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

// PromptVersionStats aggregates metrics for a specific prompt hash.
type PromptVersionStats struct {
	PromptHash  string  `json:"prompt_hash"`
	RunCount    int     `json:"run_count"`
	SuccessRate float64 `json:"success_rate"`
}

// AgentMetricsSummary is the aggregate view for a single agent.
type AgentMetricsSummary struct {
	AgentName      string               `json:"agent_name"`
	TotalRuns      int                  `json:"total_runs"`
	SuccessRate    float64              `json:"success_rate"`
	AvgDurationMS  int64                `json:"avg_duration_ms"`
	AvgTokensIn    int                  `json:"avg_tokens_in"`
	AvgTokensOut   int                  `json:"avg_tokens_out"`
	TotalCost      float64              `json:"total_cost"`
	PromptVersions []PromptVersionStats `json:"prompt_versions,omitempty"`
}

// Aggregate returns aggregate statistics for a named agent across all stored runs.
func (s *Store) Aggregate(agentName string) (*AgentMetricsSummary, error) {
	records, err := s.List(ListOpts{AgentName: agentName})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no metrics found for agent %q", agentName)
	}

	summary := &AgentMetricsSummary{AgentName: agentName, TotalRuns: len(records)}

	var successCount int
	var totalDuration int64
	var totalIn, totalOut int
	promptStats := make(map[string]*PromptVersionStats)

	for _, m := range records {
		if m.Status == "completed" {
			successCount++
		}
		totalDuration += m.DurationMS
		totalIn += m.TokenInput
		totalOut += m.TokenOutput
		summary.TotalCost += m.EstimatedCost

		if m.PromptHash != "" {
			ps, ok := promptStats[m.PromptHash]
			if !ok {
				ps = &PromptVersionStats{PromptHash: m.PromptHash}
				promptStats[m.PromptHash] = ps
			}
			ps.RunCount++
			if m.Status == "completed" {
				ps.SuccessRate = (ps.SuccessRate*float64(ps.RunCount-1) + 1) / float64(ps.RunCount)
			} else {
				ps.SuccessRate = ps.SuccessRate * float64(ps.RunCount-1) / float64(ps.RunCount)
			}
		}
	}

	summary.SuccessRate = float64(successCount) / float64(len(records))
	summary.AvgDurationMS = totalDuration / int64(len(records))
	summary.AvgTokensIn = totalIn / len(records)
	summary.AvgTokensOut = totalOut / len(records)

	for _, ps := range promptStats {
		summary.PromptVersions = append(summary.PromptVersions, *ps)
	}
	sort.Slice(summary.PromptVersions, func(i, j int) bool {
		return summary.PromptVersions[i].RunCount > summary.PromptVersions[j].RunCount
	})

	return summary, nil
}

// QueryMetrics returns a human-readable metrics summary for use by the meta-agent.
func (s *Store) QueryMetrics(agentName string, since time.Time) (string, error) {
	opts := ListOpts{AgentName: agentName, Since: since, Limit: 50}
	records, err := s.List(opts)
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		msg := "No metrics found"
		if agentName != "" {
			msg += " for agent " + agentName
		}
		if !since.IsZero() {
			msg += fmt.Sprintf(" since %s", since.Format("2006-01-02 15:04"))
		}
		return msg, nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Metrics (%d runs):", len(records)))
	for _, m := range records {
		dur := ""
		if m.DurationMS > 0 {
			dur = fmt.Sprintf(", %.1fs", float64(m.DurationMS)/1000)
		}
		cost := ""
		if m.EstimatedCost > 0 {
			cost = fmt.Sprintf(", ~$%.4f", m.EstimatedCost)
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s (%s) model=%s tokens=%d/%d tools=%d/%d%s%s",
			m.StartTS.Format("2006-01-02 15:04"),
			m.RunID,
			m.Status,
			m.Model,
			m.TokenInput,
			m.TokenOutput,
			m.ToolCallCount,
			m.ToolFailCount,
			dur,
			cost,
		))
	}
	return strings.Join(lines, "\n"), nil
}

// CompareVersions returns a comparison of metrics across prompt versions for an agent.
func (s *Store) CompareVersions(agentName string) (string, error) {
	summary, err := s.Aggregate(agentName)
	if err != nil {
		return "", err
	}
	if len(summary.PromptVersions) == 0 {
		return fmt.Sprintf("No prompt version data available for agent %q.", agentName), nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Prompt version comparison for %q (%d total runs, %.0f%% success rate overall):",
		agentName, summary.TotalRuns, summary.SuccessRate*100))
	lines = append(lines, fmt.Sprintf("Avg duration: %.1fs | Avg tokens: %d in / %d out | Total cost: ~$%.4f",
		float64(summary.AvgDurationMS)/1000, summary.AvgTokensIn, summary.AvgTokensOut, summary.TotalCost))
	lines = append(lines, "")
	lines = append(lines, "Prompt versions (by run count):")
	for _, pv := range summary.PromptVersions {
		lines = append(lines, fmt.Sprintf("  hash=%s  runs=%d  success=%.0f%%",
			pv.PromptHash[:min(8, len(pv.PromptHash))],
			pv.RunCount,
			pv.SuccessRate*100,
		))
	}
	return strings.Join(lines, "\n"), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
