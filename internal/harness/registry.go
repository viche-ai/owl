package harness

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry holds harness definitions and resolves them by name or alias.
type Registry struct {
	defs    map[string]*HarnessDefinition
	aliases map[string]string // alias -> canonical name
}

// NewRegistry creates a registry pre-populated with built-in harness definitions.
func NewRegistry() *Registry {
	r := &Registry{
		defs:    make(map[string]*HarnessDefinition),
		aliases: make(map[string]string),
	}
	r.loadBuiltins()
	return r
}

// LoadUserDir reads YAML harness definitions from ~/.owl/harnesses/.
// User definitions override built-ins by name.
func (r *Registry) LoadUserDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // no home dir, skip silently
	}
	dir := filepath.Join(home, ".owl", "harnesses")
	return r.LoadFromDir(dir)
}

// LoadFromDir reads all .yaml/.yml files from dir and registers them.
func (r *Registry) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading harness dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var def HarnessDefinition
		if err := yaml.Unmarshal(b, &def); err != nil {
			continue
		}

		// Use filename (without ext) as name if YAML doesn't specify one
		if def.Name == "" {
			def.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}

		if def.Binary == "" {
			log.Printf("Warning: harness %q in %s has no binary configured, skipping", def.Name, path)
			continue
		}

		r.register(&def)
	}

	return nil
}

// Resolve looks up a harness by name or alias.
func (r *Registry) Resolve(name string) (*HarnessDefinition, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, fmt.Errorf("harness name is required")
	}

	if def, ok := r.defs[name]; ok {
		return def, nil
	}
	if canonical, ok := r.aliases[name]; ok {
		if def, ok := r.defs[canonical]; ok {
			return def, nil
		}
	}

	available := r.listNames()
	return nil, fmt.Errorf("unknown harness %q (available: %s)", name, strings.Join(available, ", "))
}

// List returns all registered definitions, sorted by name.
func (r *Registry) List() []*HarnessDefinition {
	out := make([]*HarnessDefinition, 0, len(r.defs))
	for _, def := range r.defs {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) register(def *HarnessDefinition) {
	r.defs[def.Name] = def
	for _, alias := range def.Aliases {
		r.aliases[strings.ToLower(alias)] = def.Name
	}
}

func (r *Registry) listNames() []string {
	names := make([]string, 0, len(r.defs))
	for name := range r.defs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) loadBuiltins() {
	r.register(&HarnessDefinition{
		Name:        "codex",
		Binary:      "codex",
		Args:        []string{"exec", "{{description}}"},
		Description: "OpenAI Codex CLI",
		ContextInjection: &ContextInjectionConfig{
			Method: "arg-prepend",
		},
	})

	r.register(&HarnessDefinition{
		Name:        "opencode",
		Binary:      "opencode",
		Args:        []string{"run", "{{description}}"},
		WorkDirFlag: "--dir",
		Description: "OpenCode CLI",
		ContextInjection: &ContextInjectionConfig{
			Method: "arg-prepend",
		},
	})

	r.register(&HarnessDefinition{
		Name:         "claude-code",
		Binary:       "claude",
		Args:         []string{"-p", "--verbose", "--output-format", "stream-json", "{{description}}"},
		Aliases:      []string{"claude"},
		Persistent:   true,
		OutputFormat: "claude-stream-json",
		Description:  "Anthropic Claude Code CLI",
		ContextInjection: &ContextInjectionConfig{
			Method: "file",
			Path:   "{{workdir}}/CLAUDE.md",
		},
	})
}
