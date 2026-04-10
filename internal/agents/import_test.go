package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── ValidateImportPath ────────────────────────────────────────────────────────

func TestValidateImportPath_DirectoryWithAgentsMD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Agent")
	if err := ValidateImportPath(dir); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateImportPath_DirectoryMissingAgentsMD(t *testing.T) {
	dir := t.TempDir()
	if err := ValidateImportPath(dir); err == nil {
		t.Error("expected error for directory missing AGENTS.md")
	}
}

func TestValidateImportPath_MarkdownFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reviewer.md")
	writeFile(t, path, "# Reviewer")
	if err := ValidateImportPath(path); err != nil {
		t.Errorf("expected no error for .md file, got: %v", err)
	}
}

func TestValidateImportPath_NonMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.txt")
	writeFile(t, path, "# Agent")
	if err := ValidateImportPath(path); err == nil {
		t.Error("expected error for non-.md file")
	}
}

func TestValidateImportPath_NotFound(t *testing.T) {
	err := ValidateImportPath("/nonexistent/path/to/agent")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

// ── SuggestFixes ──────────────────────────────────────────────────────────────

func TestSuggestFixes_MissingAll(t *testing.T) {
	def := &AgentDefinition{AgentsMD: "# Agent"}
	suggestions := SuggestFixes(def)
	if len(suggestions) != 3 {
		t.Errorf("expected 3 suggestions, got %d: %v", len(suggestions), suggestions)
	}
}

func TestSuggestFixes_Complete(t *testing.T) {
	def := &AgentDefinition{
		AgentsMD:     "# Agent",
		Version:      "1.0.0",
		Description:  "Does stuff",
		Capabilities: []string{"general"},
	}
	suggestions := SuggestFixes(def)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for complete definition, got: %v", suggestions)
	}
}

func TestSuggestFixes_MissingCapabilities(t *testing.T) {
	def := &AgentDefinition{
		AgentsMD:    "# Agent",
		Version:     "1.0.0",
		Description: "Does stuff",
	}
	suggestions := SuggestFixes(def)
	found := false
	for _, s := range suggestions {
		if strings.Contains(s, "capabilities") {
			found = true
		}
	}
	if !found {
		t.Error("expected suggestion about capabilities")
	}
}

// ── AutoGenerateYAML ──────────────────────────────────────────────────────────

func TestAutoGenerateYAML_FromH1(t *testing.T) {
	yaml := AutoGenerateYAML("# My Reviewer\nDoes code review.", "")
	if !strings.Contains(yaml, "name: my-reviewer") {
		t.Errorf("expected name from H1, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "version: \"1.0.0\"") {
		t.Errorf("expected default version, got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "capabilities:") {
		t.Errorf("expected capabilities field, got:\n%s", yaml)
	}
}

func TestAutoGenerateYAML_NameOverride(t *testing.T) {
	yaml := AutoGenerateYAML("# Some Agent", "custom-name")
	if !strings.Contains(yaml, "name: custom-name") {
		t.Errorf("expected overridden name, got:\n%s", yaml)
	}
}

func TestAutoGenerateYAML_NoH1Fallback(t *testing.T) {
	yaml := AutoGenerateYAML("Just some content without a heading.", "")
	if !strings.Contains(yaml, "name: unnamed-agent") {
		t.Errorf("expected fallback name, got:\n%s", yaml)
	}
}

// ── ImportAgent ───────────────────────────────────────────────────────────────

func TestImportAgent_FromDirectory(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	writeFile(t, filepath.Join(src, "AGENTS.md"), "# Reviewer\nReviews code.")
	writeFile(t, filepath.Join(src, "agent.yaml"), "name: reviewer\nversion: \"1.0\"\ndescription: Reviews code\ncapabilities:\n  - code-review\n")

	dest := filepath.Join(tmp, "dest")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}

	def, _, err := ImportAgent(src, dest, "")
	if err != nil {
		t.Fatalf("ImportAgent failed: %v", err)
	}

	if def.Name != "reviewer" {
		t.Errorf("expected name 'reviewer', got %q", def.Name)
	}

	// Check the agent directory was created at dest/reviewer/
	if _, err := os.Stat(filepath.Join(dest, "reviewer", "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not found at destination")
	}
}

func TestImportAgent_FromSingleFile_GeneratesYAML(t *testing.T) {
	tmp := t.TempDir()
	srcFile := filepath.Join(tmp, "my-agent.md")
	writeFile(t, srcFile, "# My Agent\nDoes things.")

	dest := filepath.Join(tmp, "dest")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}

	def, suggestions, err := ImportAgent(srcFile, dest, "")
	if err != nil {
		t.Fatalf("ImportAgent failed: %v", err)
	}

	// AGENTS.md should be created in dest/<name>/
	agentsDir := filepath.Join(dest, def.Name)
	if _, err := os.Stat(filepath.Join(agentsDir, "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not found at destination")
	}

	// agent.yaml should have been generated
	if _, err := os.Stat(filepath.Join(agentsDir, "agent.yaml")); err != nil {
		t.Error("agent.yaml not generated from single-file import")
	}

	// A suggestion about the generated yaml should be returned
	foundSuggestion := false
	for _, s := range suggestions {
		if strings.Contains(s, "Generated") {
			foundSuggestion = true
		}
	}
	if !foundSuggestion {
		t.Errorf("expected suggestion about generated yaml, got: %v", suggestions)
	}
}

