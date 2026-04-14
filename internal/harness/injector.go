package harness

import (
	"fmt"
	"os"
	"strings"

	"github.com/viche-ai/owl/internal/agents"
)

// InjectAgentContext applies the agent definition to the harness environment
// using the strategy specified in the harness definition's ContextInjection config.
//
// Returns:
//   - modifiedDesc: the (possibly modified) description string
//   - envAdd: extra KEY=VALUE pairs to add to the subprocess environment
//   - cleanup: function to call after harness exits (restores backed-up files, etc.)
//   - err: if injection fails
//
// If agentDef is nil, this is a no-op.
func InjectAgentContext(
	harnessDef *HarnessDefinition,
	agentDef *agents.AgentDefinition,
	workDir string,
	description string,
) (modifiedDesc string, envAdd []string, cleanup func(), err error) {
	cleanup = func() {} // default no-op
	modifiedDesc = description

	if agentDef == nil {
		return
	}

	content := buildAgentContent(agentDef)
	if content == "" {
		return
	}

	cfg := harnessDef.ContextInjection
	if cfg == nil {
		// Default fallback: prepend to description
		cfg = &ContextInjectionConfig{Method: "arg-prepend"}
	}

	switch cfg.Method {
	case "file":
		path := expandPath(cfg.Path, workDir)
		if path == "" {
			err = fmt.Errorf("context_injection.path is required for method 'file'")
			return
		}
		cleanup, err = injectFile(path, content)
		return

	case "env":
		envVar := cfg.EnvVar
		if envVar == "" {
			envVar = "OWL_AGENT_PROMPT"
		}
		envAdd = []string{envVar + "=" + content}
		return

	case "arg-prepend":
		modifiedDesc = content + "\n\n" + description
		return

	default:
		err = fmt.Errorf("unknown context_injection method %q", cfg.Method)
		return
	}
}

// injectFile writes content to path, backing up any existing file.
// Returns a cleanup function that restores the original state.
func injectFile(path, content string) (func(), error) {
	backupPath := path + ".owl-backup"
	hadExisting := false

	// Back up existing file if present
	if _, err := os.Stat(path); err == nil {
		hadExisting = true
		if err := os.Rename(path, backupPath); err != nil {
			return func() {}, fmt.Errorf("backing up %s: %w", path, err)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		// Attempt to restore backup on write failure
		if hadExisting {
			_ = os.Rename(backupPath, path)
		}
		return func() {}, fmt.Errorf("writing %s: %w", path, err)
	}

	cleanup := func() {
		_ = os.Remove(path)
		if hadExisting {
			_ = os.Rename(backupPath, path)
		}
	}

	return cleanup, nil
}

// buildAgentContent concatenates the agent definition's prompt files.
func buildAgentContent(def *agents.AgentDefinition) string {
	var parts []string
	if def.AgentsMD != "" {
		parts = append(parts, def.AgentsMD)
	}
	if def.RoleMD != "" {
		parts = append(parts, def.RoleMD)
	}
	if def.GuardrailsMD != "" {
		parts = append(parts, def.GuardrailsMD)
	}
	return strings.Join(parts, "\n\n")
}

func expandPath(tmpl, workDir string) string {
	return strings.ReplaceAll(tmpl, "{{workdir}}", workDir)
}
