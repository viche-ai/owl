# Daemon Architecture

## Overview

`owld` is Owl's long-running background service. It owns agent state, executes agent engines, and exposes a local RPC API consumed by `owl` (CLI/TUI).

Design principle: **`owl` is a client, `owld` is the source of truth**.

That means:
- Closing/reopening `owl` should not interrupt running agents.
- Killing/restarting `owld` stops all agents.
- Multi-client support (future) should be possible because state is centralized.

## Current Architecture (Implemented)

```
┌──────────────┐     RPC over Unix socket      ┌──────────────────────────┐
│   owl CLI    │ ───────────────────────────▶  │         owld daemon      │
│   / TUI      │   /tmp/owld.sock             │                          │
└──────────────┘                                │  Service (ipc/api.go)    │
                                                │   - Agents []AgentState  │
                                                │   - InboxChans map[int]  │
                                                │                          │
                                                │  Engine goroutines        │
                                                │   - one per agent         │
                                                │   - Viche + LLM loop      │
                                                └──────────────────────────┘
```

## Responsibilities

### `owld`
- Load config (`~/.owl/config.json`)
- Build provider router
- Expose RPC methods:
  - `Daemon.Hatch`
  - `Daemon.ListAgents`
  - `Daemon.SendMessage`
  - `Daemon.Kill`
- Maintain in-memory agent state (`AgentState`)
- Maintain per-agent inbound channels (`InboxChans`)
- Spawn and supervise engine goroutines

### `owl`
- Connect to daemon via `/tmp/owld.sock`
- Render TUI / process CLI commands
- Send hatch/message/kill requests
- Poll daemon state for rendering

## IPC Contract

### Transport
- **Current:** Go net/rpc over Unix socket at `/tmp/owld.sock`
- **Pros:** Simple, fast, no auth needed for local-only
- **Cons:** Not language-neutral, brittle schema evolution, weak introspection

### Core methods

| RPC               | Purpose                              |
|-------------------|--------------------------------------|
| `Hatch(args)`     | Create agent state and start engine  |
| `ListAgents()`    | Return current daemon-held state     |
| `SendMessage()`   | Deliver user message to agent inbox  |
| `Kill()`          | Stop agent and remove it from state  |

## Agent Lifecycle Ownership

The daemon owns full lifecycle:

1. `Hatch` appends new `AgentState`
2. daemon creates buffered inbox channel
3. daemon starts engine goroutine via `RunEngineHook`
4. engine updates shared state (logs, ctx, status)
5. `Kill` closes inbox + removes state

This matches the current first-pass requirement: **agents persist across `owl` restarts, but not across `owld` restarts**.

## Current Gaps / Risks

### 1) Shutdown safety (high priority)
Current `owld` signal handler removes socket and `os.Exit(0)`.

Missing:
- Graceful agent stop sequence
- WebSocket close/deregister attempts
- Wait for goroutines to exit

**Improve:** introduce coordinated shutdown with `context.Context`, `sync.WaitGroup`, and per-agent stop hooks.

### 2) Index-based identity (high priority)
`InboxChans` keyed by slice index. Killing an agent rebuilds indexes.

Risk:
- stale references / race potential under future concurrency changes
- awkward for external API clients

**Improve:** use stable agent IDs (UUID) as primary keys:
- `map[string]*AgentRuntime`
- keep UI order separately (slice of IDs)

### 3) Concurrency model clarity (medium)
State mutation uses a single mutex callback (`Mu(func())`) and direct shared pointers to slice entries.

Works now, but becomes fragile with:
- multiple writers
- future persistence
- structured event streaming

**Improve:** define explicit runtime struct:

```go
type AgentRuntime struct {
  State   AgentState
  Inbox   chan InboundMessage
  Cancel  context.CancelFunc
  Done    chan struct{}
}
```

### 4) RPC evolution limits (medium)
Go `net/rpc` is convenient but old; no versioned schema, weak cross-platform tooling.

