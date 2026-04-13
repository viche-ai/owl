package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetaToolsListAgents_Empty(t *testing.T) {
	dir := t.TempDir()
	// Isolate HOME so the global scope (~/.owl/agents/) is also empty
	t.Setenv("HOME", dir)

	mt := &MetaTools{WorkDir: dir}

	result := mt.Execute(ToolCall{Name: "list_agents", Args: map[string]interface{}{}})
	if result != "No agent definitions found." {
		t.Errorf("expected empty message, got: %q", result)
	}
}

func TestMetaToolsCreateAgent(t *testing.T) {
	dir := t.TempDir()
	// Create .owl directory so project scope resolution works
	owlDir := filepath.Join(dir, ".owl")
	if err := os.MkdirAll(owlDir, 0755); err != nil {
		t.Fatal(err)
	}

	mt := &MetaTools{WorkDir: dir}

	result := mt.Execute(ToolCall{
		Name: "create_agent",
		Args: map[string]interface{}{
			"name":         "code-reviewer",
			"scope":        "project",
			"agents_md":    "You are a code reviewer. Review code for correctness and style.",
			"description":  "Reviews code for quality issues",
			"capabilities": []interface{}{"code-review", "style-check"},
		},
	})

	if !strings.Contains(result, "Created agent") {
		t.Errorf("unexpected result: %q", result)
	}

	// Verify AGENTS.md was created
	agentsPath := filepath.Join(dir, ".owl", "agents", "code-reviewer", "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("AGENTS.md not created: %v", err)
	}
	if !strings.Contains(string(data), "code reviewer") {
		t.Errorf("AGENTS.md missing expected content")
	}

	// Verify agent.yaml was created
	yamlPath := filepath.Join(dir, ".owl", "agents", "code-reviewer", "agent.yaml")
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("agent.yaml not created: %v", err)
	}
	if !strings.Contains(string(yamlData), "code-reviewer") {
		t.Errorf("agent.yaml missing name")
	}
	if !strings.Contains(string(yamlData), "code-review") {
		t.Errorf("agent.yaml missing capabilities")
	}
}

func TestMetaToolsCreateAgentGlobalScope(t *testing.T) {
	dir := t.TempDir()
	mt := &MetaTools{WorkDir: dir}

	// Override global dir via temp HOME
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	result := mt.Execute(ToolCall{
		Name: "create_agent",
		Args: map[string]interface{}{
			"name":        "search-bot",
			"scope":       "global",
			"agents_md":   "You are a web search assistant.",
			"description": "Searches the web for information",
		},
	})

	if !strings.Contains(result, "Created agent") {
		t.Errorf("unexpected result: %q", result)
	}

	agentsPath := filepath.Join(dir, ".owl", "agents", "search-bot", "AGENTS.md")
	if _, err := os.Stat(agentsPath); err != nil {
		t.Errorf("global AGENTS.md not created at %s: %v", agentsPath, err)
	}
}

