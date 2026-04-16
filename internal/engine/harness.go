package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/viche-ai/owl/internal/agents"
	"github.com/viche-ai/owl/internal/harness"
	"github.com/viche-ai/owl/internal/ipc"
)

func (e *AgentEngine) runHarness(ctx context.Context, args *ipc.HatchArgs, inbox chan ipc.InboundMessage, agentDef *agents.AgentDefinition, workDir string) {
	harnessStatus := "completed"
	defer func() {
		if e.collector != nil {
			m := e.collector.Finalize(harnessStatus)
			if e.MetricStore != nil {
				_ = e.MetricStore.Save(m)
			}
		}
		if e.RunStore != nil {
			if rec, err := e.RunStore.Load(e.State.RunID); err == nil {
				rec.State = "stopped"
				rec.ExitReason = harnessStatus
				now := time.Now()
				rec.EndTime = &now
				_ = e.RunStore.Save(rec)
			}
		}
	}()

	name := strings.ToLower(strings.TrimSpace(args.Harness))
	if name == "" {
		e.appendLog("[Error] Harness name is required.\n")
		harnessStatus = "failed"
		e.setState("idle")
		return
	}

	def, err := e.resolveHarness(name)
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] %v\n", err))
		harnessStatus = "failed"
		e.setState("idle")
		return
	}

	if err := def.CheckBinary(); err != nil {
		e.appendLog(fmt.Sprintf("[Error] %v\n", err))
		harnessStatus = "failed"
		e.setState("idle")
		return
	}

	extra, err := parseHarnessArgs(args.HarnessArgs)
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] %v\n", err))
		harnessStatus = "failed"
		e.setState("idle")
		return
	}

	// Inject agent definition context (if provided).
	description := args.Description
	var injectedEnv []string
	var injectionCleanup func()
	if agentDef != nil {
		var injErr error
		description, injectedEnv, injectionCleanup, injErr = harness.InjectAgentContext(def, agentDef, workDir, description)
		if injErr != nil {
			e.appendLog(fmt.Sprintf("[Warning] Agent context injection failed: %v\n", injErr))
		} else if def.ContextInjection != nil {
			e.appendLog(fmt.Sprintf("> Injected agent %q context via %s\n", agentDef.Name, def.ContextInjection.Method))
		}
	}
	if injectionCleanup != nil {
		defer injectionCleanup()
	}

	// Merge per-harness user config
	if e.Cfg != nil && e.Cfg.Harnesses != nil {
		if userCfg, ok := e.Cfg.Harnesses[name]; ok {
			extra = append(extra, userCfg.ExtraArgs...)
			for k, v := range userCfg.Env {
				injectedEnv = append(injectedEnv, k+"="+v)
			}
			if userCfg.ModelEnv != "" && args.ModelID != "" {
				injectedEnv = append(injectedEnv, userCfg.ModelEnv+"="+args.ModelID)
			}
		}
	}

	e.Mu(func() {
		e.State.Role = "harness:" + name
		e.State.ModelID = "harness/" + name
		e.State.Harness = name
		if e.State.Name == "" {
			e.State.Name = name
		}
	})

	// Start callback server (lives for the entire harness session).
	cbServer := harness.NewCallbackServer(
		func(payload map[string]interface{}) {
			if state, ok := payload["state"].(string); ok {
				e.appendLog(fmt.Sprintf("> [Callback status] %s\n", state))
			}
		},
		func(payload map[string]interface{}) {
			if msg, ok := payload["message"].(string); ok {
				e.appendLog(fmt.Sprintf("> [Callback log] %s\n", msg))
			}
		},
	)
	cbPort, cbErr := cbServer.Start()
	if cbErr == nil {
		defer cbServer.Stop()
		e.appendLog(fmt.Sprintf("> Callback server on port %d\n", cbPort))
	}

	// Build the base environment (shared across invocations in persistent mode).
	baseEnv := os.Environ()
	for k, v := range def.Env {
		baseEnv = append(baseEnv, k+"="+v)
	}
	baseEnv = append(baseEnv, injectedEnv...)
	if !args.NoNetInject {
		if url, token := e.Cfg.GetActiveRegistry(); token != "" {
			baseEnv = append(baseEnv, "VICHE_REGISTRY_TOKEN="+token)
			if url != "" {
				baseEnv = append(baseEnv, "VICHE_REGISTRY_URL="+url)
			}
		}
		baseEnv = append(baseEnv,
			"OWL_LOCAL_AGENT_ID="+e.State.ID,
			"OWL_HARNESS="+name,
		)
	}
	if cbErr == nil {
		baseEnv = append(baseEnv, "OWL_CALLBACK_URL="+cbServer.URL())
	}

	// ── Run loop ──
	// Non-persistent: run once, exit.
	// Persistent: run, then wait for next inbox message and re-invoke.
	currentDesc := description
	for {
		result := e.execHarness(ctx, def, name, currentDesc, workDir, extra, baseEnv, inbox)

		switch result.outcome {
		case harnessOutcomeStopped:
			harnessStatus = "stopped"
			e.setState("stopped")
			return
		case harnessOutcomeFailed:
			harnessStatus = "failed"
			e.setState("error")
			return
		case harnessOutcomeCompleted:
			// Continue below
		}

		if !def.Persistent {
			harnessStatus = "completed"
			e.setState("idle")
			return
		}

		// Persistent mode: wait for the next message.
		e.appendLog("> Harness idle — waiting for next message...\n")
		e.setState("idle")

		select {
		case <-ctx.Done():
			e.appendLog("\n> Agent force-stopped.\n")
			harnessStatus = "stopped"
			e.setState("stopped")
			return
		case msg, ok := <-inbox:
			if !ok {
				e.appendLog("\n> Agent stopped.\n")
				harnessStatus = "stopped"
				e.setState("stopped")
				return
			}
			e.appendLog(fmt.Sprintf("\n> [%s] %s\n", msg.From, msg.Content))
			currentDesc = msg.Content
			// Loop back to re-invoke the harness
		}
	}
}

