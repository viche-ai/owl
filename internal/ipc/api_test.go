package ipc

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDryRunHatch_UsesRequestedWorkDirForProjectAgents(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home")
	projectDir := filepath.Join(tmp, "project")
	daemonDir := filepath.Join(tmp, "daemon")

	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(daemonDir, 0o755); err != nil {
		t.Fatalf("mkdir daemon dir: %v", err)
	}
	t.Setenv("HOME", homeDir)

	agentDir := filepath.Join(projectDir, ".owl", "agents", "reviewer")
	writeTestFile(t, filepath.Join(agentDir, "AGENTS.md"), "# Project Reviewer\nDocuments the project.")
	writeTestFile(t, filepath.Join(agentDir, "agent.yaml"), "name: reviewer\ncapabilities:\n  - Agent\n")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(oldWD); chdirErr != nil {
			t.Fatalf("restore wd: %v", chdirErr)
		}
	}()
	if err := os.Chdir(daemonDir); err != nil {
		t.Fatalf("chdir daemon dir: %v", err)
	}

	var reply DryRunReply
	svc := NewService()
	err = svc.DryRunHatch(&HatchArgs{
		Agent:   "reviewer",
		WorkDir: projectDir,
	}, &reply)
	if err != nil {
		t.Fatalf("DryRunHatch returned error: %v", err)
	}

	if !reply.Valid {
		t.Fatalf("expected valid dry-run reply, got errors: %v", reply.Errors)
	}
	if reply.Scope != "project" {
		t.Fatalf("expected project scope, got %q", reply.Scope)
	}
	wantSource := filepath.Join(projectDir, ".owl", "agents", "reviewer")
	if reply.SourcePath != wantSource {
		t.Fatalf("expected source path %q, got %q", wantSource, reply.SourcePath)
	}
	if reply.ResolvedAgent != "reviewer" {
		t.Fatalf("expected resolved agent %q, got %q", "reviewer", reply.ResolvedAgent)
	}
	if reply.PromptStack == "" {
		t.Fatal("expected prompt stack to be populated")
	}
}
