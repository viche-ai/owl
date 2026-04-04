package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type ProjectConfig struct {
	Version    string   `json:"version,omitempty"`
	Context    string   `json:"context,omitempty"`
	Guardrails []string `json:"guardrails,omitempty"`
}

var projectConfigCache = make(map[string]*ProjectConfig)
var projectConfigMu sync.RWMutex

func LoadProjectConfig(workDir string) (*ProjectConfig, error) {
	projectConfigMu.RLock()
	if cached, ok := projectConfigCache[workDir]; ok {
		projectConfigMu.RUnlock()
		return cached, nil
	}
	projectConfigMu.RUnlock()

	dir := workDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	for {
		configPath := filepath.Join(dir, ".owl", "project.json")
		b, err := os.ReadFile(configPath)
		if err == nil {
			var cfg ProjectConfig
			if err := json.Unmarshal(b, &cfg); err == nil {
				projectConfigMu.Lock()
				projectConfigCache[workDir] = &cfg
				projectConfigMu.Unlock()
				return &cfg, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return &ProjectConfig{}, nil
}

func ClearProjectConfigCache() {
	projectConfigMu.Lock()
	projectConfigCache = make(map[string]*ProjectConfig)
	projectConfigMu.Unlock()
}
