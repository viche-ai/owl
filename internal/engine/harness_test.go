package engine

import (
	"strings"
	"testing"

	"github.com/viche-ai/owl/internal/ipc"
)

func TestBuildHarnessCommand(t *testing.T) {
	tests := []struct {
		name      string
		harness   string
		args      *ipc.HatchArgs
		wantCmd   string
		wantParts []string
		wantErr   string
	}{
		{
			name:      "codex",
			harness:   "codex",
			args:      &ipc.HatchArgs{Description: "fix tests"},
			wantCmd:   "codex",
			wantParts: []string{"exec", "fix tests"},
		},
		{
			name:      "opencode includes dir",
			harness:   "opencode",
			args:      &ipc.HatchArgs{Description: "review", WorkDir: "/tmp/x"},
			wantCmd:   "opencode",
			wantParts: []string{"run", "--dir", "/tmp/x", "review"},
		},
		{
			name:      "claude",
			harness:   "claude-code",
			args:      &ipc.HatchArgs{Description: "plan this"},
			wantCmd:   "claude",
			wantParts: []string{"--print", "plan this"},
		},
		{
			name:    "reject shell metacharacters",
			harness: "codex",
			args:    &ipc.HatchArgs{Description: "x", HarnessArgs: "--foo ; rm -rf /"},
			wantErr: "disallowed shell metacharacters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, parts, err := buildHarnessCommand(tc.harness, tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
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
