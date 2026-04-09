package agents

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AgentYAML is the schema for the optional agent.yaml sidecar file.
type AgentYAML struct {
	Name              string   `yaml:"name"`
	Version           string   `yaml:"version"`
	Description       string   `yaml:"description"`
	Capabilities      []string `yaml:"capabilities"`
	AllowedWorkspaces []string `yaml:"allowed_workspaces"`
	DefaultModel      string   `yaml:"default_model"`
	PromptLayers      []string `yaml:"prompt_layers"`
	Owner             string   `yaml:"owner"`
	Tags              []string `yaml:"tags"`
}

// AgentDefinition represents a fully resolved agent identity.
type AgentDefinition struct {
	Name              string
	Version           string
	Description       string
	Capabilities      []string
	AllowedWorkspaces []string
	DefaultModel      string
	PromptLayers      []string
	Owner             string
	Tags              []string

	// Resolved prompt content
	AgentsMD     string // Content of AGENTS.md (required)
	RoleMD       string // Content of role.md (optional)
	GuardrailsMD string // Content of guardrails.md (optional)

	// Source tracking
	Scope      string // "project" or "global"
	SourcePath string // Absolute path to agent directory
}

// LoadFromDirectory reads an agent definition from a directory.
// AGENTS.md is required; agent.yaml, role.md, and guardrails.md are optional.
func LoadFromDirectory(dirPath string) (*AgentDefinition, error) {
	agentsMD, err := os.ReadFile(filepath.Join(dirPath, "AGENTS.md"))
	if err != nil {
		return nil, fmt.Errorf("AGENTS.md not found in %s: %w", dirPath, err)
	}

	def := &AgentDefinition{
		AgentsMD:   string(agentsMD),
		SourcePath: dirPath,
	}

	// Parse agent.yaml if present
	if yamlBytes, err := os.ReadFile(filepath.Join(dirPath, "agent.yaml")); err == nil {
		var meta AgentYAML
		if err := yaml.Unmarshal(yamlBytes, &meta); err == nil {
			def.Name = meta.Name
			def.Version = meta.Version
			def.Description = meta.Description
			def.Capabilities = meta.Capabilities
			def.AllowedWorkspaces = meta.AllowedWorkspaces
			def.DefaultModel = meta.DefaultModel
			def.PromptLayers = meta.PromptLayers
			def.Owner = meta.Owner
			def.Tags = meta.Tags
		}
	}

	// Fall back to directory name if agent.yaml provided no name
	if def.Name == "" {
		def.Name = filepath.Base(dirPath)
	}

	// Optional supplementary files
	if b, err := os.ReadFile(filepath.Join(dirPath, "role.md")); err == nil {
		def.RoleMD = string(b)
	}
	if b, err := os.ReadFile(filepath.Join(dirPath, "guardrails.md")); err == nil {
		def.GuardrailsMD = string(b)
	}

	return def, nil
}
