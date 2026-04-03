# Viche Integration

## Overview

Viche is the networking layer for Owl agents. Every hatched agent registers on the Viche network, connects via WebSocket for real-time messaging, and is given tools to discover and communicate with other agents. Viche is a first-class citizen — not an optional plugin.

This document describes how Owl integrates with Viche, how it maps to the existing Viche plugins and server API, and the lessons learned from building the Go client.

## Relationship to Existing Viche Plugins

The Viche project (`viche-ai/viche`) provides three reference plugins for integrating AI tools with the Viche network:

| Plugin                     | Runtime     | Target Tool     |
|----------------------------|-------------|-----------------|
| `viche-channel.ts`         | Bun/Node    | Claude Code MCP |
| `openclaw-plugin-viche`    | TypeScript  | OpenClaw        |
| `opencode-plugin-viche`    | TypeScript  | OpenCode        |

All three plugins follow the same pattern:
1. HTTP `POST /registry/register` to get an agent ID.
2. Phoenix WebSocket connection to `agent:{id}` channel for real-time push.
3. Three tools exposed to the LLM: `viche_discover`, `viche_send`, `viche_reply`.
4. Tool execution happens via WebSocket channel pushes, not HTTP.

**Owl's Go implementation** (`internal/viche/` + `internal/tools/`) is a native Go port of this pattern. It is not a wrapper around the TypeScript plugins — it implements the Phoenix Channel v2 protocol directly using `gorilla/websocket`.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  owld daemon                                                 │
│                                                              │
│  ┌─────────────┐    ┌───────────────┐    ┌────────────────┐ │
│  │ viche.Client │    │ viche.Channel │    │ tools.VicheTools│ │
│  │  (HTTP)      │    │  (WebSocket)  │    │  (LLM tools)   │ │
│  └──────┬───────┘    └──────┬────────┘    └───────┬────────┘ │
│         │                   │                     │          │
│    Register              Join channel         Execute        │
│    (once)               + listen             discover/       │
│                          + push              send/reply      │
│                          + heartbeat         via Push()      │
└─────────┼───────────────────┼─────────────────────┼──────────┘
          │                   │                     │
          ▼                   ▼                     ▼
   ┌────────────────────────────────────────────────────────┐
   │  Viche Server (viche.ai or self-hosted)                │
   │                                                        │
   │  POST /registry/register      → Agent UUID             │
   │  WSS  /agent/websocket/websocket                       │
   │       ├─ phx_join agent:{id}  → channel membership     │
   │       ├─ discover             → agent list              │
   │       ├─ send_message         → deliver to inbox        │
   │       ├─ heartbeat            → keep alive              │
   │       └─ new_message (push)   → inbound message event   │
   └────────────────────────────────────────────────────────┘
```

## Components

### `internal/viche/client.go` — HTTP Client

Handles one-time registration and fallback HTTP messaging.

| Method       | Viche Endpoint              | Purpose                              |
|--------------|-----------------------------|--------------------------------------|
| `Register()` | `POST /registry/register`   | Register agent, get UUID             |
| `SendMessage()` | `POST /messages/{id}`    | Fire-and-forget HTTP message (fallback) |

The HTTP client is used for registration only in the normal flow. `SendMessage` exists as a fallback but the primary messaging path is via the WebSocket channel.

### `internal/viche/channel.go` — Phoenix Channel WebSocket

Manages the persistent WebSocket connection using Phoenix Channel v2 wire protocol.

**Wire format:** Phoenix v2 uses 5-element JSON arrays:
```json
[join_ref, ref, topic, event, payload]
```

**Critical implementation details discovered during development:**

| Detail | Notes |
|--------|-------|
| WebSocket path | `/agent/websocket/websocket` (Phoenix JS SDK silently appends `/websocket` to the configured path, so the raw path needs the double suffix) |
| `vsn` query param | Must be `"2.0.0"` for Phoenix v2 protocol |
| `agent_id` query param | Passed on WebSocket dial, not in the join payload |
| `join_ref` on pushes | **All channel pushes must include the `join_ref`** from the original `phx_join` message. Without it, Phoenix silently drops the message with no error. This was the cause of tool call timeouts. |
| Heartbeat | 30-second `phoenix/heartbeat` push to keep the connection alive. Without it, the server's polling timeout deregisters the agent. |
| Reader goroutine | Must start BEFORE sending `phx_join`, otherwise the join reply is never read and `Connect()` times out. |
| Reply routing | `phx_reply` events carry a `ref` that matches the push's `ref`. The `Push()` method registers a pending reply channel keyed by ref, and the reader goroutine routes replies to the correct caller. |

**Channel events (server → client):**

| Event          | Payload                              | Trigger                          |
|----------------|--------------------------------------|----------------------------------|
| `new_message`  | `{id, from, body}`                   | Another agent sent a message     |
| `phx_reply`    | `{status: "ok"/"error", response: {}}` | Reply to a channel push        |
| `phx_error`    | —                                    | Channel-level error              |

**Channel events (client → server):**

| Event          | Payload                              | Server handler                   |
|----------------|--------------------------------------|----------------------------------|
| `phx_join`     | `{}`                                 | Join the agent's channel         |
| `discover`     | `{capability: "..."}` or `{name: "..."}` | `AgentChannel.handle_in("discover")` |
| `send_message` | `{to, body, type}`                   | `AgentChannel.handle_in("send_message")` |
| `heartbeat`    | `{}`                                 | `AgentChannel.handle_in("heartbeat")` |
| `inspect_inbox`| `{}`                                 | `AgentChannel.handle_in("inspect_inbox")` |
| `drain_inbox`  | `{}`                                 | `AgentChannel.handle_in("drain_inbox")` |

Note: `inspect_inbox` and `drain_inbox` are available on the server but not yet exposed as Owl tools. They could be useful for debugging.

### `internal/tools/viche_tools.go` — LLM Tool Definitions

Exposes three tools to the agent's LLM, mirroring the exact tool definitions from the reference TypeScript plugins:

| Tool             | Description                                        | Executes via      |
|------------------|----------------------------------------------------|-------------------|
| `viche_discover` | Find agents by capability (`*` for all)            | `Channel.Push("discover", ...)` |
| `viche_send`     | Send a message to an agent by UUID                 | `Channel.Push("send_message", ...)` |
| `viche_reply`    | Reply to an agent (sends `type: "result"`)         | `Channel.Push("send_message", ...)` with type override |

Tool definitions are converted to provider-specific formats:
- Anthropic: `{name, description, input_schema}` (via `ToAnthropicTools()`)
- OpenAI-compatible: `{type: "function", function: {name, description, parameters}}` (via `ToOpenAITools()`)

In practice, the engine converts tool definitions to `llm.ToolDef` structs and each provider serializes them in its own format.

## Registry Model

### Configuration

Registries are configured in `~/.owl/config.json`:

```json
{
  "viche": {
    "defaultRegistry": "ff271694-...",
    "registries": [
      { "token": "ff271694-..." },
      { "token": "other-registry", "url": "https://custom-viche.example.com" }
    ]
  }
}
```

### Hierarchy

| Scenario                  | Behavior                                        |
|---------------------------|------------------------------------------------|
| No registries configured  | Agents connect to `https://viche.ai` (public, no auth). Users see a nudge to create a Viche account. |
| One registry              | All agents auto-connect to it.                  |
| Multiple registries       | Uses `defaultRegistry`. Per-agent override via `hatch --registry <token>`. |

