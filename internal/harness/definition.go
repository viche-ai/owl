package harness

// HarnessDefinition describes how to invoke an external agent harness.
// Built-in definitions are constructed in Go; users can add custom ones
// via YAML files in ~/.owl/harnesses/<name>.yaml.
type HarnessDefinition struct {
	Name          string            `yaml:"name"`
	Binary        string            `yaml:"binary"`
	Args          []string          `yaml:"args"`
	Env           map[string]string `yaml:"env,omitempty"`
	SupportsStdin bool              `yaml:"supports_stdin"`
	Persistent    bool              `yaml:"persistent"`
	OutputFormat  string            `yaml:"output_format"`
	WorkDirFlag   string            `yaml:"workdir_flag,omitempty"`
	Description   string            `yaml:"description,omitempty"`
	Aliases       []string          `yaml:"aliases,omitempty"`

	ContextInjection *ContextInjectionConfig `yaml:"context_injection,omitempty"`
}

// ContextInjectionConfig controls how Owl injects agent definition content
// into the harness environment.
type ContextInjectionConfig struct {
	// Method is "file", "env", or "arg-prepend".
	//   file:        write content to Path (supports {{workdir}} template)
	//   env:         set EnvVar to the content
	//   arg-prepend: prepend content to the description argument
	Method string `yaml:"method"`
	Path   string `yaml:"path,omitempty"`
	EnvVar string `yaml:"env_var,omitempty"`
}
