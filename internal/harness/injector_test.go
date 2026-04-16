package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viche-ai/owl/internal/agents"
)

func testAgentDef() *agents.AgentDefinition {
	return &agents.AgentDefinition{
		Name:         "reviewer",
		AgentsMD:     "# Reviewer\nYou review code carefully.",
		RoleMD:       "Role: Senior code reviewer",
		GuardrailsMD: "Never approve without tests.",
	}
}

func TestInjectAgentContext_NilAgentDef(t *testing.T) {
	def := &HarnessDefinition{Name: "test", Binary: "echo"}

	desc, env, cleanup, err := InjectAgentContext(def, nil, "/tmp", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc != "hello" {
		t.Fatalf("description should be unchanged, got %q", desc)
	}
	if len(env) != 0 {
		t.Fatalf("env should be empty, got %v", env)
	}
	cleanup() // should be a no-op
}

func TestInjectAgentContext_ArgPrepend(t *testing.T) {
	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
		ContextInjection: &ContextInjectionConfig{
			Method: "arg-prepend",
		},
	}

	desc, env, cleanup, err := InjectAgentContext(def, testAgentDef(), "/tmp", "review PR #42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	// Description should be prepended with agent content
	if !strings.Contains(desc, "# Reviewer") {
		t.Fatalf("description should contain AGENTS.md content, got %q", desc)
	}
	if !strings.Contains(desc, "review PR #42") {
		t.Fatalf("description should still contain original, got %q", desc)
	}
	if len(env) != 0 {
		t.Fatalf("env should be empty for arg-prepend, got %v", env)
	}
}

func TestInjectAgentContext_ArgPrepend_DefaultFallback(t *testing.T) {
	// No ContextInjection set — should default to arg-prepend
	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
	}

	desc, _, cleanup, err := InjectAgentContext(def, testAgentDef(), "/tmp", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if !strings.Contains(desc, "# Reviewer") {
		t.Fatalf("should default to arg-prepend, got %q", desc)
	}
}

func TestInjectAgentContext_Env(t *testing.T) {
	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
		ContextInjection: &ContextInjectionConfig{
			Method: "env",
			EnvVar: "MY_PROMPT",
		},
	}

	desc, env, cleanup, err := InjectAgentContext(def, testAgentDef(), "/tmp", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	// Description should be unchanged
	if desc != "hello" {
		t.Fatalf("description should be unchanged for env method, got %q", desc)
	}

	// Env should have the prompt
	if len(env) != 1 {
		t.Fatalf("expected 1 env var, got %d: %v", len(env), env)
	}
	if !strings.Contains(env[0], "MY_PROMPT=") {
		t.Fatalf("env var should start with MY_PROMPT=, got %q", env[0])
	}
	if !strings.Contains(env[0], "# Reviewer") {
		t.Fatalf("env var should contain agent content, got %q", env[0])
	}
}

func TestInjectAgentContext_EnvDefaultVar(t *testing.T) {
	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
		ContextInjection: &ContextInjectionConfig{
			Method: "env",
			// No EnvVar specified — should default to OWL_AGENT_PROMPT
		},
	}

	_, env, cleanup, err := InjectAgentContext(def, testAgentDef(), "/tmp", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if len(env) != 1 || !strings.Contains(env[0], "OWL_AGENT_PROMPT=") {
		t.Fatalf("should default to OWL_AGENT_PROMPT, got %v", env)
	}
}

func TestInjectAgentContext_File(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "CLAUDE.md")

	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
		ContextInjection: &ContextInjectionConfig{
			Method: "file",
			Path:   "{{workdir}}/CLAUDE.md",
		},
	}

	desc, env, cleanup, err := InjectAgentContext(def, testAgentDef(), dir, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Description unchanged for file method
	if desc != "hello" {
		t.Fatalf("description should be unchanged, got %q", desc)
	}
	if len(env) != 0 {
		t.Fatalf("env should be empty, got %v", env)
	}

	// File should exist with agent content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("injected file should exist: %v", err)
	}
	if !strings.Contains(string(content), "# Reviewer") {
		t.Fatalf("file should contain agent content, got %q", string(content))
	}

	// Cleanup should remove the file
	cleanup()
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("cleanup should remove injected file")
	}
}

func TestInjectAgentContext_FileBackupRestore(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "CLAUDE.md")
	originalContent := "# Original CLAUDE.md content"

	// Create an existing file
	if err := os.WriteFile(filePath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
		ContextInjection: &ContextInjectionConfig{
			Method: "file",
			Path:   "{{workdir}}/CLAUDE.md",
		},
	}

	_, _, cleanup, err := InjectAgentContext(def, testAgentDef(), dir, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should now contain injected content
	content, _ := os.ReadFile(filePath)
	if strings.Contains(string(content), originalContent) {
		t.Fatal("file should contain injected content, not original")
	}

	// Backup should exist
	backupPath := filePath + ".owl-backup"
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup should exist: %v", err)
	}

	// Cleanup should restore original
	cleanup()

	restored, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("original file should be restored: %v", err)
	}
	if string(restored) != originalContent {
		t.Fatalf("restored content should match original, got %q", string(restored))
	}

	// Backup should be gone
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatal("backup should be removed after restore")
	}
}

func TestInjectAgentContext_EmptyAgentContent(t *testing.T) {
	def := &HarnessDefinition{
		Name:   "test",
		Binary: "echo",
		ContextInjection: &ContextInjectionConfig{
			Method: "arg-prepend",
		},
	}
	emptyDef := &agents.AgentDefinition{Name: "empty"}

	desc, _, cleanup, err := InjectAgentContext(def, emptyDef, "/tmp", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	// With empty content, description should be unchanged
	if desc != "hello" {
		t.Fatalf("description should be unchanged for empty agent, got %q", desc)
	}
}

func TestInjectAgentContext_ClaudeCodeBuiltin(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry()
	def, _ := r.Resolve("claude-code")

	_, _, cleanup, err := InjectAgentContext(def, testAgentDef(), dir, "review code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// claude-code built-in uses file method → CLAUDE.md
	filePath := filepath.Join(dir, "CLAUDE.md")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("CLAUDE.md should be created: %v", err)
	}
	if !strings.Contains(string(content), "# Reviewer") {
		t.Fatalf("CLAUDE.md should contain agent content, got %q", string(content))
	}

	cleanup()
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatal("cleanup should remove CLAUDE.md")
	}
}
