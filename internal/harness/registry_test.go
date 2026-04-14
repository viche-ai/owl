package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRegistry_BuiltinsLoaded(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"codex", "opencode", "claude-code"} {
		if _, err := r.Resolve(name); err != nil {
			t.Errorf("built-in %q not found: %v", name, err)
		}
	}
}

func TestResolve_Alias(t *testing.T) {
	r := NewRegistry()
	def, err := r.Resolve("claude")
	if err != nil {
		t.Fatalf("alias 'claude' should resolve: %v", err)
	}
	if def.Name != "claude-code" {
		t.Fatalf("expected canonical name 'claude-code', got %q", def.Name)
	}
}

func TestResolve_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Resolve("Claude-Code"); err != nil {
		t.Fatalf("case-insensitive lookup should work: %v", err)
	}
}

func TestResolve_Unknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown harness")
	}
	// Error should list available harnesses
	if got := err.Error(); !contains(got, "claude-code") || !contains(got, "codex") {
		t.Fatalf("error should list available harnesses, got: %s", got)
	}
}

func TestResolve_Empty(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestList(t *testing.T) {
	r := NewRegistry()
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 built-ins, got %d", len(list))
	}
	// Verify sorted order
	for i := 1; i < len(list); i++ {
		if list[i].Name < list[i-1].Name {
			t.Fatalf("list not sorted: %q before %q", list[i-1].Name, list[i].Name)
		}
	}
}

func TestLoadFromDir_UserOverride(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: codex
binary: my-codex
args: ["run", "{{description}}"]
description: Custom codex
`
	if err := os.WriteFile(filepath.Join(dir, "codex.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	def, err := r.Resolve("codex")
	if err != nil {
		t.Fatalf("resolve overridden codex: %v", err)
	}
	if def.Binary != "my-codex" {
		t.Fatalf("expected binary 'my-codex', got %q", def.Binary)
	}
}

func TestLoadFromDir_CustomHarness(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: aider
binary: aider
args: ["--message", "{{description}}"]
supports_stdin: true
`
	if err := os.WriteFile(filepath.Join(dir, "aider.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	def, err := r.Resolve("aider")
	if err != nil {
		t.Fatalf("resolve custom harness: %v", err)
	}
	if def.Binary != "aider" {
		t.Fatalf("expected binary 'aider', got %q", def.Binary)
	}
	if !def.SupportsStdin {
		t.Fatal("expected supports_stdin to be true")
	}
}

func TestLoadFromDir_NameFromFilename(t *testing.T) {
	dir := t.TempDir()
	// YAML without name field — should use filename
	yaml := `binary: goose
args: ["exec", "{{description}}"]
`
	if err := os.WriteFile(filepath.Join(dir, "goose.yml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	def, err := r.Resolve("goose")
	if err != nil {
		t.Fatalf("resolve by filename: %v", err)
	}
	if def.Name != "goose" {
		t.Fatalf("expected name 'goose', got %q", def.Name)
	}
}

func TestLoadFromDir_Nonexistent(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadFromDir("/nonexistent/path"); err != nil {
		t.Fatalf("nonexistent dir should not error: %v", err)
	}
}

func TestLoadFromDir_SkipsNonYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	// Should still have only 3 built-ins
	if len(r.List()) != 3 {
		t.Fatalf("expected 3 built-ins, got %d", len(r.List()))
	}
}

func TestLoadFromDir_WithAliases(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: my-tool
binary: mytool
args: ["{{description}}"]
aliases: ["mt", "my"]
`
	if err := os.WriteFile(filepath.Join(dir, "my-tool.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}

	for _, alias := range []string{"mt", "my"} {
		def, err := r.Resolve(alias)
		if err != nil {
			t.Fatalf("alias %q should resolve: %v", alias, err)
		}
		if def.Name != "my-tool" {
			t.Fatalf("alias %q resolved to %q, expected 'my-tool'", alias, def.Name)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
