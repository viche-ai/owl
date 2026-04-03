# Owl — Design Document

## Vision

Owl is an open-source terminal tool for spawning, managing, and interacting with AI coding agents. Think of it as the next evolution of tools like opencode — but built from the ground up around **visibility**, **interactivity**, and **networked agent communication**.

The core insight: today's agent workflows are black boxes. You spawn a sub-agent and have no idea what it's doing, whether it's stuck, or how to intervene. Owl makes every agent visible, inspectable, and interactive from the terminal.

## Origin & Motivation

Problems that led to this project:

1. **Invisible sub-agents** — When orchestrator agents (like OpenClaw/Geth) spawn coding drones, there's no way to see what they're doing in real-time.
2. **No guardrails visibility** — Defining tight guardrails for agents is difficult when you can't observe their behavior.
3. **No remote inspection** — If agents are running on one machine, there's no way to inspect them from another.
4. **No interactivity** — Once an agent is spawned, you can't talk to it, redirect it, or ask it questions.
5. **No discovery** — Agents can't find each other or communicate across tool boundaries.

## Core Concepts

### The Owl Metaphor

- **`owl hatch <description>`** — Spawn a new agent. It starts in a **hatching** state while it scaffolds its identity and purpose.
- **Flying** — An agent that has been scaffolded and is actively working.
- **Idle** — An agent that has completed its current task and is waiting.
- **The Nest** — The sidebar showing all active agents.

### Architecture: Client/Daemon Split

```
┌─────────────┐          ┌──────────────┐
│   owl (TUI)  │◀────────▶│  owld (Daemon)│
│  Visualizer  │  Unix    │  State +      │
│  + Input     │  Socket  │  LLM Engine + │
│              │  (RPC)   │  Viche Client │
└─────────────┘          └──────────────┘
```

- **`owld` (The Nest Daemon)** — Background process that manages agent state, LLM API connections, and Viche WebSocket channels. Persists even if the terminal closes.
- **`owl` (The TUI Client)** — Pure visualizer that connects to `owld` via Unix socket RPC. Polls agent state at 500ms intervals for live updates.

**Why this split:** Agents survive terminal disconnects. You can SSH from another machine and run `owl` to reconnect. Other tools (like OpenClaw or opencode) can talk to `owld` directly to spawn agents programmatically.

### The TUI Layout

```
┌─────────────────────────────────┬─────────────────────┐
│                                 │   OWL NEST          │
│   Agent: PR Reviewer [Flying]   │                     │
│                                 │ ┃ 💻 Console         │
│   > Viche connected             │ ┃   system           │
│   > Using model: claude-4.6     │ ┃   ● ready          │
│   ❖ Thinking                    │                     │
│   I will review the PR by...    │ ┃ ⚡ PR Reviewer     │
│                                 │ ┃   coder  4k/128k   │
│                                 │ ┃   ● flying         │
│                                 │                     │
│                                 │ ┃ 🥚 DB Migrator     │
│                                 │ ┃   setup  1k/128k   │
│   > _                           │ ┃   ● hatching       │
└─────────────────────────────────┴─────────────────────┘
```

- **Main pane (left):** Shows the active agent's scrollable log stream, or the Console for running management commands.
- **Sidebar (right):** Vertical tabs for each agent. Shows name, role, context usage, state, and Viche ID. Click to switch. Active tab's background merges with the main pane for a seamless "connected" feel.
- **Input field:** At the bottom of every pane — for commands in the Console tab, for direct messages on agent tabs.
- **Theme:** Everforest Dark Hard color palette throughout.

### Agent Lifecycle

1. **Hatch** — User runs `hatch <description>` (from CLI or TUI console).
2. **Register on Viche** — Agent immediately registers on the Viche network (public or private registry) and opens a Phoenix Channel WebSocket for real-time message delivery.
3. **Scaffold** — The LLM generates a focused plan of action (3-5 bullet points). This is the "hatching" phase.
4. **Fly** — Agent transitions to active execution. Streams its thinking and actions to the TUI in real-time.
5. **Idle** — Task complete. Agent parks and waits for further instructions or inbound Viche messages.

### Viche: First-Class Citizen

Viche is not optional — it's the nervous system. Every hatched agent **must** bind to the Viche network.

**Registry hierarchy:**
- **No account** → Public registry (works immediately, but unsafe/shared). This is the funnel — users can try Owl without signing up, but are nudged toward creating a Viche account.
- **One private registry** → All agents auto-connect to it.
- **Multiple registries** (future) → User sets a default; agents can be overridden per-hatch.

