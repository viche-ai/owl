package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SystemTools provides shell execution and file manipulation tools
type SystemTools struct {
	WorkDir string // default working directory for shell commands
}

func (st *SystemTools) Definitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "shell_exec",
			Description: "Execute a shell command and return its stdout and stderr. Use this for git, gh, mix, and any other CLI operations.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The shell command to execute",
					},
					"workdir": map[string]interface{}{
						"type":        "string",
						"description": "Working directory for the command (optional, defaults to agent working directory)",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "file_read",
			Description: "Read the contents of a file. Returns the full file content.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to read (absolute or relative to working directory)",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "file_write",
			Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to write",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "file_edit",
			Description: "Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace and newlines).",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to edit",
					},
					"old_text": map[string]interface{}{
						"type":        "string",
						"description": "Exact text to find (must match exactly)",
					},
					"new_text": map[string]interface{}{
						"type":        "string",
						"description": "Text to replace it with",
					},
				},
				"required": []string{"path", "old_text", "new_text"},
			},
		},
	}
}

func (st *SystemTools) Execute(call ToolCall) string {
	switch call.Name {
	case "shell_exec":
		return st.shellExec(call.Args)
	case "file_read":
		return st.fileRead(call.Args)
	case "file_write":
		return st.fileWrite(call.Args)
	case "file_edit":
		return st.fileEdit(call.Args)
	default:
		return fmt.Sprintf("Unknown tool: %s", call.Name)
	}
}

func (st *SystemTools) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	if st.WorkDir != "" {
		return filepath.Join(st.WorkDir, path)
	}
	return path
}

func (st *SystemTools) shellExec(args map[string]interface{}) string {
	command, _ := args["command"].(string)
	if command == "" {
		return "Error: command is required"
	}

	workdir, _ := args["workdir"].(string)
	if workdir == "" {
		workdir = st.WorkDir
	}
	if strings.HasPrefix(workdir, "~/") {
		home, _ := os.UserHomeDir()
		workdir = filepath.Join(home, workdir[2:])
	}

	cmd := exec.Command("bash", "-c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	// Set a timeout
	done := make(chan error, 1)
	var output []byte
	go func() {
		var err error
		output, err = cmd.CombinedOutput()
		done <- err
	}()

	select {
	case err := <-done:
		result := string(output)
		if err != nil {
			result += fmt.Sprintf("\n[exit code: %v]", err)
		}
		if len(result) > 50000 {
			result = result[:50000] + "\n... (truncated)"
		}
		return result
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		return "Error: command timed out after 120 seconds"
	}
}

func (st *SystemTools) fileRead(args map[string]interface{}) string {
	path, _ := args["path"].(string)
	if path == "" {
		return "Error: path is required"
	}

	path = st.resolvePath(path)

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}

	result := string(content)
	if len(result) > 100000 {
		result = result[:100000] + "\n... (truncated)"
	}
	return result
}

func (st *SystemTools) fileWrite(args map[string]interface{}) string {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return "Error: path is required"
	}

	path = st.resolvePath(path)

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("Error creating directories: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err)
	}

	return fmt.Sprintf("File written: %s (%d bytes)", path, len(content))
}

func (st *SystemTools) fileEdit(args map[string]interface{}) string {
	path, _ := args["path"].(string)
	oldText, _ := args["old_text"].(string)
	newText, _ := args["new_text"].(string)
	if path == "" || oldText == "" {
		return "Error: path and old_text are required"
	}

	path = st.resolvePath(path)

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}

	fileContent := string(content)
	if !strings.Contains(fileContent, oldText) {
		return "Error: old_text not found in file. Make sure it matches exactly (including whitespace)."
	}

	count := strings.Count(fileContent, oldText)
	if count > 1 {
		return fmt.Sprintf("Error: old_text found %d times in file. It must be unique. Add more surrounding context to make the match unique.", count)
	}

	newContent := strings.Replace(fileContent, oldText, newText, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err)
	}

	return fmt.Sprintf("File edited: %s (replaced 1 occurrence)", path)
}
