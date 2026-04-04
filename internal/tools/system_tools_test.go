package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellExec(t *testing.T) {
	st := &SystemTools{WorkDir: t.TempDir()}

	result := st.Execute(ToolCall{Name: "shell_exec", Args: map[string]interface{}{
		"command": "echo hello world",
	}})

	if !strings.Contains(result, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %s", result)
	}
}

func TestShellExecWithWorkdir(t *testing.T) {
	dir := t.TempDir()
	st := &SystemTools{WorkDir: "/tmp"}

	result := st.Execute(ToolCall{Name: "shell_exec", Args: map[string]interface{}{
		"command": "pwd",
		"workdir": dir,
	}})

	if !strings.Contains(result, dir) {
		t.Errorf("expected workdir %s in output, got: %s", dir, result)
	}
}

func TestShellExecFailingCommand(t *testing.T) {
	st := &SystemTools{WorkDir: t.TempDir()}

	result := st.Execute(ToolCall{Name: "shell_exec", Args: map[string]interface{}{
		"command": "exit 1",
	}})

	if !strings.Contains(result, "exit code") {
		t.Errorf("expected exit code in output, got: %s", result)
	}
}

func TestShellExecMissingCommand(t *testing.T) {
	st := &SystemTools{}
	result := st.Execute(ToolCall{Name: "shell_exec", Args: map[string]interface{}{}})

	if !strings.Contains(result, "Error") {
		t.Errorf("expected error for missing command, got: %s", result)
	}
}

func TestFileRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(path, []byte("hello file"), 0644)

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_read", Args: map[string]interface{}{
		"path": path,
	}})

	if result != "hello file" {
		t.Errorf("expected 'hello file', got: %s", result)
	}
}

func TestFileReadNonexistent(t *testing.T) {
	st := &SystemTools{}
	result := st.Execute(ToolCall{Name: "file_read", Args: map[string]interface{}{
		"path": "/tmp/nonexistent-owl-test-file-xyz",
	}})

	if !strings.Contains(result, "Error") {
		t.Errorf("expected error for nonexistent file, got: %s", result)
	}
}

func TestFileReadRelativePath(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "relative.txt"), []byte("relative content"), 0644)

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_read", Args: map[string]interface{}{
		"path": "relative.txt",
	}})

	if result != "relative content" {
		t.Errorf("expected 'relative content', got: %s", result)
	}
}

func TestFileWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_write", Args: map[string]interface{}{
		"path":    path,
		"content": "written content",
	}})

	if !strings.Contains(result, "File written") {
		t.Errorf("expected success message, got: %s", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "written content" {
		t.Errorf("expected 'written content', got: %s", string(data))
	}
}

func TestFileWriteCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "file.txt")

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_write", Args: map[string]interface{}{
		"path":    path,
		"content": "nested content",
	}})

	if !strings.Contains(result, "File written") {
		t.Errorf("expected success, got: %s", result)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "nested content" {
		t.Errorf("expected 'nested content', got: %s", string(data))
	}
}

func TestFileEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	_ = os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_edit", Args: map[string]interface{}{
		"path":     path,
		"old_text": "foo bar",
		"new_text": "baz qux",
	}})

	if !strings.Contains(result, "replaced 1 occurrence") {
		t.Errorf("expected success, got: %s", result)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "baz qux") {
		t.Errorf("expected edited content, got: %s", string(data))
	}
	if strings.Contains(string(data), "foo bar") {
		t.Error("old text should be replaced")
	}
}

func TestFileEditNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	_ = os.WriteFile(path, []byte("hello world\n"), 0644)

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_edit", Args: map[string]interface{}{
		"path":     path,
		"old_text": "nonexistent text",
		"new_text": "replacement",
	}})

	if !strings.Contains(result, "not found") {
		t.Errorf("expected 'not found' error, got: %s", result)
	}
}

func TestFileEditAmbiguous(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	_ = os.WriteFile(path, []byte("foo\nfoo\n"), 0644)

	st := &SystemTools{WorkDir: dir}
	result := st.Execute(ToolCall{Name: "file_edit", Args: map[string]interface{}{
		"path":     path,
		"old_text": "foo",
		"new_text": "bar",
	}})

	if !strings.Contains(result, "2 times") {
		t.Errorf("expected ambiguity error, got: %s", result)
	}

	// File should be unchanged
	data, _ := os.ReadFile(path)
	if string(data) != "foo\nfoo\n" {
		t.Error("file should not have been modified")
	}
}

func TestUnknownTool(t *testing.T) {
	st := &SystemTools{}
	result := st.Execute(ToolCall{Name: "unknown_tool", Args: map[string]interface{}{}})

	if !strings.Contains(result, "Unknown tool") {
		t.Errorf("expected unknown tool error, got: %s", result)
	}
}

func TestResolveTildePath(t *testing.T) {
	st := &SystemTools{WorkDir: "/tmp"}
	home, _ := os.UserHomeDir()

	resolved := st.resolvePath("~/test.txt")
	expected := filepath.Join(home, "test.txt")
	if resolved != expected {
		t.Errorf("expected %s, got %s", expected, resolved)
	}
}

func TestDefinitions(t *testing.T) {
	st := &SystemTools{}
	defs := st.Definitions()

	if len(defs) != 4 {
		t.Fatalf("expected 4 tool definitions, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}

	for _, expected := range []string{"shell_exec", "file_read", "file_write", "file_edit"} {
		if !names[expected] {
			t.Errorf("missing tool definition: %s", expected)
		}
	}
}
