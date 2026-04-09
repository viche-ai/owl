package agents

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ValidateImportPath checks that path is suitable for import.
// For directories, AGENTS.md must be present inside.
// For files, the file must have a .md extension.
func ValidateImportPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path %q not found: %w", path, err)
	}

	if info.IsDir() {
		if _, err := os.Stat(filepath.Join(path, "AGENTS.md")); err != nil {
			return fmt.Errorf("directory %q has no AGENTS.md", path)
		}
		return nil
	}

	if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
		return fmt.Errorf("file %q is not a markdown file (.md)", path)
	}
	return nil
}

// SuggestFixes returns human-readable suggestions for an incomplete agent definition.
func SuggestFixes(def *AgentDefinition) []string {
	var suggestions []string
	if def.Version == "" {
		suggestions = append(suggestions, `Missing version in agent.yaml — add: version: "1.0.0"`)
	}
	if len(def.Capabilities) == 0 {
		suggestions = append(suggestions, "Missing capabilities in agent.yaml — add at least one, e.g.: capabilities: [general]")
	}
	if def.Description == "" {
		suggestions = append(suggestions, `Missing description in agent.yaml — add: description: "Brief description"`)
	}
	return suggestions
}

// AutoGenerateYAML creates a minimal agent.yaml based on AGENTS.md content.
// nameOverride takes precedence; otherwise the H1 heading is used.
func AutoGenerateYAML(agentsMD string, nameOverride string) string {
	name := nameOverride
	if name == "" {
		name = extractH1(agentsMD)
	}
	if name == "" {
		name = "unnamed-agent"
	}
	var sb strings.Builder
	sb.WriteString("name: " + name + "\n")
	sb.WriteString("version: \"1.0.0\"\n")
	sb.WriteString("description: \"\"\n")
	sb.WriteString("capabilities:\n  - general\n")
	return sb.String()
}

// extractH1 returns the first H1 heading from markdown text, lowercased and hyphenated.
func extractH1(md string) string {
	scanner := bufio.NewScanner(strings.NewReader(md))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(line, "# "), " ", "-"))
		}
	}
	return ""
}

// generatedFiles lists filenames excluded from export by default.
var generatedFiles = map[string]bool{
	"metrics.md":   true,
	"CHANGELOG.md": true,
}

// ImportAgent copies an agent definition from srcPath into destDir/<name>/.
// If srcPath is a directory, all its files are copied.
// If srcPath is a single .md file, an agent directory is created with that file as AGENTS.md.
// nameOverride renames the resulting agent directory; otherwise the name is inferred.
// Returns the imported definition, any auto-fix suggestions, and any error.
func ImportAgent(srcPath, destDir, nameOverride string) (*AgentDefinition, []string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, nil, fmt.Errorf("source path %q not found: %w", srcPath, err)
	}

	var agentsDir string

	if info.IsDir() {
		def, err := LoadFromDirectory(srcPath)
		if err != nil {
			return nil, nil, err
		}
		name := nameOverride
		if name == "" {
			name = def.Name
		}
		agentsDir = filepath.Join(destDir, name)
		if err := copyDir(srcPath, agentsDir); err != nil {
			return nil, nil, fmt.Errorf("copy failed: %w", err)
		}
	} else {
		b, err := os.ReadFile(srcPath)
		if err != nil {
			return nil, nil, err
		}
		name := nameOverride
		if name == "" {
			name = extractH1(string(b))
		}
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
		}
		agentsDir = filepath.Join(destDir, name)
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			return nil, nil, err
		}
		if err := os.WriteFile(filepath.Join(agentsDir, "AGENTS.md"), b, 0644); err != nil {
			return nil, nil, err
		}
	}

	def, err := LoadFromDirectory(agentsDir)
	if err != nil {
		return nil, nil, err
	}

	// If a name override was given, keep it as the logical identity of this import.
	if nameOverride != "" {
		def.Name = nameOverride
	}

	var suggestions []string
	yamlPath := filepath.Join(agentsDir, "agent.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		generated := AutoGenerateYAML(def.AgentsMD, def.Name)
		if writeErr := os.WriteFile(yamlPath, []byte(generated), 0644); writeErr == nil {
			suggestions = append(suggestions, "Generated skeleton agent.yaml — fill in description and capabilities")
		}
		// Re-load to pick up the generated yaml, preserving any name override.
		if reloaded, rerr := LoadFromDirectory(agentsDir); rerr == nil {
			def = reloaded
			if nameOverride != "" {
				def.Name = nameOverride
			}
		}
	} else {
		suggestions = append(suggestions, SuggestFixes(def)...)
	}

	return def, suggestions, nil
}

// ExportAgent copies an agent directory to destPath.
// Generated files (metrics.md, CHANGELOG.md) are excluded unless includeGenerated is true.
func ExportAgent(def *AgentDefinition, destPath string, includeGenerated bool) error {
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(def.SourcePath)
	if err != nil {
		return fmt.Errorf("cannot read agent source: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !includeGenerated && generatedFiles[name] {
			continue
		}
		if err := copyFile(filepath.Join(def.SourcePath, name), filepath.Join(destPath, name)); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}
	return nil
}

// PromoteAgent copies a project-scoped agent to globalDir/<name>/.
// Returns an error if the agent already exists in global scope and force is false.
func PromoteAgent(def *AgentDefinition, globalDir string, force bool) error {
	if def.Scope != "project" {
		return fmt.Errorf("agent %q is not in project scope (current scope: %s)", def.Name, def.Scope)
	}
	destPath := filepath.Join(globalDir, def.Name)
	if _, err := os.Stat(destPath); err == nil && !force {
		return fmt.Errorf("agent %q already exists in global scope; use --force to overwrite", def.Name)
	}
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}
	return copyDir(def.SourcePath, destPath)
}

// DemoteAgent copies a global-scoped agent to projectDir/<name>/.
// Returns an error if the agent already exists in project scope and force is false.
func DemoteAgent(def *AgentDefinition, projectDir string, force bool) error {
	if def.Scope != "global" {
		return fmt.Errorf("agent %q is not in global scope (current scope: %s)", def.Name, def.Scope)
	}
	destPath := filepath.Join(projectDir, def.Name)
	if _, err := os.Stat(destPath); err == nil && !force {
		return fmt.Errorf("agent %q already exists in project scope; use --force to overwrite", def.Name)
	}
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}
	return copyDir(def.SourcePath, destPath)
}

// copyDir copies all files (non-recursive) from src to dst.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}