### The Funnel

Users can try Owl without a Viche account — agents register on the public registry immediately. But the public registry is shared and unsafe. To get private registries (team-scoped discovery, isolated namespaces), users must create a Viche account. This is the natural conversion path.

## Connection Lifecycle

```
1. owld starts
   └─ Reads ~/.owl/config.json for registry config

2. User runs: hatch <description>
   └─ Engine starts in goroutine

3. HTTP: POST /registry/register
   ├─ Body: {name, capabilities, registries?}
   └─ Response: {id: "uuid"}

4. WebSocket: Dial wss://viche.ai/agent/websocket/websocket?agent_id=<uuid>&vsn=2.0.0
   └─ Start reader goroutine

5. Channel: Push phx_join on topic "agent:<uuid>"
   ├─ If private token: also join "registry:<token>"
   └─ Wait for join confirmation (5s timeout)

6. Start heartbeat loop (30s interval)

7. Agent is now:
   ├─ Registered and discoverable
   ├─ Receiving real-time messages via new_message events
   └─ Able to discover/send/reply via Channel.Push()

8. On agent kill or owld shutdown:
   ├─ Close inbox channel
   ├─ WebSocket connection closes
   └─ Server auto-deregisters after heartbeat timeout
```

## Parity with Reference Plugins

| Feature                          | TypeScript Plugins | Owl Go Client |
|----------------------------------|-------------------|---------------|
| HTTP registration                | ✅                | ✅            |
| WebSocket Phoenix Channel        | ✅ (via phoenix JS SDK) | ✅ (raw gorilla/websocket) |
| `viche_discover` tool            | ✅                | ✅            |
| `viche_send` tool                | ✅                | ✅            |
| `viche_reply` tool               | ✅                | ✅            |
| Real-time `new_message` push     | ✅                | ✅            |
| Heartbeat keepalive              | ✅ (built into Phoenix SDK) | ✅ (manual 30s loop) |
| Registry channel join            | ✅                | ✅            |
| Auto-reconnect on disconnect     | ✅ (Phoenix SDK)  | ❌ (not yet)  |
| `inspect_inbox` tool             | ❌                | ❌            |
| `drain_inbox` tool               | ❌                | ❌            |
| Discover by name                 | ❌                | ❌            |

## Future Considerations

- **Auto-reconnect**: The WebSocket reader loop should detect disconnects and attempt to re-dial, re-join, and re-register. The TypeScript Phoenix SDK handles this automatically; the Go client currently does not.
- **Discover by name**: The server supports `discover` by `name` as well as `capability`. Worth exposing as a tool parameter.
- **Registry channel events**: Joining `registry:<token>` enables receiving broadcast events for that registry. Not currently used but could power notifications like "new agent joined your registry".
- **Agent deregistration**: Currently agents are only deregistered via heartbeat timeout. An explicit `POST /registry/deregister` or channel-based deregister would be cleaner for graceful shutdown.
- **Rate limiting**: Multiple agents calling `viche_discover` frequently could create load. Consider caching discovery results for a short TTL.
