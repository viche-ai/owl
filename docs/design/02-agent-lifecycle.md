# Agent Lifecycle

## Overview

An Owl agent is a persistent, networked AI process managed by the `owld` daemon. Each agent has a defined lifecycle: it is hatched with a purpose, scaffolds its identity, connects to the Viche network, and then enters a conversation loop where it processes messages from the user and other agents.

## States

```
  hatch command
       │
       ▼
  ┌──────────┐     identity scaffolded     ┌──────────┐
  │ HATCHING  │ ──────────────────────────▶ │  FLYING  │
  └──────────┘     + viche registered       └──────────┘
                                               │    ▲
                                    idle/done  │    │  new message
                                               ▼    │
                                            ┌──────────┐
                                            │   IDLE   │
                                            └──────────┘
                                               │
                                          kill command
                                               │
                                               ▼
                                            ┌──────────┐
                                            │  STOPPED  │
                                            └──────────┘
```

| State       | Description                                                    | Sidebar Indicator |
|-------------|----------------------------------------------------------------|-------------------|
| `hatching`  | Agent is scaffolding its identity via LLM                      | 🥚 purple         |
| `flying`    | Agent is actively processing (thinking, calling tools, responding) | ⚡ green       |
| `idle`      | Agent has finished its current task, waiting for input         | ○ blue            |
| `thinking`  | Agent is waiting for LLM response (sub-state of flying)       | ⚡ green + animation |
| `stopped`   | Agent has been killed or `owld` shut down                     | (removed from sidebar) |

### Thinking Sub-State

When an agent is waiting for an LLM response, the sidebar should reflect this. The TUI renders a thinking animation (see `01-tui.md`) and the agent's state in `AgentState` should be set to a value that the TUI can distinguish — either a dedicated `"thinking"` state or a flag on the existing state.

When the agent receives an inbound message (from user or Viche) or begins responding, the state transitions back to `"flying"`. When the response is complete and no further work is pending, the state transitions to `"idle"`.

## Lifecycle Phases

### Phase 1: Hatching

Triggered by `hatch <description>` from the console or CLI.

**Current behavior:**
1. Daemon creates an `AgentState` struct with `State: "hatching"`.
2. Engine goroutine starts.
3. Agent registers on Viche immediately with the raw description as its name and `["owl-agent"]` as capabilities.
4. WebSocket channel opens.
5. LLM is called with a system prompt + "Initialize. What is your plan?" to scaffold the agent's identity.
6. On completion, state transitions to `"flying"`.

**Desired behavior (change):**
1. Steps 1-2 same as above.
2. The LLM scaffolding call happens FIRST, before Viche registration.
3. The scaffolding response should produce structured output: a **name**, a list of **capabilities**, and a **plan**. The system prompt should instruct the model to output these in a parseable format.
4. After scaffolding, the agent registers on Viche using the LLM-generated name and capabilities (not the raw user description).
5. WebSocket channel opens.
6. State transitions to `"flying"`.

This ensures the agent's Viche presence is meaningful — other agents can discover it by its actual capabilities rather than a generic `"owl-agent"` tag.

**Alternative (also acceptable):** Register immediately with the raw description, then re-register (or update) after scaffolding completes. This avoids delaying the network presence but requires a Viche "update registration" API or a deregister + re-register sequence.

### Phase 2: Flying

The agent is active and processing. This is the main operational state.

- The engine runs a persistent `for msg := range inbox` loop.
- Messages arrive from two sources:
  - **User**: Typed in the TUI agent input field, routed via `Daemon.SendMessage` RPC.
  - **Viche**: Pushed via Phoenix WebSocket `new_message` event.
- Each inbound message is appended to the conversation history and processed through the LLM.
- The LLM may call tools (viche_discover, viche_send, viche_reply), which are executed and fed back in a loop until the model produces a final text response.
- Tool call results are appended to the conversation history for full context.

**State transitions during flying:**
- When waiting for LLM: state = `"thinking"` (drives TUI animation)
- When streaming response: state = `"flying"`
- When response complete and inbox empty: state = `"idle"`

### Phase 3: Idle

The agent has completed its current work and is waiting. Its WebSocket channel remains connected. Inbound messages (user or Viche) will transition it back to `"flying"`.

### Phase 4: Stopped

An agent is stopped when:
- The user runs `kill <agent>` in the console.
- The `owld` daemon is shut down (all agents stop).

**Cleanup on stop:**
1. Close the inbox channel (breaks the conversation loop).
2. Deregister the agent from Viche (HTTP or channel push).
3. Close the WebSocket connection.
4. Close the log file.
5. Remove the agent from the daemon's agent list.

**On `owld` shutdown:**
- All agents are stopped and cleaned up.
- Viche connections are closed (agents auto-deregister after heartbeat timeout if cleanup fails).
- No persistence — all state is lost. This is by design for now. Future: optional disk persistence for resume-on-restart.

## Per-Agent Model Configuration

### Current behavior

All agents use the global default model from `~/.owl/config.json` (`models.default`). There is no per-agent override.

### Desired behavior

Each agent should have its own model configuration that can be changed at runtime.

**At hatch time:**
- The agent inherits the global default model.
- Optional flag: `hatch --model google/gemini-2.5-flash <description>` to override at spawn time.

**At runtime:**
- When focused on an agent tab, the user can type `/model google/gemini-2.5-pro` to switch that agent's model.
- The change takes effect on the next LLM call (not mid-stream).
- `/models` lists all configured providers and their available models.

**Implementation:**
- `AgentState` gets a `ModelID string` field.
- The engine reads `ModelID` at the start of each `processMessage` call, not just once at boot.
- The TUI sends a `Daemon.SetAgentModel` RPC call when the user runs `/model`.