func TestImportAgent_NameOverride(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	writeFile(t, filepath.Join(src, "AGENTS.md"), "# Reviewer")

	dest := filepath.Join(tmp, "dest")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}

	def, _, err := ImportAgent(src, dest, "my-custom-reviewer")
	if err != nil {
		t.Fatalf("ImportAgent failed: %v", err)
	}

	if def.Name != "my-custom-reviewer" {
		t.Errorf("expected name 'my-custom-reviewer', got %q", def.Name)
	}
	if _, err := os.Stat(filepath.Join(dest, "my-custom-reviewer", "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not found under overridden name")
	}
}

// ── ExportAgent ───────────────────────────────────────────────────────────────

func TestExportAgent_ExcludesGeneratedByDefault(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "reviewer")
	writeFile(t, filepath.Join(src, "AGENTS.md"), "# Reviewer")
	writeFile(t, filepath.Join(src, "agent.yaml"), "name: reviewer\ncapabilities:\n  - review\n")
	writeFile(t, filepath.Join(src, "metrics.md"), "# Metrics")
	writeFile(t, filepath.Join(src, "CHANGELOG.md"), "# Changelog")

	def := &AgentDefinition{Name: "reviewer", SourcePath: src, Scope: "project"}

	dest := filepath.Join(tmp, "export-out")
	if err := ExportAgent(def, dest, false); err != nil {
		t.Fatalf("ExportAgent failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "AGENTS.md")); err != nil {
		t.Error("AGENTS.md should be in export")
	}
	if _, err := os.Stat(filepath.Join(dest, "agent.yaml")); err != nil {
		t.Error("agent.yaml should be in export")
	}
	if _, err := os.Stat(filepath.Join(dest, "metrics.md")); err == nil {
		t.Error("metrics.md should be excluded from default export")
	}
	if _, err := os.Stat(filepath.Join(dest, "CHANGELOG.md")); err == nil {
		t.Error("CHANGELOG.md should be excluded from default export")
	}
}

func TestExportAgent_IncludesGeneratedWhenFlagSet(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "reviewer")
	writeFile(t, filepath.Join(src, "AGENTS.md"), "# Reviewer")
	writeFile(t, filepath.Join(src, "metrics.md"), "# Metrics")

	def := &AgentDefinition{Name: "reviewer", SourcePath: src, Scope: "project"}

	dest := filepath.Join(tmp, "export-with-generated")
	if err := ExportAgent(def, dest, true); err != nil {
		t.Fatalf("ExportAgent failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "metrics.md")); err != nil {
		t.Error("metrics.md should be included when includeGenerated=true")
	}
}

// ── PromoteAgent ──────────────────────────────────────────────────────────────