// harnessOutcome represents how a single harness invocation ended.
type harnessOutcome int

const (
	harnessOutcomeCompleted harnessOutcome = iota
	harnessOutcomeFailed
	harnessOutcomeStopped
)

type harnessResult struct {
	outcome harnessOutcome
}

// execHarness runs a single invocation of the harness subprocess.
// It blocks until the subprocess exits or the context/inbox signals shutdown.
func (e *AgentEngine) execHarness(
	ctx context.Context,
	def *harness.HarnessDefinition,
	name, description, workDir string,
	extra, env []string,
	inbox chan ipc.InboundMessage,
) harnessResult {
	cmdName, cmdArgs, err := def.BuildCommand(description, workDir, extra)
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] %v\n", err))
		return harnessResult{harnessOutcomeFailed}
	}

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Dir = workDir
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] failed creating stdout pipe: %v\n", err))
		return harnessResult{harnessOutcomeFailed}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		e.appendLog(fmt.Sprintf("[Error] failed creating stderr pipe: %v\n", err))
		return harnessResult{harnessOutcomeFailed}
	}

	var stdinPipe io.WriteCloser
	if def.SupportsStdin {
		stdinPipe, err = cmd.StdinPipe()
		if err != nil {
			e.appendLog(fmt.Sprintf("[Error] failed creating stdin pipe: %v\n", err))
			return harnessResult{harnessOutcomeFailed}
		}
	}

	e.appendLog(fmt.Sprintf("> Harness: %s\n", name))
	e.appendLog(fmt.Sprintf("> Command: %s %s\n", cmdName, strings.Join(cmdArgs, " ")))
	e.appendLog(fmt.Sprintf("> Working dir: %s\n\n", workDir))

	if err := cmd.Start(); err != nil {
		e.appendLog(fmt.Sprintf("[Error] failed starting harness: %v\n", err))
		return harnessResult{harnessOutcomeFailed}
	}

	e.setState("flying")

	var wg sync.WaitGroup
	parser := harness.NewParser(def.OutputFormat)

	// stdout: parse through the output parser
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		// Increase buffer size to handle long JSON lines from stream-json
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			for _, ev := range parser.ProcessLine(scanner.Text()) {
				switch ev.Type {
				case "usage":
					input, output := harness.ExtractTokenUsage(ev)
					if e.collector != nil && (input > 0 || output > 0) {
						e.collector.RecordTokenUsage(input, output)
					}
					e.appendLog(fmt.Sprintf("> [Usage] input=%d output=%d\n", input, output))
				case "tool_call":
					e.appendLog(fmt.Sprintf("> [Harness tool] %s\n", ev.Content))
					if e.collector != nil {
						e.collector.RecordToolCall(ev.Content, true)
					}
				case "error":
					e.appendLog(fmt.Sprintf("[Error] %s\n", ev.Content))
				case "status":
					e.appendLog(fmt.Sprintf("> [Status] %s\n", ev.Content))
				default:
					e.appendLog(ev.Content + "\n")
				}
			}
		}
		if err := scanner.Err(); err != nil {
			e.appendLog(fmt.Sprintf("[Warning] stdout scanner error (line may have exceeded 1MB buffer): %v\n", err))
		}
	}()

	// stderr: always plain text
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			e.appendLog("[stderr] " + scanner.Text() + "\n")
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case <-ctx.Done():
			if stdinPipe != nil {
				_ = stdinPipe.Close()
			}
			_ = terminateHarness(cmd)
			<-done
			wg.Wait()
			e.appendLog("\n> Agent force-stopped.\n")
			return harnessResult{harnessOutcomeStopped}

		case err := <-done:
			wg.Wait()
			if err != nil {
				e.appendLog(fmt.Sprintf("\n[Harness exited with error] %v\n", err))
				return harnessResult{harnessOutcomeFailed}
			}
			e.appendLog("\n[Harness exited successfully]\n")
			return harnessResult{harnessOutcomeCompleted}

		case msg, ok := <-inbox:
			if !ok {
				if stdinPipe != nil {
					_ = stdinPipe.Close()
				}
				_ = terminateHarness(cmd)
				<-done
				wg.Wait()
				e.appendLog("\n> Agent stopped.\n")
				return harnessResult{harnessOutcomeStopped}
			}
			e.appendLog(fmt.Sprintf("\n> [%s] %s\n", msg.From, msg.Content))
			if stdinPipe != nil {
				_, _ = fmt.Fprintf(stdinPipe, "%s\n", msg.Content)
				e.appendLog("> Message relayed to harness stdin.\n")
			} else if def.Persistent {
				// In persistent mode, queue messages for next invocation.
				// For now, just log — the message will be lost if harness
				// is still running. The next invocation uses the message
				// delivered to the outer loop.
				e.appendLog("> Message received (will process after current task completes).\n")
			} else {
				e.appendLog("> Harness does not support interactive stdin relay.\n")
			}
		}
	}
}

// resolveHarness looks up a harness definition from the registry, falling back
// to a fresh registry (with user definitions) if no registry is set.
func (e *AgentEngine) resolveHarness(name string) (*harness.HarnessDefinition, error) {
	if e.HarnessRegistry != nil {
		return e.HarnessRegistry.Resolve(name)
	}
	r := harness.NewRegistry()
	_ = r.LoadUserDir()
	return r.Resolve(name)
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
