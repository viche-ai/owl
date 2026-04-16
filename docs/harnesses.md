# Harnesses

Owl can wrap external AI coding tools — Claude Code, OpenCode, Codex, or any CLI — as managed subprocesses. This gives you Owl's visibility (TUI, logs, metrics, run tracking) on top of whatever agent runtime you already use.

---

## Quick start

```bash
owl hatch --harness claude-code "Fix the failing test in auth_test.go"
```

That's it. Owl spawns Claude Code in streaming mode, pipes all output (thinking, tool calls, results) into the TUI in real time, and tracks the run in `~/.owl/runs/`. When Claude finishes, the harness stays alive — send another message from the TUI and it kicks off a new task.

---

## Built-in harnesses

Three harnesses ship out of the box:

| Name | Binary | Command template | Notes |
|------|--------|-----------------|-------|
| `claude-code` | `claude` | `claude -p --verbose --output-format stream-json <description>` | Alias `claude`. Streaming output. Persistent (stays alive between tasks). |
| `opencode` | `opencode` | `opencode run --dir <workdir> <description>` | Working directory auto-injected |
| `codex` | `codex` | `codex exec <description>` | |

### Examples

```bash
# Claude Code
owl hatch --harness claude-code "Refactor the database layer to use connection pooling"

# OpenCode
owl hatch --harness opencode --dir ~/projects/api "Add pagination to the /users endpoint"

# Codex
owl hatch --harness codex "Explain what this codebase does"

# Pass extra flags to the harness binary
owl hatch --harness claude-code --harness-args "--verbose" "Review this PR"
```

---

## Combining harnesses with agent definitions

Agent definitions (`.owl/agents/<name>/`) provide identity, instructions, and guardrails for your agents. When you combine `--agent` with `--harness`, Owl injects the agent's content into the harness:

```bash
owl hatch --agent reviewer --harness claude-code "Review PR #42"
```

How injection works depends on the harness:

| Harness | Injection method | What happens |
|---------|-----------------|--------------|
| `claude-code` | `file` | Writes a temporary `CLAUDE.md` in the working directory containing the agent's AGENTS.md, role.md, and guardrails.md. Cleaned up after the harness exits. If a CLAUDE.md already exists, it's backed up and restored. |
| `codex` | `arg-prepend` | Prepends the agent content to the description argument |
| `opencode` | `arg-prepend` | Prepends the agent content to the description argument |

This means you can write your agent definitions once and use them across different harnesses — the same `reviewer` agent works on Claude Code, Codex, or a custom harness.

---

## Flags

| Flag | Description |
|------|-------------|
| `--harness <name>` | Which harness to run (built-in or custom) |
| `--harness-args "<args>"` | Extra arguments passed to the harness binary |
| `--no-network-inject` | Don't inject `VICHE_REGISTRY_TOKEN`, `OWL_LOCAL_AGENT_ID`, or `OWL_HARNESS` into the harness environment |
| `--dir <path>` | Working directory for the harness process |
| `--agent <name>` | Agent definition to inject into the harness |

### Environment variables injected

Unless `--no-network-inject` is set, the harness subprocess receives:

| Variable | Value |
|----------|-------|
| `VICHE_REGISTRY_TOKEN` | Your active registry token (if authenticated) |
| `VICHE_REGISTRY_URL` | Your active registry URL (if authenticated) |
| `OWL_LOCAL_AGENT_ID` | The Owl agent ID managing this harness |
| `OWL_HARNESS` | The harness name (e.g. `claude-code`) |
| `OWL_CALLBACK_URL` | Localhost HTTP URL for harness-to-Owl callbacks |

---

## Viche networking with Claude Code

