// Package shellpath hydrates the current process's PATH from the user's
// login shell.
//
// Daemons launched by launchd (macOS) or systemd user (Linux) inherit a
// minimal PATH that typically excludes user-local bins such as
// ~/.local/bin, /opt/homebrew/bin, and nvm-managed Node bins. The harness
// subsystem resolves binaries with exec.LookPath, so without this step
// `owl --harness claude-code` (and similar) fails to find tools that the
// user can run from their terminal.
package shellpath

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// defaultShell returns a best-effort fallback shell for the current OS
// when $SHELL is unset (e.g. under launchd with a minimal environment).
func defaultShell() string {
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/bash"
}

// Hydrate runs the user's login shell once, captures its PATH, and applies
// it to the current process environment. Failures are logged and the
// existing PATH is left untouched.
func Hydrate() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = defaultShell()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// -l sources profile files; -i sources rc files (where nvm, pyenv,
	// homebrew shellenv, etc. typically live). Stdin is detached so the
	// shell cannot block on prompts; stderr is discarded to suppress rc
	// chatter.
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", `printf %s "$PATH"`)
	cmd.Stdin = nil
	cmd.Stderr = io.Discard

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Printf("path hydration: %s failed: %v (keeping current PATH)", shell, err)
		return
	}

	p := strings.TrimSpace(out.String())
	if p == "" || !strings.Contains(p, "/") {
		log.Printf("path hydration: %s returned no usable PATH (keeping current PATH)", shell)
		return
	}

	if p == os.Getenv("PATH") {
		return
	}

	if err := os.Setenv("PATH", p); err != nil {
		log.Printf("path hydration: setenv failed: %v", err)
		return
	}

	log.Printf("path hydration: loaded PATH from %s (%d entries)", shell, strings.Count(p, ":")+1)
}