### Thinking and Effort Levels

Some models support configurable thinking/reasoning effort (e.g., Anthropic's extended thinking, Google's thinking budget). Users should be able to control this:

- `/thinking on` / `/thinking off` — Enable or disable extended thinking for the focused agent.
- `/effort <low|medium|high>` — Set the reasoning effort level.

These are stored per-agent in `AgentState` and passed to the provider at call time.

### Verbosity Levels

Users should be able to control how much detail is shown in the agent's output:

| Level    | Shows                                                  |
|----------|--------------------------------------------------------|
| `normal` | Agent text responses, errors, inbound messages         |
| `verbose`| Above + tool call names and results                    |
| `debug`  | Above + full tool arguments, raw API responses         |

- Default: `verbose` (show tool calls — important for understanding agent behavior).
- `/verbosity <normal|verbose|debug>` to change per-agent.
- The verbosity setting controls what `appendLog` writes to the TUI log, not what the engine processes internally. Full logs are always written to disk.

## Message Persistence

### Daemon-lifetime persistence (current scope)

Agent state (including full conversation history and logs) persists in daemon memory for the lifetime of `owld`. If the TUI (`owl`) disconnects and reconnects, it fetches the current state and renders it. No data is lost.

**Implementation:** The TUI is stateless — it polls `Daemon.ListAgents` every 500ms. Agent logs and state live entirely in the daemon's memory. Reconnecting the TUI is seamless.

### Disk persistence (future scope)

For surviving `owld` restarts:
- Conversation history could be serialized to `~/.owl/agents/<id>/history.jsonl`.
- Agent metadata (name, model, viche ID, capabilities) to `~/.owl/agents/<id>/meta.json`.
- On startup, `owld` loads persisted agents and resumes their conversation loops.
- The log files in `~/.owl/logs/` already provide an append-only record.

## Agent Prompt Templates

### Concept

Users should be able to define reusable prompt templates that pre-configure an agent's system prompt, role, capabilities, model, and behavior.

### Template format (proposed)

Templates live in `~/.owl/templates/` as JSON or YAML files:

```json
{
  "name": "code-reviewer",
  "description": "An agent specialized in reviewing pull requests",
  "system_prompt": "You are a senior code reviewer. Focus on correctness, security, and readability. Be direct and specific in your feedback.",
  "capabilities": ["code-review", "coding"],
  "model": "anthropic/claude-sonnet-4-6",
  "thinking": true,
  "effort": "high"
}
```

### Usage

```
hatch --template code-reviewer
hatch --template code-reviewer "Review the latest PR on viche-ai/owl"
```

The template pre-fills the system prompt, capabilities, model, and thinking settings. The user's description is appended as the initial task.

### Hatch flags (full set)

| Flag                  | Description                                    |
|-----------------------|------------------------------------------------|
| `--model <id>`       | Override the default model                     |
| `--template <name>`  | Use a prompt template                          |
| `--registry <token>` | Override the Viche registry for this agent      |
| `--thinking`         | Enable extended thinking                       |
| `--effort <level>`   | Set reasoning effort (low/medium/high)         |
| `--name <name>`      | Override the agent's display name              |

## Sidebar Status Updates

The sidebar must reflect the agent's current activity in real-time:

| Activity                           | Sidebar state         |
|------------------------------------|-----------------------|
| Scaffolding identity               | 🥚 `hatching`         |
| Waiting for LLM response           | ⚡ `thinking` (animated) |
| Streaming LLM response             | ⚡ `flying`            |
| Executing a tool call              | ⚡ `flying`            |
| Receiving a Viche message          | ⚡ `flying` (auto-wake from idle) |
| Receiving a user message           | ⚡ `flying` (auto-wake from idle) |
| Done processing, inbox empty       | ○ `idle`              |
| Stopped / killed                   | (removed)             |

The state field in `AgentState` is the source of truth. The engine must update it at each transition point using `setState()`. The TUI reads it on each 500ms poll.

## Graceful Shutdown

### `owld` shutdown (SIGINT / SIGTERM)

1. Signal handler catches the signal.
2. Iterate all agents:
   a. Close inbox channels (stops conversation loops).
   b. Deregister from Viche.
   c. Close WebSocket connections.
   d. Close log files.
3. Remove the Unix socket file.
4. Exit.

### `owl` shutdown (TUI closes)

No agent impact. The TUI is stateless. Agents continue running in the daemon. The user can restart `owl` and reconnect instantly — all agents and their history are still there.

## Current vs Desired Summary

| Feature                           | Current                     | Desired                                |
|-----------------------------------|-----------------------------|----------------------------------------|
| Viche registration timing         | Before LLM scaffolding      | After scaffolding (use generated name) |
| Viche capabilities                | Hardcoded `["owl-agent"]`   | LLM-generated from description         |
| Per-agent model                   | Global only                 | Per-agent, changeable at runtime       |
| Thinking/effort                   | Not supported               | Per-agent, configurable                |
| Verbosity                         | Everything shown            | Three levels (normal/verbose/debug)    |
| Message persistence (TUI restart) | Lost                        | Persisted in daemon memory ✓           |
| Message persistence (owld restart)| Lost                        | Future: disk persistence               |
| Agent shutdown                    | No kill command             | `kill` command + graceful cleanup      |
| Prompt templates                  | Not supported               | `~/.owl/templates/` with hatch flags   |
| Sidebar state accuracy            | Basic (hatching/flying/idle)| Granular (thinking animation, auto-wake)|
| Graceful owld shutdown            | Basic signal + socket cleanup| Full agent cleanup + viche deregister  |
