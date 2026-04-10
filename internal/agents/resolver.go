package agents

import (
	"fmt"
	"os"
	"path/filepath"
)

// ValidationError describes a validation failure for an agent definition.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Resolver handles agent definition lookup with scope precedence.
type Resolver struct {
	ProjectDir string // <project_root>/.owl/agents/  (may be empty if no project root found)
	GlobalDir  string // ~/.owl/agents/
}

// NewResolver creates a Resolver anchored to workDir.
// It walks up from workDir to find a directory containing .owl/, setting that
// as the project root. Falls back to workDir itself if none is found.
func NewResolver(workDir string) *Resolver {
	home, _ := os.UserHomeDir()

	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	projectDir := ""
	dir := workDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".owl")); err == nil {
			projectDir = filepath.Join(dir, ".owl", "agents")
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return &Resolver{
		ProjectDir: projectDir,
		GlobalDir:  filepath.Join(home, ".owl", "agents"),
	}
}

// Resolve finds an agent definition by name, applying scope precedence.
// scopeHint restricts resolution to "project" or "global"; "" searches both (project first).
func (r *Resolver) Resolve(name string, scopeHint string) (*AgentDefinition, error) {
	type candidate struct {
		scope string
		dir   string
	}

	var candidates []candidate
	if (scopeHint == "" || scopeHint == "project") && r.ProjectDir != "" {
		candidates = append(candidates, candidate{"project", filepath.Join(r.ProjectDir, name)})
	}
	if scopeHint == "" || scopeHint == "global" {
		candidates = append(candidates, candidate{"global", filepath.Join(r.GlobalDir, name)})
	}

	for _, c := range candidates {
		def, err := LoadFromDirectory(c.dir)
		if err == nil {
			def.Scope = c.scope
			return def, nil
		}
	}

	if scopeHint != "" {
		return nil, fmt.Errorf("agent %q not found in %s scope", name, scopeHint)
	}
	return nil, fmt.Errorf("agent %q not found (searched project and global scopes)", name)
}

// List returns all available agent definitions, optionally filtered by scope.
// Project-scope agents take precedence: an agent with the same name in both scopes
// appears only once (project scope wins).
func (r *Resolver) List(scopeFilter string) ([]AgentDefinition, error) {
	type scopeDir struct {
		scope string
		dir   string
	}

	var dirs []scopeDir
	if (scopeFilter == "" || scopeFilter == "project") && r.ProjectDir != "" {
		dirs = append(dirs, scopeDir{"project", r.ProjectDir})
	}
	if scopeFilter == "" || scopeFilter == "global" {
		dirs = append(dirs, scopeDir{"global", r.GlobalDir})
	}

	seen := make(map[string]bool)
	var results []AgentDefinition

	for _, sd := range dirs {
		entries, err := os.ReadDir(sd.dir)
		if err != nil {
			continue // directory may not exist yet
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if seen[name] {
				continue
			}
			seen[name] = true

			def, err := LoadFromDirectory(filepath.Join(sd.dir, name))
			if err != nil {
				continue
			}
			def.Scope = sd.scope
			results = append(results, *def)
		}
	}
	return results, nil
}

// Validate checks an agent definition for required fields.
// In strict mode, agent.yaml with name, version, and description is required.
func (r *Resolver) Validate(def *AgentDefinition, strict bool) []ValidationError {
	var errs []ValidationError

	if def.AgentsMD == "" {
		errs = append(errs, ValidationError{Field: "AGENTS.md", Message: "must not be empty"})
	}

	if strict {
		if def.Version == "" {
			errs = append(errs, ValidationError{Field: "agent.yaml:version", Message: "required in strict mode"})
		}
		if def.Description == "" {
			errs = append(errs, ValidationError{Field: "agent.yaml:description", Message: "required in strict mode"})
		}
	}

	if len(def.Capabilities) == 0 {
		errs = append(errs, ValidationError{Field: "capabilities", Message: "at least one capability is required"})
	}

	return errs
}
