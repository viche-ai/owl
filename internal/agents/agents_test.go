package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viche-ai/owl/internal/config"
)

// helpers

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// --- LoadFromDirectory ---

func TestLoadFromDirectory_RequiresAgentsMD(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFromDirectory(dir)
	if err == nil {
		t.Fatal("expected error when AGENTS.md is missing")
	}
}

func TestLoadFromDirectory_AgentsMDOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# My Agent\nDoes stuff.")

	def, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if def.AgentsMD != "# My Agent\nDoes stuff." {
		t.Errorf("unexpected AgentsMD: %q", def.AgentsMD)
	}
	// Name falls back to directory base name
	if def.Name != filepath.Base(dir) {
		t.Errorf("expected name %q, got %q", filepath.Base(dir), def.Name)
	}
	if def.SourcePath != dir {
		t.Errorf("unexpected SourcePath: %q", def.SourcePath)
	}
}

func TestLoadFromDirectory_ParsesAgentYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Reviewer")
	writeFile(t, filepath.Join(dir, "agent.yaml"), `name: reviewer
version: "1.0"
description: Reviews code
capabilities:
  - code-review
  - linting
default_model: anthropic/claude-sonnet-4-6
owner: team-a
tags:
  - quality
`)

	def, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "reviewer" {
		t.Errorf("name: got %q", def.Name)
	}
	if def.Version != "1.0" {
		t.Errorf("version: got %q", def.Version)
	}
	if def.Description != "Reviews code" {
		t.Errorf("description: got %q", def.Description)
	}
	if len(def.Capabilities) != 2 || def.Capabilities[0] != "code-review" {
		t.Errorf("capabilities: got %v", def.Capabilities)
	}
	if def.DefaultModel != "anthropic/claude-sonnet-4-6" {
		t.Errorf("default_model: got %q", def.DefaultModel)
	}
}

func TestLoadFromDirectory_OptionalFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Agent")
	writeFile(t, filepath.Join(dir, "role.md"), "You are a senior engineer.")
	writeFile(t, filepath.Join(dir, "guardrails.md"), "Never delete prod data.")

	def, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if def.RoleMD != "You are a senior engineer." {
		t.Errorf("RoleMD: got %q", def.RoleMD)
	}
	if def.GuardrailsMD != "Never delete prod data." {
		t.Errorf("GuardrailsMD: got %q", def.GuardrailsMD)
	}
}

// --- Resolver ---