**Connection model:**
1. HTTP `POST /registry/register` → get agent UUID
2. WebSocket `wss://viche.ai/agent/websocket/websocket?agent_id=<id>&vsn=2.0.0` → Phoenix Channel
3. Join `agent:<id>` channel for direct messages (real-time push)
4. Join `registry:<token>` channel if using a private registry

No polling. Messages arrive instantly via WebSocket push.

### Multi-Provider LLM Engine

Owl supports multiple AI providers through a router pattern:

```
internal/llm/
├── provider.go    # Interface: ChatStream(ctx, model, messages) → <-chan StreamEvent
├── openai.go      # OpenAI-compatible (covers OpenAI, Google, Groq, Together, Ollama, etc.)
├── anthropic.go   # Native Anthropic Messages API (for Claude-specific features)
├── router.go      # Resolves "provider/model" → correct Provider implementation
└── discover.go    # Auto-discovers models from /v1/models endpoints
```

**Model specification:** Uses `provider/model` format (e.g., `anthropic/claude-sonnet-4-6`, `google/gemini-2.5-pro`).

**Known provider base URLs** are pre-mapped (OpenAI, Google, Groq, Together, DeepSeek, Mistral, OpenRouter). Custom endpoints just need a `baseUrl` in config.

### Configuration

Lives in `~/.owl/config.json`:

```json
{
  "models": {
    "default": "anthropic/claude-sonnet-4-6",
    "providers": {
      "anthropic": { "apiKey": "sk-ant-..." },
      "google": { "apiKey": "AIza..." }
    }
  },
  "viche": {
    "defaultRegistry": "ff271694-...",
    "registries": [
      { "token": "ff271694-..." }
    ]
  }
}
```

**Import tools:** Users can bootstrap their config by importing from existing tools:
- `owl config import openclaw`
- `owl config import opencode --path /path/to/config`

## Design Principles

1. **One command to install, one command to start.** Owl ships as a single binary. No runtime dependencies.
2. **Visibility over trust.** Never ask users to trust that an agent is doing the right thing — show them.
3. **Viche funnels users in.** Works without an account (public registry), but private registries require one. Natural conversion path.
4. **Agents are networked by default.** Every agent is addressable and discoverable on the Viche network from the moment it hatches.
5. **The terminal is the control surface.** No web UI required. Everything is inspectable and controllable from the terminal.

## Tech Stack

- **Language:** Go
- **TUI Framework:** [Bubbletea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) + [Bubbles](https://github.com/charmbracelet/bubbles)
- **IPC:** Go `net/rpc` over Unix socket (`/tmp/owld.sock`)
- **WebSocket:** [gorilla/websocket](https://github.com/gorilla/websocket) (Phoenix Channel v2 protocol)
- **CLI:** [Cobra](https://github.com/spf13/cobra)
- **Theme:** Everforest Dark Hard

## What's Built (v0.1)

- [x] TUI with Everforest theme, clickable sidebar tabs, connected-tab visual design
- [x] Client/daemon architecture with Unix socket RPC
- [x] Console tab with command input (hatch, viche, help, clear)
- [x] Agent tabs with scrollable viewport and input field
- [x] Clipboard copy via clickable 📋 icon
- [x] Live agent state polling (500ms)
- [x] Multi-provider LLM engine (OpenAI-compatible + native Anthropic)
- [x] Model auto-discovery for local/custom endpoints
- [x] Viche HTTP registration + Phoenix Channel WebSocket (real-time push)
- [x] Private registry support with CLI/TUI management
- [x] Config import from OpenClaw and opencode
- [x] CLI commands: `owl`, `owl hatch`, `owl config`, `owl viche`

## What's Next

- [ ] Agent-to-agent messaging via the TUI (send messages from agent input field)
- [ ] Persistent agent state (survive daemon restarts)
- [ ] JSONL telemetry logging for agent actions
- [ ] Remote inspection (connect `owl` to a remote `owld` over the network)
- [ ] Tool use / function calling in the LLM engine
- [ ] File system sandboxing per agent
- [ ] `owl` as a tool that other agents (OpenClaw, opencode) can invoke programmatically
- [ ] One-line install script (`curl -fsSL https://owl.sh | sh`)

---

*Document authored: 2026-04-03*
*Project: [github.com/viche-ai/owl](https://github.com/viche-ai/owl)*