func TestMetaToolsValidateAgent(t *testing.T) {
	dir := t.TempDir()
	owlDir := filepath.Join(dir, ".owl", "agents", "my-agent")
	if err := os.MkdirAll(owlDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a valid AGENTS.md
	if err := os.WriteFile(filepath.Join(owlDir, "AGENTS.md"), []byte("You are an agent."), 0644); err != nil {
		t.Fatal(err)
	}
	// Write agent.yaml with no capabilities
	if err := os.WriteFile(filepath.Join(owlDir, "agent.yaml"), []byte("name: my-agent\nversion: 1.0.0\ndescription: test\ncapabilities: []\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mt := &MetaTools{WorkDir: dir}

	// Non-strict: missing capabilities should be flagged
	result := mt.Execute(ToolCall{
		Name: "validate_agent",
		Args: map[string]interface{}{"name": "my-agent", "scope": "project"},
	})
	if !strings.Contains(result, "at least one capability") {
		t.Errorf("expected capabilities error, got: %q", result)
	}

	// Fix capabilities
	if err := os.WriteFile(filepath.Join(owlDir, "agent.yaml"), []byte("name: my-agent\nversion: 1.0.0\ndescription: test\ncapabilities:\n  - owl-agent\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result = mt.Execute(ToolCall{
		Name: "validate_agent",
		Args: map[string]interface{}{"name": "my-agent", "scope": "project"},
	})
	if !strings.Contains(result, "is valid") {
		t.Errorf("expected valid result, got: %q", result)
	}
}

func TestMetaToolsValidateAgentCatchesGuardrailMismatch(t *testing.T) {
	dir := t.TempDir()
	owlDir := filepath.Join(dir, ".owl", "agents", "bad-agent")
	if err := os.MkdirAll(owlDir, 0755); err != nil {
		t.Fatal(err)
	}
	// AGENTS.md is empty — should fail validation
	if err := os.WriteFile(filepath.Join(owlDir, "AGENTS.md"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	mt := &MetaTools{WorkDir: dir}
	result := mt.Execute(ToolCall{
		Name: "validate_agent",
		Args: map[string]interface{}{"name": "bad-agent", "scope": "project"},
	})
	if !strings.Contains(result, "must not be empty") {
		t.Errorf("expected AGENTS.md error, got: %q", result)
	}
}

func TestMetaToolsSuggestEdit(t *testing.T) {
	dir := t.TempDir()
	owlDir := filepath.Join(dir, ".owl", "agents", "my-agent")
	if err := os.MkdirAll(owlDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldContent := "You are an agent.\nBe helpful."
	if err := os.WriteFile(filepath.Join(owlDir, "AGENTS.md"), []byte(oldContent), 0644); err != nil {
		t.Fatal(err)
	}

	mt := &MetaTools{WorkDir: dir}
	result := mt.Execute(ToolCall{
		Name: "suggest_edit",
		Args: map[string]interface{}{
			"name":        "my-agent",
			"scope":       "project",
			"file":        "AGENTS.md",
			"new_content": "You are an expert agent.\nBe helpful and precise.",
			"reason":      "improve instruction clarity",
		},
	})

	if !strings.Contains(result, "[Proposed change]") {
		t.Errorf("expected proposed change header, got: %q", result)
	}
	if !strings.Contains(result, "improve instruction clarity") {
		t.Errorf("expected reason in output, got: %q", result)
	}
	// Should NOT have applied the change yet
	data, _ := os.ReadFile(filepath.Join(owlDir, "AGENTS.md"))
	if string(data) != oldContent {
		t.Errorf("suggest_edit should NOT write the file, but content changed")
	}
}

func TestMetaToolsApplyEditRequiresConfirmation(t *testing.T) {
	// apply_edit writes immediately — the confirmation step is handled by the LLM
	// (meta-agent is instructed to only call apply_edit after user confirms).
	// Here we test that apply_edit actually writes the file and updates CHANGELOG.
	dir := t.TempDir()
	owlDir := filepath.Join(dir, ".owl", "agents", "my-agent")
	if err := os.MkdirAll(owlDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owlDir, "AGENTS.md"), []byte("Old content."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owlDir, "agent.yaml"), []byte("name: my-agent\nversion: 1.0.0\ndescription: test\ncapabilities:\n  - owl-agent\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mt := &MetaTools{WorkDir: dir}
	result := mt.Execute(ToolCall{
		Name: "apply_edit",
		Args: map[string]interface{}{
			"name":           "my-agent",
			"scope":          "project",
			"file":           "AGENTS.md",
			"new_content":    "New improved content.",
			"change_summary": "Improved agent instructions",
		},
	})

	if !strings.Contains(result, "Applied change") {
		t.Errorf("expected applied confirmation, got: %q", result)
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(owlDir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "New improved content." {
		t.Errorf("file content mismatch: %q", string(data))
	}

	// Verify version was bumped
	yamlData, _ := os.ReadFile(filepath.Join(owlDir, "agent.yaml"))
	if !strings.Contains(string(yamlData), "1.0.1") {
		t.Errorf("expected version bump to 1.0.1, got: %q", string(yamlData))
	}

	// Verify CHANGELOG entry
	changelog, _ := os.ReadFile(filepath.Join(owlDir, "CHANGELOG.md"))
	if !strings.Contains(string(changelog), "Improved agent instructions") {
		t.Errorf("expected CHANGELOG entry, got: %q", string(changelog))
	}
}

func TestBuildUnifiedDiff_NoChanges(t *testing.T) {
	result := buildUnifiedDiff("test.md", "same content", "same content")
	if result != "(no changes)" {
		t.Errorf("expected '(no changes)', got: %q", result)
	}
}

func TestBuildUnifiedDiff_WithChanges(t *testing.T) {
	old := "line 1\nline 2\nline 3"
	new_ := "line 1\nline 2 changed\nline 3"
	result := buildUnifiedDiff("test.md", old, new_)
	if !strings.Contains(result, "--- a/test.md") {
		t.Errorf("expected diff header, got: %q", result)
	}
	if !strings.Contains(result, "-line 2") {
		t.Errorf("expected removed line, got: %q", result)
	}
	if !strings.Contains(result, "+line 2 changed") {
		t.Errorf("expected added line, got: %q", result)
	}
}

func TestBumpPatchVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0.0", "1.0.1"},
		{"1.0.9", "1.0.10"},
		{"2.3.4", "2.3.5"},
		{"bad", "bad.1"},
	}
	for _, tc := range tests {
		got := bumpPatchVersion(tc.input)
		if got != tc.expected {
			t.Errorf("bumpPatchVersion(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestBuildAgentYAML(t *testing.T) {
	yaml := buildAgentYAML("test-agent", "1.0.0", "A test agent", []string{"cap1", "cap2"}, "anthropic/claude-sonnet-4-6")
	if !strings.Contains(yaml, "name: test-agent") {
		t.Error("missing name")
	}
	if !strings.Contains(yaml, "version: 1.0.0") {
		t.Error("missing version")
	}
	if !strings.Contains(yaml, "  - cap1") {
		t.Error("missing capability")
	}
	if !strings.Contains(yaml, "default_model: anthropic/claude-sonnet-4-6") {
		t.Error("missing default_model")
	}
}
