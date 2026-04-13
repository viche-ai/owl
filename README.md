# Owl - The meta-harness for AI agents

**Create, monitor, and improve multi-agent systems — with Viche networking built in.**

---

You have agents, maybe your agents have agents. Sometimes they go off on a tagent for 30 minutes and you don't know why.

You fire one at a task. You wait. If it goes wrong, you find out at the end — if you find out at all. You can't tell if it ignored your guardrails, burned tokens on the wrong branch, or is just stuck in a loop. And if you're running several at once, they have no idea the others exist.

Other developers are starting to run *fleets* of coordinated agents. The gap between them and everyone else is opening up fast.

Owl helps devs, teams, and orgs close that gap.

> **New here?** [try the 5-minute quickstart →](docs/quickstart.md) — spin up federated agents + review pipeline and watch them coordinate in real-time.

---

![Owl Demo](assets/owl-demo.gif)

---

## What Owl is

Owl is a meta-harness: it wraps your existing agent runtimes — `claude-code`, `opencode`, `codex`, or Owl's own built-in LLM loop — and gives you a production-grade control surface on top of them.

Think of it as what you put *around* your agents, not what replaces them.

```
owl design ──► AGENTS.md ──► owl hatch ──► owld ──► Viche registry
                                              │            │
                                         TUI (owl)   other agents
                                              │
                                        metrics ──► owl recommend ──► better AGENTS.md
```

Three things Owl does that nothing else does together:

| | |
|---|---|
| **Create** | Define reusable, portable agent identities with governance built in |
| **Monitor** | Every run is visible, inspectable, and controllable in real-time |
| **Improve** | Structured metrics per run feed a prompt improvement loop |