**Improve (later):** migrate to JSON-RPC 2.0 over Unix socket (or gRPC over UDS) once command set stabilizes.

### 5) Socket path portability (medium)
Hardcoded `/tmp/owld.sock` is simple but not ideal across OS conventions.

**Improve:**
- Linux: `${XDG_RUNTIME_DIR}/owld.sock` fallback `/tmp`
- macOS: `/tmp/owld.sock` acceptable
- configurable via `OWL_SOCKET`

### 6) Observability (medium)
Logs are per-agent text files only; daemon health is not structured.

**Improve:**
- daemon log file (`~/.owl/logs/owld.log`)
- `owl daemon status` command (uptime, agent count, socket path)
- optional JSONL event stream for debugging

## First-Pass Improvement Plan (practical)

Given your "first pass implementation" goal, these are the best ROI upgrades before deeper refactors:

1. **Graceful daemon shutdown**
2. **Stable agent IDs in daemon internals**
3. **Daemon status + health command**
4. **Basic retry wrappers for provider overload errors**
5. **Install + service management flow**

Everything else can layer in later without breaking UX.

## Service Management UX

Goal: users should never need to manually run `go run ./cmd/owld`.

### CLI commands (target)

| Command                 | Behavior |
|-------------------------|----------|
| `owl daemon start`      | Start `owld` as background service |
| `owl daemon stop`       | Stop service |
| `owl daemon restart`    | Restart service |
| `owl daemon status`     | Show running state, pid, socket, agent count |
| `owl daemon logs`       | Tail daemon logs |

### Per-OS backend

- **macOS:** LaunchAgent (`~/Library/LaunchAgents/ai.viche.owl.plist`)
- **Linux (systemd user):** `~/.config/systemd/user/owl.service`
- **Fallback:** plain background process + PID file (least preferred)

## Installation & Deployment

You want:

```bash
curl -fsSL https://get.viche.ai/owl | sh
```

That is the right end-state. Proposed rollout:

### Phase A — manual release artifacts
- Build binaries for `darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-arm64`
- Publish checksums + signed release
- Document manual install commands

### Phase B — installer script (`get.viche.ai/owl`)
Script responsibilities:
1. detect OS/arch
2. download matching tarball
3. verify checksum/signature
4. install `owl` + `owld` to `~/.local/bin` (or `/usr/local/bin` if allowed)
5. install service file (LaunchAgent/systemd user)
6. start service
7. run `owl config import openclaw || owl config import opencode` (best-effort)
8. print next-step hints (`owl`, `owl daemon status`, `owl config show`)

### Phase C — `owl upgrade`
- self-update command for binary and service unit refresh

## Security considerations for installer

- Use HTTPS + pinned checksums
- Avoid piping unsigned opaque payload directly into root shells
- Prefer non-root user installs by default
- If escalation is needed, prompt explicitly
- Keep service user-scoped by default

## What’s Missing Most (my opinion)

If I rank the biggest missing pieces right now:

1. **Production-grade daemon lifecycle** (graceful shutdown/start/status)
2. **Install/service story** (so people can actually run Owl daily)
3. **Retry and resilience in LLM path** (reduce "stuck" perception)
4. **Context window management** (prevent long-session degradation)
5. **Stable internal runtime model** (ID-based runtime map)

Once these are in, Owl moves from "great prototype" to "solid daily driver."

## Current vs Target Summary

| Area                     | Current                         | Target                              |
|--------------------------|----------------------------------|-------------------------------------|
| Daemon start             | manual `go run`                 | `owl daemon start` service-managed  |
| Shutdown                 | immediate `os.Exit`             | graceful stop with cleanup          |
| Agent runtime keys       | slice index                     | stable UUID keys                    |
| IPC                      | Go net/rpc                      | keep now; evaluate JSON-RPC later   |
| Install                  | source/build workflow           | one-line installer + service setup  |
| Health visibility        | minimal                         | daemon status + logs command        |
| Cross-session persistence| daemon-memory only              | same for now (disk later optional)  |
