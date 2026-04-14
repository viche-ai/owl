package engine

import (
	"strings"
	"testing"

	"github.com/viche-ai/owl/internal/harness"
)

func TestBuildHarnessCommand_ViaRegistry(t *testing.T) {
	r := harness.NewRegistry()

	tests := []struct {
		name      string
		harness   string
		desc      string
		workDir   string
		wantCmd   string
		wantParts []string
	}{
		{
			name:      "codex",
			harness:   "codex",
			desc:      "fix tests",
			workDir:   "/work",
			wantCmd:   "codex",
			wantParts: []string{"exec", "fix tests"},
		},
		{
			name:      "opencode includes dir",
			harness:   "opencode",
			desc:      "review",
			workDir:   "/tmp/x",
			wantCmd:   "opencode",
			wantParts: []string{"run", "--dir", "/tmp/x", "review"},
		},
		{
			name:      "claude-code",
			harness:   "claude-code",
			desc:      "plan this",
			workDir:   "/work",
			wantCmd:   "claude",
			wantParts: []string{"-p", "--verbose", "--output-format", "stream-json", "plan this"},
		},
		{
			name:      "claude alias",
			harness:   "claude",
			desc:      "plan this",
			workDir:   "/work",
			wantCmd:   "claude",
			wantParts: []string{"-p", "--verbose", "--output-format", "stream-json", "plan this"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, err := r.Resolve(tc.harness)
			if err != nil {
				t.Fatalf("resolve %q: %v", tc.harness, err)
			}

			cmd, parts, err := def.BuildCommand(tc.desc, tc.workDir, nil)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if cmd != tc.wantCmd {
				t.Fatalf("cmd got %q want %q", cmd, tc.wantCmd)
			}
			if len(parts) < len(tc.wantParts) {
				t.Fatalf("parts too short: got %v want prefix %v", parts, tc.wantParts)
			}
			for i := range tc.wantParts {
				if parts[i] != tc.wantParts[i] {
					t.Fatalf("parts[%d] got %q want %q (all: %v)", i, parts[i], tc.wantParts[i], parts)
				}
			}
		})
	}
}

func TestParseHarnessArgs_Valid(t *testing.T) {
	args, err := parseHarnessArgs("--verbose --flag value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"--verbose", "--flag", "value"}
	if len(args) != len(want) {
		t.Fatalf("got %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] got %q want %q", i, args[i], want[i])
		}
	}
}

func TestParseHarnessArgs_Empty(t *testing.T) {
	args, err := parseHarnessArgs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args != nil {
		t.Fatalf("expected nil, got %v", args)
	}
}

func TestParseHarnessArgs_RejectMetacharacters(t *testing.T) {
	bad := []string{
		"--foo ; rm -rf /",
		"--foo | cat",
		"--foo & bg",
		"--foo `cmd`",
		"--foo > file",
		"--foo < file",
		"--foo\nbar",
	}
	for _, input := range bad {
		_, err := parseHarnessArgs(input)
		if err == nil {
			t.Errorf("expected rejection for %q", input)
		}
		if err != nil && !strings.Contains(err.Error(), "disallowed shell metacharacters") {
			t.Errorf("unexpected error message: %v", err)
		}
	}
}

func TestResolveHarness_UnknownName(t *testing.T) {
	r := harness.NewRegistry()
	_, err := r.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown harness")
	}
	if !strings.Contains(err.Error(), "unknown harness") {
		t.Fatalf("expected 'unknown harness' error, got: %v", err)
	}
}