Claude Code harnesses can participate in the Viche agent network — discovering and messaging other agents — by installing the [Viche plugin for Claude Code](https://github.com/viche-ai/viche).

### Setup

Install the plugin from the Claude Code marketplace:

```
/install-plugin viche-ai/viche:channel/claude-code-plugin-viche
```

Or for local development, add the MCP server manually to your Claude Code settings.

### How it works

When Owl hatches a `--harness claude-code` agent, it injects `VICHE_REGISTRY_TOKEN` and `VICHE_REGISTRY_URL` into the subprocess environment. The Viche plugin reads these env vars automatically and joins the same registry as your other Owl agents.

This gives Claude Code three additional tools:

| Tool | Description |
|------|-------------|
| `viche_discover` | Find agents by capability (e.g. `"debugging"`) or list all (`"*"`) |
| `viche_send` | Send a task, result, or ping to another agent by ID |
| `viche_reply` | Reply to an inbound message from another agent |

### Example: multi-agent workflow with harnesses

```bash
# Start a native Owl agent that listens for review requests
owl hatch --agent reviewer --ambient

# Use Claude Code to write code, then hand off to the reviewer
owl hatch --harness claude-code "Implement user auth, then use viche_discover to find a reviewer and send it your changes"
```

The Claude Code harness discovers the reviewer agent on Viche, sends it a message, and the reviewer picks up the work — same coordination pattern as native Owl agents.

### Without the plugin

If the Viche plugin is not installed, Claude Code still works as a harness — it just can't discover or message other agents. The env vars are injected but unused.

---

## Persistent mode

By default, a harness runs once and exits. Persistent harnesses stay alive after each task completes — when you send a new message from the TUI, Owl re-invokes the harness with that message as the new task.

The `claude-code` built-in is persistent by default. After it finishes a task, you'll see `> Harness idle — waiting for next message...` in the TUI. Type a follow-up and it runs again.

For custom harnesses, set `persistent: true` in the YAML:

```yaml
# ~/.owl/harnesses/my-tool.yaml
name: my-tool
binary: my-tool
args: ["{{description}}"]
persistent: true
```

The lifecycle looks like:

```
hatch → flying (task 1) → idle → flying (task 2) → idle → ... → stopped (user stop)
```

Non-persistent harnesses (like `codex` and `opencode`) run once and transition straight to idle/stopped.

---

## Per-harness configuration

Set persistent defaults per harness in `~/.owl/config.json`:

```json
{
  "harnesses": {
    "claude-code": {
      "extra_args": ["--verbose"],
      "env": {
        "ANTHROPIC_MODEL": "claude-sonnet-4-6"
      },
      "model_env": "ANTHROPIC_MODEL"
    },
    "codex": {
      "env": {
        "OPENAI_API_KEY": "sk-..."
      }
    }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `extra_args` | `string[]` | Appended to every invocation of this harness |
| `env` | `map[string]string` | Extra environment variables set on the harness subprocess |
| `model_env` | `string` | When `--model` is passed on the CLI, set this env var to the model ID |

Config values merge with (don't replace) values from the harness definition and CLI flags.

---

## Custom harnesses

Any CLI tool that accepts a task description and writes output to stdout can be an Owl harness. Define one by creating a YAML file in `~/.owl/harnesses/`:

### Example: Aider

```yaml
# ~/.owl/harnesses/aider.yaml
name: aider
binary: aider
args: ["--message", "{{description}}"]
supports_stdin: true
description: Aider AI pair programming
```

```bash
owl hatch --harness aider "Add type hints to utils.py"
```

### Example: a custom script

```yaml
# ~/.owl/harnesses/my-tool.yaml
name: my-tool
binary: python
args: ["/path/to/my_agent.py", "--task", "{{description}}", "--workdir", "{{workdir}}"]
env:
  MY_TOOL_MODE: "autonomous"
output_format: ndjson
description: My custom agent wrapper
```

### YAML schema

```yaml
# Required
name: my-harness           # unique identifier
binary: my-binary          # executable name (must be on PATH)
args:                      # argument list with template placeholders
  - "--flag"
  - "{{description}}"      # replaced with the task description
  - "{{workdir}}"          # replaced with the working directory

# Optional
aliases:                   # alternative names that resolve to this harness
  - "mh"
  - "my"
description: "..."         # human-readable description
env:                       # extra environment variables
  KEY: "value"
supports_stdin: false      # if true, user/Viche messages are piped to stdin
persistent: false          # if true, stays alive between tasks (re-invoked per message)
output_format: text        # "text" (default), "ndjson", or "claude-stream-json"
workdir_flag: "--dir"      # if set, injected as [flag, workdir] before description

# Context injection (how agent definitions are passed to this harness)
context_injection:
  method: arg-prepend      # "arg-prepend", "file", or "env"
  path: "{{workdir}}/CLAUDE.md"   # for method: file
  env_var: "SYSTEM_PROMPT"        # for method: env
```

### Template placeholders

| Placeholder | Replaced with |
|-------------|--------------|
| `{{description}}` | The task description from `owl hatch ... "description"` |
| `{{workdir}}` | The working directory (from `--dir` or current directory) |

### Context injection methods

When `--agent` is used with a harness, Owl needs to pass the agent definition content to the harness. The `context_injection` config controls how:

| Method | Behavior |
|--------|----------|
| `arg-prepend` | Prepends agent content to the description argument. Safest default — works with any CLI. |
| `file` | Writes agent content to a file at `path`. Backs up any existing file and restores it after the harness exits. Good for tools that read config files (e.g. Claude Code reads `CLAUDE.md`). |
| `env` | Sets the environment variable named in `env_var` to the agent content. Good for tools that accept a system prompt via env. |

If `context_injection` is omitted and an agent definition is provided, Owl defaults to `arg-prepend`.

---

## Structured output

By default, Owl treats harness stdout as plain text. If your harness emits structured output, set `output_format: ndjson` and emit one JSON object per line:

```jsonl
{"type": "text", "content": "Reading the codebase..."}
{"type": "usage", "input_tokens": 1500, "output_tokens": 300}
{"type": "tool_call", "name": "file_read", "args": "{\"path\": \"main.go\"}"}
{"type": "status", "state": "thinking"}
{"type": "error", "message": "Rate limited, retrying..."}
```

Owl recognizes these event types:

| Type | Effect |
|------|--------|
| `text` | Displayed in the TUI log |
| `usage` | `input_tokens` and `output_tokens` are recorded in Owl's metrics collector |
| `tool_call` | `name` is recorded in metrics; displayed as a tool call in the log |
| `error` | Displayed as an error in the log |
| `status` | Displayed as a status update |

Lines that aren't valid JSON are treated as plain text. This means a harness that sometimes emits JSON and sometimes emits plain text will work correctly.

### Claude Code streaming format

The `claude-code` built-in uses `output_format: claude-stream-json`, a parser purpose-built for Claude Code's `--output-format stream-json` output. It understands:

| Claude event | Owl event | What's extracted |
|-------------|-----------|-----------------|
| `{"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}` | `text` | Assistant text, streamed live |
| `{"type":"tool_use","tool":{"name":"Read",...}}` | `tool_call` | Tool name, recorded in metrics |
| `{"type":"tool_result","tool":{"name":"Read"},"content":"..."}` | `text` | Tool result (truncated preview) |
| `{"type":"result","input_tokens":N,"output_tokens":N,...}` | `text` + `usage` | Final result text + token counts |

This means you see Claude's full thought process — tool calls, results, and text — streaming into the TUI in real time, with token usage automatically captured in metrics.

Custom harnesses can use `output_format: claude-stream-json` if they emit the same format.

---

## Callback API

Every harness run gets a localhost HTTP callback server. The URL is injected as `OWL_CALLBACK_URL`. Your harness can POST structured events back to Owl:

### POST /status

Report a state change:

```bash
curl -X POST "$OWL_CALLBACK_URL/status" \
  -H "Content-Type: application/json" \
  -d '{"state": "thinking", "message": "Analyzing dependencies..."}'
```

### POST /log

Send a structured log entry:

```bash
curl -X POST "$OWL_CALLBACK_URL/log" \
  -H "Content-Type: application/json" \
  -d '{"message": "Found 3 test failures", "level": "warn"}'
```

Both endpoints return `{"ok": true}` on success. The callback server shuts down when the harness exits.

---

## How it works

When you run `owl hatch --harness <name>`:

1. **Resolve** — Owl looks up the harness definition (built-in or from `~/.owl/harnesses/`)
2. **Pre-check** — Verifies the binary exists on PATH
3. **Inject context** — If `--agent` is set, applies the agent definition via the harness's injection method
4. **Merge config** — Applies per-harness config from `~/.owl/config.json`
5. **Start callback server** — Binds to an ephemeral localhost port
6. **Spawn subprocess** — Runs the harness with expanded args, merged env vars, and injected `OWL_CALLBACK_URL`
7. **Stream output** — Parses stdout through the output parser, streams stderr as-is
8. **Track lifecycle** — Records run metadata, timing, and metrics to `~/.owl/runs/`
9. **Cleanup** — Stops callback server, restores backed-up files, records final state

The harness subprocess inherits your shell environment, plus any env vars from the definition, config, and Owl's network injection.

---

## Comparison: harness vs native agent

| | Harness mode | Native mode |
|---|---|---|
| **Runtime** | External subprocess | Owl's built-in LLM loop |
| **Identity** | From harness name | LLM-scaffolded or from agent definition |
| **Viche** | Optional (env vars available) | Automatic registration + WebSocket |
| **Tools** | Whatever the harness provides | viche_discover, viche_send, shell, file, tasks |
| **Metrics** | Exit code + parsed output | Full token usage, tool calls, duration |
| **Stdin** | Configurable per harness | Always interactive |

Use harnesses when you want Owl's management layer around a tool you already use. Use native mode when you want the full Owl agent experience with Viche networking and tool use.
