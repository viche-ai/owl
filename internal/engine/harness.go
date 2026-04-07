package engine

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/viche-ai/owl/internal/ipc"
)

func (e *AgentEngine) runHarness(args *ipc.HatchArgs, inbox chan ipc.InboundMessage) {
	name := strings.ToLower(strings.TrimSpace(args.Harness))
	if name == "" {
		e.appendLog("[Error] Harness name is required.\n")
		e.setState("idle")
		return
	}

	cmdName, cmdArgs, err := buildHarnessCommand(name, args)
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] %v\n", err))
		e.setState("idle")
		return
	}

	workDir := args.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = os.Environ()
	if !args.NoNetInject {
		if _, token := e.Cfg.GetActiveRegistry(); token != "" {
			cmd.Env = append(cmd.Env, "VICHE_REGISTRY_TOKEN="+token)
		}
		cmd.Env = append(cmd.Env,
			"OWL_LOCAL_AGENT_ID="+e.State.ID,
			"OWL_HARNESS="+name,
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] failed creating stdout pipe: %v\n", err))
		e.setState("idle")
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] failed creating stderr pipe: %v\n", err))
		e.setState("idle")
		return
	}

	e.Mu(func() {
		e.State.Role = "harness:" + name
		e.State.ModelID = "harness/" + name
	})
	if e.State.Name == "" {
		e.Mu(func() { e.State.Name = name })
	}

	e.appendLog(fmt.Sprintf("> Harness: %s\n", name))
	e.appendLog(fmt.Sprintf("> Command: %s %s\n", cmdName, strings.Join(cmdArgs, " ")))
	e.appendLog(fmt.Sprintf("> Working dir: %s\n\n", workDir))

	if err := cmd.Start(); err != nil {
		e.appendLog(fmt.Sprintf("[Error] failed starting harness: %v\n", err))
		e.setState("idle")
		return
	}

	e.setState("flying")

	var wg sync.WaitGroup
	stream := func(prefix string, s *bufio.Scanner) {
		defer wg.Done()
		for s.Scan() {
			e.appendLog(prefix + s.Text() + "\n")
		}
	}

	wg.Add(2)
	go stream("", bufio.NewScanner(stdout))
	go stream("[stderr] ", bufio.NewScanner(stderr))

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case err := <-done:
			wg.Wait()
			if err != nil {
				e.appendLog(fmt.Sprintf("\n[Harness exited with error] %v\n", err))
				e.setState("error")
			} else {
				e.appendLog("\n[Harness exited successfully]\n")
				e.setState("idle")
			}
			return
		case msg, ok := <-inbox:
			if !ok {
				_ = terminateHarness(cmd)
				<-done
				wg.Wait()
				e.appendLog("\n> Agent stopped.\n")
				e.setState("stopped")
				return
			}
			e.appendLog(fmt.Sprintf("\n> [%s] %s\n", msg.From, msg.Content))
			e.appendLog("> Harness mode currently does not support interactive stdin relay.\n")
		}
	}
}

func terminateHarness(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}

func parseHarnessArgs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.ContainsAny(raw, "\n\r;&|`><") {
		return nil, fmt.Errorf("harness-args contains disallowed shell metacharacters")
	}
	return strings.Fields(raw), nil
}

func buildHarnessCommand(h string, args *ipc.HatchArgs) (string, []string, error) {
	extra, err := parseHarnessArgs(args.HarnessArgs)
	if err != nil {
		return "", nil, err
	}
	workDir := args.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	switch h {
	case "codex":
		base := []string{"exec", args.Description}
		base = append(base, extra...)
		return "codex", base, nil
	case "opencode":
		base := []string{"run", "--dir", workDir, args.Description}
		base = append(base, extra...)
		return "opencode", base, nil
	case "claude-code", "claude":
		base := []string{"--print", args.Description}
		base = append(base, extra...)
		return "claude", base, nil
	default:
		return "", nil, fmt.Errorf("unsupported harness %q (allowed: codex, opencode, claude-code)", h)
	}
}
