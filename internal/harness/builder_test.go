package harness

import (
	"strings"
	"testing"
)

func TestBuildCommand_Codex(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("codex")

	bin, args, err := def.BuildCommand("fix tests", "/tmp/work", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin != "codex" {
		t.Fatalf("expected binary 'codex', got %q", bin)
	}
	expectArgs(t, args, []string{"exec", "fix tests"})
}

func TestBuildCommand_Opencode(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("opencode")

	bin, args, err := def.BuildCommand("review", "/tmp/work", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin != "opencode" {
		t.Fatalf("expected binary 'opencode', got %q", bin)
	}
	expectArgs(t, args, []string{"run", "--dir", "/tmp/work", "review"})
}

func TestBuildCommand_ClaudeCode(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("claude-code")

	bin, args, err := def.BuildCommand("plan this", "/tmp/work", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin != "claude" {
		t.Fatalf("expected binary 'claude', got %q", bin)
	}
	expectArgs(t, args, []string{"-p", "--verbose", "--output-format", "stream-json", "plan this"})
}

func TestBuildCommand_WithExtraArgs(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("codex")

	_, args, _ := def.BuildCommand("fix", "/tmp", []string{"--verbose", "--fast"})
	expectArgs(t, args, []string{"exec", "fix", "--verbose", "--fast"})
}

func TestBuildCommand_WorkDirTemplate(t *testing.T) {
	def := &HarnessDefinition{
		Name:   "test",
		Binary: "test-bin",
		Args:   []string{"--root", "{{workdir}}", "{{description}}"},
	}

	_, args, _ := def.BuildCommand("hello", "/my/dir", nil)
	expectArgs(t, args, []string{"--root", "/my/dir", "hello"})
}

func TestCheckBinary_Exists(t *testing.T) {
	// "ls" should exist on any system
	def := &HarnessDefinition{Name: "test", Binary: "ls"}
	if err := def.CheckBinary(); err != nil {
		t.Fatalf("ls should exist on PATH: %v", err)
	}
}

func TestCheckBinary_Missing(t *testing.T) {
	def := &HarnessDefinition{Name: "test", Binary: "this-binary-definitely-does-not-exist-xyz123"}
	err := def.CheckBinary()
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if got := err.Error(); !strings.Contains(got, "not found on PATH") {
		t.Fatalf("error should mention PATH, got: %s", got)
	}
}

func TestBuildCommand_ClaudeAlias(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("claude")

	bin, args, err := def.BuildCommand("hello", "/tmp", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bin != "claude" {
		t.Fatalf("expected binary 'claude', got %q", bin)
	}
	expectArgs(t, args, []string{"-p", "--verbose", "--output-format", "stream-json", "hello"})
}

func TestBuildCommand_BackwardCompat_CodexExact(t *testing.T) {
	// Verify the registry-based codex produces EXACTLY the same command
	// as the old hardcoded buildHarnessCommand.
	r := NewRegistry()
	def, _ := r.Resolve("codex")
	bin, args, _ := def.BuildCommand("fix tests", "/work", nil)

	if bin != "codex" || len(args) != 2 || args[0] != "exec" || args[1] != "fix tests" {
		t.Fatalf("codex backward compat broken: got %s %v", bin, args)
	}
}

func TestBuildCommand_BackwardCompat_OpencodeExact(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("opencode")
	bin, args, _ := def.BuildCommand("review", "/tmp/x", nil)

	if bin != "opencode" || len(args) != 4 ||
		args[0] != "run" || args[1] != "--dir" || args[2] != "/tmp/x" || args[3] != "review" {
		t.Fatalf("opencode backward compat broken: got %s %v", bin, args)
	}
}

func TestBuildCommand_ClaudeCodeStreamJSON(t *testing.T) {
	r := NewRegistry()
	def, _ := r.Resolve("claude-code")
	bin, args, _ := def.BuildCommand("plan this", "/work", nil)

	if bin != "claude" || len(args) != 5 ||
		args[0] != "-p" || args[1] != "--verbose" || args[2] != "--output-format" || args[3] != "stream-json" || args[4] != "plan this" {
		t.Fatalf("claude-code stream-json broken: got %s %v", bin, args)
	}
	if !def.Persistent {
		t.Fatal("claude-code should be persistent")
	}
	if def.OutputFormat != "claude-stream-json" {
		t.Fatalf("claude-code output format should be 'claude-stream-json', got %q", def.OutputFormat)
	}
}

func expectArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args length: got %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d]: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}