func makeResolver(t *testing.T) (r *Resolver, projectAgentsDir, globalAgentsDir string) {
	t.Helper()
	tmp := t.TempDir()
	projectAgentsDir = filepath.Join(tmp, "project", ".owl", "agents")
	globalAgentsDir = filepath.Join(tmp, "global", ".owl", "agents")
	if err := os.MkdirAll(projectAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalAgentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	r = &Resolver{
		ProjectDir: projectAgentsDir,
		GlobalDir:  globalAgentsDir,
	}
	return
}

func TestResolver_ProjectScopePrecedence(t *testing.T) {
	r, projectDir, globalDir := makeResolver(t)
	writeFile(t, filepath.Join(projectDir, "reviewer", "AGENTS.md"), "# Project Reviewer")
	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Global Reviewer")

	def, err := r.Resolve("reviewer", "")
	if err != nil {
		t.Fatal(err)
	}
	if def.Scope != "project" {
		t.Errorf("expected project scope, got %q", def.Scope)
	}
	if !strings.Contains(def.AgentsMD, "Project Reviewer") {
		t.Errorf("expected project content, got %q", def.AgentsMD)
	}
}

func TestResolver_FallsBackToGlobal(t *testing.T) {
	r, _, globalDir := makeResolver(t)
	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Global Reviewer")

	def, err := r.Resolve("reviewer", "")
	if err != nil {
		t.Fatal(err)
	}
	if def.Scope != "global" {
		t.Errorf("expected global scope, got %q", def.Scope)
	}
}

func TestResolver_RespectsScopeHintProject(t *testing.T) {
	r, _, globalDir := makeResolver(t)
	// Only exists in global — resolving with "project" hint must fail
	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Global Reviewer")

	_, err := r.Resolve("reviewer", "project")
	if err == nil {
		t.Fatal("expected error when agent not in project scope")
	}
}

func TestResolver_RespectsScopeHintGlobal(t *testing.T) {
	r, projectDir, globalDir := makeResolver(t)
	writeFile(t, filepath.Join(projectDir, "reviewer", "AGENTS.md"), "# Project Reviewer")
	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Global Reviewer")

	def, err := r.Resolve("reviewer", "global")
	if err != nil {
		t.Fatal(err)
	}
	if def.Scope != "global" {
		t.Errorf("expected global scope, got %q", def.Scope)
	}
}

func TestResolver_NotFound(t *testing.T) {
	r, _, _ := makeResolver(t)
	_, err := r.Resolve("nonexistent", "")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

// --- Validate ---

func TestValidate_MissingAgentsMD(t *testing.T) {
	r := &Resolver{}
	def := &AgentDefinition{AgentsMD: "", Capabilities: []string{"x"}}
	errs := r.Validate(def, false)
	if len(errs) == 0 {
		t.Fatal("expected validation error for empty AGENTS.md")
	}
}

func TestValidate_EmptyCapabilities(t *testing.T) {
	r := &Resolver{}
	def := &AgentDefinition{AgentsMD: "# Agent", Capabilities: nil}
	errs := r.Validate(def, false)
	found := false
	for _, e := range errs {
		if e.Field == "capabilities" {
			found = true
		}
	}
	if !found {
		t.Error("expected capabilities validation error")
	}
}

func TestValidate_StrictModeRequiresFields(t *testing.T) {
	r := &Resolver{}
	def := &AgentDefinition{
		AgentsMD:     "# Agent",
		Capabilities: []string{"x"},
		// Version and Description empty
	}
	errs := r.Validate(def, true)
	if len(errs) == 0 {
		t.Fatal("expected strict mode validation errors")
	}
	fields := make(map[string]bool)
	for _, e := range errs {
		fields[e.Field] = true
	}
	if !fields["agent.yaml:version"] {
		t.Error("expected version error in strict mode")
	}
	if !fields["agent.yaml:description"] {
		t.Error("expected description error in strict mode")
	}
}

func TestValidate_Valid(t *testing.T) {
	r := &Resolver{}
	def := &AgentDefinition{
		AgentsMD:     "# Agent",
		Capabilities: []string{"code-review"},
		Version:      "1.0",
		Description:  "A reviewer",
	}
	errs := r.Validate(def, true)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

// --- BuildPromptStack / Render / Explain ---

func TestBuildPromptStack_FiveLayers(t *testing.T) {
	globalCfg := &config.Config{SystemPrompt: "Global system prompt"}
	projectCfg := &config.ProjectConfig{
		Context:    "Project context",
		Guardrails: []string{"no delete"},
	}
	def := &AgentDefinition{
		AgentsMD:   "# Reviewer agent",
		Scope:      "project",
		SourcePath: "/some/path",
	}

	stack := BuildPromptStack(def, globalCfg, projectCfg, "runtime override")

	if len(stack.Layers) != 4 {
		t.Errorf("expected 4 layers (owl-defaults, project-policy, project-agent, runtime-override), got %d", len(stack.Layers))
	}

	priorities := make(map[int]string)
	for _, l := range stack.Layers {
		priorities[l.Priority] = l.Name
	}
	if priorities[5] != "owl-defaults" {
		t.Errorf("layer 5 should be owl-defaults, got %q", priorities[5])
	}
	if priorities[3] != "project-policy" {
		t.Errorf("layer 3 should be project-policy, got %q", priorities[3])
	}
	if priorities[2] != "project-agent" {
		t.Errorf("layer 2 should be project-agent, got %q", priorities[2])
	}
	if priorities[1] != "runtime-override" {
		t.Errorf("layer 1 should be runtime-override, got %q", priorities[1])
	}
}

func TestBuildPromptStack_GlobalAgentLayer(t *testing.T) {
	globalCfg := &config.Config{}
	def := &AgentDefinition{
		AgentsMD:   "# Global agent",
		Scope:      "global",
		SourcePath: "/global/path",
	}
	stack := BuildPromptStack(def, globalCfg, nil, "")

	found := false
	for _, l := range stack.Layers {
		if l.Name == "global-agent" {
			found = true
			if l.Priority != 4 {
				t.Errorf("global-agent priority: want 4, got %d", l.Priority)
			}
		}
	}
	if !found {
		t.Error("expected global-agent layer")
	}
}

func TestBuildPromptStack_NoDef(t *testing.T) {
	// No agent definition — should still produce defaults + project policy
	globalCfg := &config.Config{SystemPrompt: "default"}
	projectCfg := &config.ProjectConfig{Context: "ctx"}
	stack := BuildPromptStack(nil, globalCfg, projectCfg, "")

	if len(stack.Layers) != 2 {
		t.Errorf("expected 2 layers, got %d", len(stack.Layers))
	}
}

func TestRender_OrderAndHeaders(t *testing.T) {
	globalCfg := &config.Config{SystemPrompt: "defaults"}
	def := &AgentDefinition{AgentsMD: "agent content", Scope: "project", SourcePath: "/p"}
	stack := BuildPromptStack(def, globalCfg, nil, "")

	rendered := stack.Render()
	// owl-defaults should appear before project-agent (lower priority rendered first)
	defaultsIdx := strings.Index(rendered, "[OWL-DEFAULTS]")
	agentIdx := strings.Index(rendered, "[PROJECT-AGENT]")
	if defaultsIdx == -1 || agentIdx == -1 {
		t.Fatalf("missing expected section headers in rendered output:\n%s", rendered)
	}
	if defaultsIdx > agentIdx {
		t.Error("owl-defaults should appear before project-agent in rendered output")
	}
}

func TestExplain_ContainsSourcePaths(t *testing.T) {
	globalCfg := &config.Config{SystemPrompt: "defaults"}
	def := &AgentDefinition{
		AgentsMD:   "# Agent",
		Scope:      "project",
		SourcePath: "/my/agent/dir",
	}
	stack := BuildPromptStack(def, globalCfg, nil, "")
	explain := stack.Explain()

	if !strings.Contains(explain, "/my/agent/dir") {
		t.Errorf("Explain() should include source path, got:\n%s", explain)
	}
	if !strings.Contains(explain, "config") {
		t.Errorf("Explain() should include 'config' source for owl-defaults, got:\n%s", explain)
	}
}