func TestPromoteAgent_CopiesCorrectly(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project", ".owl", "agents")
	globalDir := filepath.Join(tmp, "global", ".owl", "agents")

	writeFile(t, filepath.Join(projectDir, "reviewer", "AGENTS.md"), "# Reviewer\nProject version.")
	writeFile(t, filepath.Join(projectDir, "reviewer", "agent.yaml"), "name: reviewer\ncapabilities:\n  - review\n")

	def := &AgentDefinition{
		Name:       "reviewer",
		Scope:      "project",
		SourcePath: filepath.Join(projectDir, "reviewer"),
	}

	if err := PromoteAgent(def, globalDir, false); err != nil {
		t.Fatalf("PromoteAgent failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(globalDir, "reviewer", "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not found in global scope after promote")
	}
	if _, err := os.Stat(filepath.Join(globalDir, "reviewer", "agent.yaml")); err != nil {
		t.Error("agent.yaml not found in global scope after promote")
	}
	// Source should still exist
	if _, err := os.Stat(filepath.Join(projectDir, "reviewer", "AGENTS.md")); err != nil {
		t.Error("source should still exist after promote (it's a copy, not a move)")
	}
}

func TestPromoteAgent_FailsWithoutForceIfExists(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project", ".owl", "agents")
	globalDir := filepath.Join(tmp, "global", ".owl", "agents")

	writeFile(t, filepath.Join(projectDir, "reviewer", "AGENTS.md"), "# Project Reviewer")
	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Global Reviewer")

	def := &AgentDefinition{
		Name:       "reviewer",
		Scope:      "project",
		SourcePath: filepath.Join(projectDir, "reviewer"),
	}

	if err := PromoteAgent(def, globalDir, false); err == nil {
		t.Error("expected error when agent already exists in global and force=false")
	}
}

func TestPromoteAgent_ForceOverwrites(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project", ".owl", "agents")
	globalDir := filepath.Join(tmp, "global", ".owl", "agents")

	writeFile(t, filepath.Join(projectDir, "reviewer", "AGENTS.md"), "# Updated Reviewer")
	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Old Global Reviewer")

	def := &AgentDefinition{
		Name:       "reviewer",
		Scope:      "project",
		SourcePath: filepath.Join(projectDir, "reviewer"),
	}

	if err := PromoteAgent(def, globalDir, true); err != nil {
		t.Fatalf("PromoteAgent with force failed: %v", err)
	}

	b, _ := os.ReadFile(filepath.Join(globalDir, "reviewer", "AGENTS.md"))
	if !strings.Contains(string(b), "Updated Reviewer") {
		t.Error("force overwrite should have updated global agent")
	}
}

func TestPromoteAgent_RejectsNonProjectScope(t *testing.T) {
	def := &AgentDefinition{Name: "reviewer", Scope: "global"}
	if err := PromoteAgent(def, "/any/dir", false); err == nil {
		t.Error("expected error when promoting a non-project-scoped agent")
	}
}

// ── DemoteAgent ───────────────────────────────────────────────────────────────

func TestDemoteAgent_CopiesCorrectly(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project", ".owl", "agents")
	globalDir := filepath.Join(tmp, "global", ".owl", "agents")

	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Reviewer\nGlobal version.")

	def := &AgentDefinition{
		Name:       "reviewer",
		Scope:      "global",
		SourcePath: filepath.Join(globalDir, "reviewer"),
	}

	if err := DemoteAgent(def, projectDir, false); err != nil {
		t.Fatalf("DemoteAgent failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, "reviewer", "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not found in project scope after demote")
	}
	// Source should still exist
	if _, err := os.Stat(filepath.Join(globalDir, "reviewer", "AGENTS.md")); err != nil {
		t.Error("source should still exist after demote (it's a copy, not a move)")
	}
}

func TestDemoteAgent_FailsWithoutForceIfExists(t *testing.T) {
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project", ".owl", "agents")
	globalDir := filepath.Join(tmp, "global", ".owl", "agents")

	writeFile(t, filepath.Join(globalDir, "reviewer", "AGENTS.md"), "# Global Reviewer")
	writeFile(t, filepath.Join(projectDir, "reviewer", "AGENTS.md"), "# Project Reviewer")

	def := &AgentDefinition{
		Name:       "reviewer",
		Scope:      "global",
		SourcePath: filepath.Join(globalDir, "reviewer"),
	}

	if err := DemoteAgent(def, projectDir, false); err == nil {
		t.Error("expected error when agent already exists in project and force=false")
	}
}

func TestDemoteAgent_RejectsNonGlobalScope(t *testing.T) {
	def := &AgentDefinition{Name: "reviewer", Scope: "project"}
	if err := DemoteAgent(def, "/any/dir", false); err == nil {
		t.Error("expected error when demoting a non-global-scoped agent")
	}
}

// ── Full round-trip ───────────────────────────────────────────────────────────

func TestRoundTrip_CreateExportImportValidate(t *testing.T) {
	tmp := t.TempDir()
	resolver := &Resolver{
		ProjectDir: filepath.Join(tmp, "project", ".owl", "agents"),
		GlobalDir:  filepath.Join(tmp, "global", ".owl", "agents"),
	}
	if err := os.MkdirAll(resolver.ProjectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resolver.GlobalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Step 1: Create a project-scoped agent
	writeFile(t, filepath.Join(resolver.ProjectDir, "tester", "AGENTS.md"), "# Tester\nRuns tests.")
	writeFile(t, filepath.Join(resolver.ProjectDir, "tester", "agent.yaml"),
		"name: tester\nversion: \"1.0\"\ndescription: Runs tests\ncapabilities:\n  - testing\n")

	// Step 2: Resolve and validate
	def, err := resolver.Resolve("tester", "project")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	errs := resolver.Validate(def, true)
	if len(errs) != 0 {
		t.Fatalf("validation failed: %v", errs)
	}

	// Step 3: Export
	exportDir := filepath.Join(tmp, "export")
	if err := ExportAgent(def, exportDir, false); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Step 4: Import into global scope under a different name
	importDest := resolver.GlobalDir
	imported, _, err := ImportAgent(exportDir, importDest, "tester-global")
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if imported.Name != "tester-global" {
		t.Errorf("expected imported name 'tester-global', got %q", imported.Name)
	}

	// Step 5: Validate imported definition
	imported.Scope = "global"
	errs = resolver.Validate(imported, true)
	if len(errs) != 0 {
		t.Errorf("imported definition failed validation: %v", errs)
	}
}
