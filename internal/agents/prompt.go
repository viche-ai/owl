package agents

import (
	"fmt"
	"strings"

	"github.com/viche-ai/owl/internal/config"
)

// PromptLayer is a single tier in the 5-layer prompt resolution stack.
type PromptLayer struct {
	Name     string // e.g., "owl-defaults", "global-agent", "project-policy", "project-agent", "runtime-override"
	Source   string // absolute file path, or "config" / "cli-flag"
	Content  string
	Priority int // 1 = highest, 5 = lowest
}

// PromptStack holds the fully resolved prompt with source attribution.
type PromptStack struct {
	Layers []PromptLayer
}

// BuildPromptStack resolves the full prompt for a given agent and runtime overrides.
//
// 5-tier priority (1 = highest, applied last / overrides lower tiers):
//
//	5  owl-defaults    — globalCfg.SystemPrompt or config.DefaultSystemPrompt()
//	4  global-agent    — def (scope == "global"): AGENTS.md + role.md + guardrails.md
//	3  project-policy  — projectCfg.Context + projectCfg.Guardrails
//	2  project-agent   — def (scope == "project"): AGENTS.md + role.md + guardrails.md
//	1  runtime-override — runtimePrompt from --prompt / --from-file
func BuildPromptStack(
	def *AgentDefinition,
	globalCfg *config.Config,
	projectCfg *config.ProjectConfig,
	runtimePrompt string,
) *PromptStack {
	var layers []PromptLayer

	// Layer 5: Owl system defaults (lowest priority)
	owlDefault := ""
	if globalCfg != nil {
		owlDefault = globalCfg.SystemPrompt
	}
	if owlDefault == "" {
		owlDefault = config.DefaultSystemPrompt()
	}
	layers = append(layers, PromptLayer{
		Name:     "owl-defaults",
		Source:   "config",
		Content:  owlDefault,
		Priority: 5,
	})

	// Layer 4: Global agent base
	if def != nil && def.Scope == "global" {
		layers = append(layers, PromptLayer{
			Name:     "global-agent",
			Source:   def.SourcePath,
			Content:  buildAgentContent(def),
			Priority: 4,
		})
	}

	// Layer 3: Project shared policy
	if projectCfg != nil {
		var parts []string
		if projectCfg.Context != "" {
			parts = append(parts, projectCfg.Context)
		}
		if len(projectCfg.Guardrails) > 0 {
			var gb strings.Builder
			gb.WriteString("CRITICAL GUARDRAILS:\n")
			for _, g := range projectCfg.Guardrails {
				gb.WriteString("- " + g + "\n")
			}
			parts = append(parts, gb.String())
		}
		if len(parts) > 0 {
			layers = append(layers, PromptLayer{
				Name:     "project-policy",
				Source:   ".owl/project.json",
				Content:  strings.Join(parts, "\n\n"),
				Priority: 3,
			})
		}
	}

	// Layer 2: Project agent overlays
	if def != nil && def.Scope == "project" {
		layers = append(layers, PromptLayer{
			Name:     "project-agent",
			Source:   def.SourcePath,
			Content:  buildAgentContent(def),
			Priority: 2,
		})
	}

	// Layer 1: Runtime override (highest priority)
	if runtimePrompt != "" {
		layers = append(layers, PromptLayer{
			Name:     "runtime-override",
			Source:   "cli-flag",
			Content:  runtimePrompt,
			Priority: 1,
		})
	}

	return &PromptStack{Layers: layers}
}

// buildAgentContent concatenates AGENTS.md + role.md + guardrails.md for an agent definition.
func buildAgentContent(def *AgentDefinition) string {
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

// Render produces the final system prompt by concatenating layers from lowest to highest
// priority (5 → 1), so higher-priority content appears later in the prompt.
func (ps *PromptStack) Render() string {
	sorted := sortedLayers(ps.Layers, false) // false = descending priority (5 first)
	var sb strings.Builder
	for _, layer := range sorted {
		if layer.Content == "" {
			continue
		}
		fmt.Fprintf(&sb, "[%s]\n", strings.ToUpper(layer.Name))
		sb.WriteString(layer.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Explain produces a human-readable breakdown of each layer and its source,
// ordered from highest to lowest priority (1 → 5).
func (ps *PromptStack) Explain() string {
	sorted := sortedLayers(ps.Layers, true) // true = ascending priority (1 first)
	var sb strings.Builder
	sb.WriteString("Prompt stack (highest → lowest priority):\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n")
	for _, layer := range sorted {
		preview := strings.ReplaceAll(layer.Content, "\n", " ")
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		fmt.Fprintf(&sb, "  [%d] %-20s  source: %s\n", layer.Priority, layer.Name, layer.Source)
		if preview != "" {
			fmt.Fprintf(&sb, "      %s\n", preview)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// sortedLayers returns a copy of layers sorted by priority.
// ascending=true → priority 1 first; ascending=false → priority 5 first.
func sortedLayers(layers []PromptLayer, ascending bool) []PromptLayer {
	out := make([]PromptLayer, len(layers))
	copy(out, layers)
	// Insertion sort (N ≤ 5)
	for i := 1; i < len(out); i++ {
		key := out[i]
		j := i - 1
		for j >= 0 {
			var less bool
			if ascending {
				less = out[j].Priority > key.Priority
			} else {
				less = out[j].Priority < key.Priority
			}
			if !less {
				break
			}
			out[j+1] = out[j]
			j--
		}
		out[j+1] = key
	}
	return out
}