And every agent you hatch is automatically connected to [Viche](https://viche.ai), a real-time agent network, so they can find, reach, and communicate with each other out of the box, across processes, terminals, and devices.

---

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://owl.viche.ai/install.sh | bash
```

Installs `owl` and `owld` to `~/.local/bin` and sets up the daemon as a background service (launchd on macOS, systemd on Linux).

### Build from Source

```bash
git clone https://github.com/viche-ai/owl.git
cd owl
make build
```

Produces `bin/owl` and `bin/owld`.

---

## Quick Start

```bash
# 1. Start the daemon (auto-starts if installed via install.sh)
owld

# 2. Open the TUI (in a separate terminal)
owl

# 3. Design your first agent (guided interview)
owl design "a code reviewer for PRs"

# 4. Or hatch an agent directly
owl hatch --agent reviewer
```

First time? Run the interactive setup wizard:

```bash
owl setup
```

---

## Agent Definitions: agents that have identity

A prompt is disposable. An agent definition is reusable, versioned, and governable.

Owl's definition system lets you encode *who an agent is* — not just what it does right now. Definitions live in plain-text files you can version-control, share, and compose.

### Structure

```
.owl/agents/reviewer/
├── AGENTS.md       # Required. The agent's role, capabilities, and behavior.
├── agent.yaml      # Optional. Metadata: name, version, model, capabilities, owner.
├── role.md         # Optional. Extended role description and context.
└── guardrails.md   # Optional. Constraints this agent must follow.
```

### Designing Agents

The fastest way to create a well-structured agent definition is with the guided design interview:

```bash
# Start the interview in the TUI
owl design

# Or provide an initial description to skip early questions
owl design "a security auditor that scans for OWASP top 10 vulnerabilities"
```

You can also start the interview by typing "design an agent" directly in the TUI console (owl tab).

The meta-agent will:
1. Gather context about your project (existing agents, guardrails, available models)
2. Interview you about the agent's purpose, workflow, coordination needs, and constraints
3. Generate a complete draft (AGENTS.md + agent.yaml + optional files) for your review
4. Create the files after you approve

### Scopes

Agent definitions live at two scopes with clear precedence:

- **Project scope** — `.owl/agents/<name>/` — specific to this repository
- **Global scope** — `~/.owl/agents/<name>/` — available everywhere on your machine

Project scope wins when both exist. Use `--scope` to be explicit.

### Lifecycle Commands

```bash
# Discovery
owl agents list [--scope project|global|all]
owl agents show <agent>
owl agents validate <agent> [--strict]

# Portability
owl agents export --agent <name> --out ./bundles/
owl agents import --path ./bundles/reviewer/ [--scope global]

# Promotion
owl agents promote --agent reviewer          # project → global
owl agents demote  --agent reviewer          # global → project

# Versioning
owl agents diff reviewer --from HEAD~3 --to HEAD

# Debugging
owl agents explain <agent>   # show the full resolved prompt stack
owl explain <agent>          # same, top-level alias
```

### Prompt Layers

When an agent is hatched, Owl assembles its system prompt in layers — highest priority wins:

```
1. [RUNTIME OVERRIDE]  —  --prompt flag or --from-file
2. [PROJECT AGENT]     —  AGENTS.md + role.md + guardrails.md
3. [PROJECT POLICY]    —  .owl/project.json context + guardrails
4. [OWL DEFAULTS]      —  ~/.owl/config.json system_prompt
```

Run `owl agents explain <name>` to see exactly what an agent will receive before you hatch it.

---

## Viche: your agents, networked

When you hatch an agent with Owl, it is automatically registered on [Viche](https://viche.ai) — a real-time agent network built on Phoenix Channels. Every agent gets a secure identity and bidirectional communication channel, immediately, without configuration.

This is what makes multi-agent orchestration real rather than theoretical:

- **Agents discover each other** — a builder agent can find and message a reviewer agent by name, even across machines
- **Agents communicate in real-time** — via WebSocket push, not polling
- **You can inspect any agent remotely** — without being in the same terminal session
- **Identity is secure** — private registries use token-based authentication

By default, agents join the public Viche registry. For team or production use, create a private registry at [viche.ai](https://viche.ai/signup) or self-host.

```bash
owl viche status
owl viche add-registry <token>
owl viche set-default <token>
```

---

## Monitoring: nothing runs blind

Every agent runs inside `owld` — the Nest Daemon — and survives terminal disconnects. The `owl` TUI gives you a live view of every active agent, its logs, and its state. You can stop, inspect, or remove any run without touching the agent process directly.

### Runs

```bash
owl runs list [--state flying|stopped|archived]
owl runs inspect <run-id>
owl runs stop   <run-id> [--force]
owl runs remove <run-id> [--no-archive --force]
```

### Logs

```bash
owl logs list
owl logs show   <run-id>
owl logs tail   <run-id>
owl logs query  --agent reviewer --since 1h --json
```

### External tools

Scripts, external harnesses, and other processes can stream into the Owl TUI over HTTP — they show up as first-class tabs alongside native agents:

```bash
POST http://localhost:7890/api/v1/stream
Content-Type: application/json

{"agent_id": "build-1", "agent_name": "CI build", "state": "flying", "log_line": "Running tests..."}
```

---

## Improvement: agents get better over time

After runs complete, Owl captures structured metrics per agent: task success, tool call patterns, failure categories, and more. These feed into a prompt improvement loop.

```bash
owl metrics show <agent|run-id>      # inspect what happened
owl recommend --agent <name>         # get prompt improvement suggestions
owl design [description...]          # guided interview to create a new agent
```

The improvement loop is designed around *approval*, not autonomy — Owl surfaces suggestions and diffs, you decide what to apply and commit. The goal is a tightening cycle: better definitions → better runs → better metrics → better definitions.

---

## Hatching agents

```bash
owl hatch [description...] [flags]
```

| Flag | Description |
|---|---|
| `--agent <name>` | Use a named agent definition (recommended) |
| `--from-file <path>` | Load the task prompt from a file |
| `--scope project\|global` | Restrict agent definition resolution to a scope |
| `--dry-run` | Resolve and print the full agent setup without spawning |
| `--model <provider/model>` | Override the model (e.g. `google/gemini-2.5-pro`) |
| `--name <name>` | Give this run a display name |
| `--count <n>` | Spawn `n` agents simultaneously |
| `--ambient` | Start the agent waiting for network messages, not a task |
| `--harness <name>` | Run via an external harness: `claude-code`, `opencode`, `codex` |
| `--harness-args "..."` | Extra arguments passed to the harness |
| `--dir <path>` | Set the working directory |
| `--thinking` | Enable extended reasoning mode |
| `--effort low\|medium\|high` | Set reasoning effort |
| `--registry <token>` | Override the Viche registry for this agent |
| `--no-network-inject` | Disable Viche environment variable injection |

```bash
# Reproducible hatch from a named definition
owl hatch --agent reviewer --scope project

# Preview the full prompt stack without running
owl hatch --agent reviewer --dry-run

# Spawn 3 parallel build agents
owl hatch --agent builder --count 3

# Run via claude-code with a project-level definition
owl hatch --agent architect --harness claude-code --dir ./
```

---

## Project Setup

```bash
# Initialize Owl in the current repo
owl project init

# Run the AGENTS.md wizard for this project
owl project agents

# Define guardrails agents must follow
owl project guards
```

Project configuration lives in `.owl/project.json`:

```json
{
  "version": "1.0",
  "context": "This is a Go microservice using PostgreSQL and gRPC",
  "guardrails": [
    "Never modify database migrations without explicit approval",
    "All new packages must have tests before being committed",
    "Never push directly to main"
  ]
}
```

---

## Configuration

Config lives in `~/.owl/config.json`. Set it interactively with `owl setup` or directly:

```bash
owl config show
owl config set-key anthropic sk-ant-...
owl config set-model anthropic/claude-sonnet-4-6
owl config import opencode --path /path/to/config
```

### Multi-provider support

Owl routes to any provider in `provider/model` format:

```json
{
  "models": {
    "default": "anthropic/claude-sonnet-4-6",
    "providers": {
      "anthropic": { "apiKey": "sk-ant-..." },
      "google":    { "apiKey": "AIza..." },
      "openai":    { "apiKey": "sk-..." }
    }
  }
}
```

Supported: Anthropic, OpenAI, Google, Groq, Together, Ollama, and custom endpoints.

---

## Migrating from templates

Owl previously used JSON templates (`~/.owl/templates/<name>.json`). Agent definitions replace them. To check for and convert legacy templates:

```bash
owl migrate check
owl migrate templates
```

The `--template` flag still works but is deprecated in favor of `--agent`.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  owl (TUI Client)                                        │
│  Visualizer + console. Connects via Unix socket RPC.     │
└───────────────────────┬─────────────────────────────────┘
                        │ Unix socket (/tmp/owld.sock)
┌───────────────────────▼─────────────────────────────────┐
│  owld (The Nest Daemon)                                  │
│  Manages agent state, LLM connections, run records,      │
│  structured logs, metrics. Exposes HTTP on :7890.        │
└───────────────────────┬─────────────────────────────────┘
                        │ WebSocket (Phoenix Channels)
┌───────────────────────▼─────────────────────────────────┐
│  Viche Registry                                          │
│  Real-time agent network. Identity, discovery, comms.    │
│  Public: viche.ai  |  Private: your registry token       │
└─────────────────────────────────────────────────────────┘
```

Agents run inside `owld` and survive terminal disconnects. The TUI is a pure visualizer — you can close it and reattach later without affecting running agents.

---

## License

GPLv3 — see [LICENSE](LICENSE).
