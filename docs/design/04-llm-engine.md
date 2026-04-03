# LLM Engine

## Overview

The LLM engine is the core intelligence layer of Owl. It manages provider connections, routes model requests, handles streaming responses and tool calling, and maintains conversation history for each agent. The design goal is to feel as close to opencode's model configuration experience as possible while supporting Owl's multi-agent, tool-calling architecture.

## Provider Architecture

### The Provider Interface

```go
type Provider interface {
    ChatStream(ctx context.Context, model string, messages []Message) (<-chan StreamEvent, error)
    ChatStreamWithTools(ctx context.Context, model string, messages []Message, tools []ToolDef) (<-chan StreamEvent, error)
    Name() string
}
```

Every LLM backend implements this interface. The engine never talks to APIs directly — it always goes through a provider.

### Current Providers

| Provider       | Implementation   | Covers                                              |
|----------------|------------------|------------------------------------------------------|
| `anthropic`    | `anthropic.go`   | Claude models (native Messages API with tool_use)     |
| `openai` (compat) | `openai.go`  | OpenAI, Google Gemini, Groq, Together, DeepSeek, Mistral, OpenRouter, Ollama, vLLM, LM Studio, any OpenAI-compatible endpoint |

### Why Two Providers

The OpenAI-compatible protocol covers ~90% of providers. But Anthropic's API has unique features worth supporting natively:
- `tool_use` / `tool_result` content blocks (different wire format from OpenAI function calling)
- System prompt as a top-level parameter (not a message)
- Extended thinking / reasoning (future)

If a provider is named `"anthropic"` in the config, it uses the native Anthropic provider. Everything else goes through the OpenAI-compatible provider.

### The Router

The router resolves `provider/model` strings (e.g., `anthropic/claude-sonnet-4-6`) into the correct Provider implementation + bare model name.

Pre-mapped base URLs for known providers:

| Provider     | Base URL                                                   |
|--------------|-------------------------------------------------------------|
| `openai`     | `https://api.openai.com/v1`                                |
| `anthropic`  | `https://api.anthropic.com`                                |
| `google`     | `https://generativelanguage.googleapis.com/v1beta/openai`  |
| `openrouter` | `https://openrouter.ai/api/v1`                             |
| `groq`       | `https://api.groq.com/openai/v1`                           |
| `together`   | `https://api.together.xyz/v1`                               |
| `deepseek`   | `https://api.deepseek.com/v1`                               |
| `mistral`    | `https://api.mistral.ai/v1`                                 |

Custom endpoints: Any provider with a `baseUrl` in config uses that URL directly via the OpenAI-compatible provider.

## Configuration

### File: `~/.owl/config.json`

```json
{
  "models": {
    "default": "anthropic/claude-sonnet-4-6",
    "providers": {
      "anthropic": { "apiKey": "sk-ant-..." },
      "google": { "apiKey": "AIza..." },
      "openai": { "apiKey": "sk-..." },
      "openrouter": { "apiKey": "sk-or-..." },
      "local": { "baseUrl": "http://localhost:11434/v1", "apiKey": "optional" }
    }
  }
}
```

### CLI Setup Commands

Provider configuration should feel like opencode. The primary providers to support out of the box:

| Command                                  | Description                          |
|------------------------------------------|--------------------------------------|
| `owl config set-key <provider> <key>`    | Set API key for a provider           |
| `owl config set-model <provider/model>`  | Set the global default model         |
| `owl config show`                        | Show all providers, keys (masked), default model |
| `owl config import openclaw [--path]`    | Import providers from OpenClaw config |
| `owl config import opencode [--path]`    | Import providers from opencode config |

**Provider auto-setup flow (future):**
When a user runs `owl` for the first time without any config, the TUI could show an interactive setup wizard:
1. Select providers (checkboxes: Anthropic, OpenAI, Google, OpenRouter)
2. Paste API keys
3. Choose default model
4. Done — writes `~/.owl/config.json`

### Per-Agent Model Override

Each agent can run on a different model. See `02-agent-lifecycle.md` for the `/model` command and `--model` hatch flag.

Implementation: `AgentState` stores a `ModelID` field. The engine reads it at the start of each `processMessage()` call, resolving through the router each time. This allows hot-switching without restarting the agent.

## Streaming

### Stream Event Types

```go
type StreamEvent struct {
    Delta    string         // Incremental text content
    Done     bool           // Stream is complete
    Usage    *Usage         // Token counts (on final event)
    Error    error          // Stream-level error
    ToolCall *ToolCallEvent // Model is calling a tool
}
```

### SSE Parsing

Both providers parse Server-Sent Events (SSE) from the HTTP response body:
- Lines starting with `data: ` contain JSON chunks.
- `data: [DONE]` signals stream end (OpenAI format).
- Anthropic uses typed events: `content_block_delta`, `content_block_start`, `content_block_stop`, `message_stop`.

### Error Handling During Streaming

Errors can arrive mid-stream in several ways:

| Error Type                     | How It Arrives                           | Current Handling |
|--------------------------------|------------------------------------------|------------------|
| Anthropic `overloaded_error`   | SSE event with `type: "error"`           | ✅ Caught, surfaced as `StreamEvent.Error` |
| Gemini 503                     | Non-SSE JSON response body               | ✅ Caught via Content-Type check |
| Network disconnect             | Read error on SSE scanner                | ✅ Caught via `scanner.Err()` |
| Malformed JSON chunks          | Unmarshal failure                        | ✅ Silently skipped (continue) |
| HTTP 4xx/5xx                   | Before streaming starts                  | ✅ Caught, returned as error |

## Tool Calling

### The Tool Loop

When the LLM calls tools, the engine runs a loop:

```
1. Send messages + tool definitions to LLM
2. Stream response
3. If response contains tool calls:
   a. Append assistant message (with tool_use refs) to history
   b. Execute each tool
   c. Append tool results to history
   d. Go to step 1
4. If no tool calls: return text response
```

### Anthropic Tool Format

Anthropic requires a specific message structure for tool calling:
- Assistant messages that call tools must contain `tool_use` content blocks (not just text).
- `tool_result` messages are `role: "user"` with a `content` array containing `{type: "tool_result", tool_use_id, content}` blocks.
- Each `tool_result` must reference a `tool_use_id` from the immediately preceding assistant message.

The `Message` struct carries `ToolCalls []ToolCallRef` to track this:
```go
type Message struct {
    Role       string
    Content    string
    ToolCallID string        // For tool result messages
    ToolCalls  []ToolCallRef // For assistant messages with tool_use
}
```

The Anthropic provider serializes these into the correct content block format.

### OpenAI Tool Format

OpenAI uses `function_call` / `tool_calls` in the delta, with `role: "tool"` messages for results. The OpenAI provider handles this natively.

## Retry System

### Design (to be implemented)

When a provider returns a retryable error, the engine should automatically retry with exponential backoff:

| Error Type              | Retryable? | Strategy                              |
|-------------------------|------------|---------------------------------------|
| `overloaded_error`      | Yes        | Backoff: 2s, 4s, 8s (max 3 retries)  |
| `rate_limit_error`      | Yes        | Respect `Retry-After` header if present, else backoff |
| 429 Too Many Requests   | Yes        | Respect `Retry-After` header          |
| 503 Service Unavailable | Yes        | Backoff: 2s, 4s, 8s                   |
| 500 Internal Error      | Yes (once) | Single retry after 2s                 |
| 400 Bad Request         | No         | Surface error immediately             |
| 401 Unauthorized        | No         | Surface error immediately             |
| Network timeout         | Yes        | Single retry after 2s                 |

Implementation approach:
- Wrap the `ChatStream` / `ChatStreamWithTools` calls in a retry loop inside the engine's `runWithTools()`.
- Log each retry attempt to the agent's output so the user can see what's happening.
- Configurable max retries (default 3) and base delay (default 2s).

### Streaming Retry Consideration

If an error occurs mid-stream (after some tokens have been emitted), the partial response is already in the log. Retrying would produce duplicate output. Options:
1. **Don't retry mid-stream** — only retry if the error occurs before any deltas are emitted.
2. **Retry with a note** — log `"> Retrying..."` and accept some duplication.

Recommendation: Option 1 for now. Only retry on connection/pre-stream errors.

## Context Window Management

### The Problem

Each agent maintains a `[]Message` conversation history that grows indefinitely. Eventually it will exceed the model's context window, causing API errors.

### Current State

No management. The `Ctx` field in `AgentState` shows a rough token count from API usage reports, but there's no truncation, summarization, or window management.

### Proposed Approach

1. **Token estimation**: Maintain a running token count based on API usage responses. For Anthropic, `input_tokens` + `output_tokens` gives exact counts. For OpenAI-compatible, `usage.total_tokens`.

2. **Soft limit warning**: When context reaches 80% of the model's limit, log a warning to the agent's output: `"> Context window at 80% (102k/128k). Consider starting a new agent."`.

3. **Automatic truncation**: When context reaches 90%, truncate older messages while preserving:
   - The system prompt (always kept).
   - The last N user/assistant turns (configurable, default 10).
   - Tool call/result pairs are kept together or dropped together.

4. **Summarization (future)**: Before truncating, summarize the dropped messages into a condensed "memory" block that gets prepended to the system prompt. This preserves context without consuming as many tokens.

### Model Context Limits

The engine should know each model's context window size. This can be:
- Hardcoded for known models (claude-sonnet-4-6 = 200k, gemini-2.5-flash = 1M, etc.)
- Configurable in the provider config
- Queried via `/v1/models` endpoint where available

## Provider-Specific Parameters

Different models support different generation parameters. The engine should pass these through when configured per-agent:

| Parameter         | Anthropic          | OpenAI/Gemini       | Per-Agent Config  |
|-------------------|--------------------|---------------------|-------------------|
| Temperature       | `temperature`      | `temperature`       | `/temperature 0.7` |
| Max tokens        | `max_tokens`       | `max_tokens`        | `/max-tokens 4096` |
| Extended thinking | (future API)       | —                   | `/thinking on`    |
| Reasoning effort  | —                  | (model-dependent)   | `/effort high`    |
| Top-p             | `top_p`            | `top_p`             | Future            |

These are stored in `AgentState` and read by the provider at call time.

## System Prompt Management

### Current State

The system prompt is constructed inline in `engine.go` using `fmt.Sprintf`. It includes:
- The agent's purpose (from hatch description)
- Viche ID and network info
- Tool descriptions
- Instruction to output a plan

### Desired State

The system prompt should be composed from layers:

1. **Base prompt**: Core identity and behavior instructions.
2. **Viche context**: Network connection info, agent ID, available tools.
3. **Template overlay** (if using a prompt template): User-defined system prompt additions.
4. **Agent-generated identity** (after hatching): Name, capabilities, plan from the scaffolding step.

These layers are concatenated at prompt construction time. The Viche context layer is added automatically and should not be part of user-editable templates.

## Current vs Desired Summary

| Feature                        | Current                    | Desired                              |
|--------------------------------|----------------------------|--------------------------------------|
| Providers                      | Anthropic + OpenAI-compat  | Same (covers the big 3-4)            |
| Configuration CLI              | `set-key`, `set-model`, `show`, `import` | Same + interactive setup wizard |
| Model format                   | `provider/model`           | Same                                 |
| Streaming                      | ✅ Working                 | Same                                 |
| Tool calling                   | ✅ Working (Anthropic + OpenAI) | Same                            |
| Retry on overload              | ❌ Errors surface immediately | Exponential backoff with logging  |
| Context window management      | ❌ None                    | Token tracking + truncation          |
| Per-agent model override       | ❌ Global only             | Per-agent, hot-swappable             |
| Provider-specific params       | ❌ Hardcoded max_tokens    | Configurable per-agent               |
| System prompt composition      | Hardcoded sprintf          | Layered composition                  |
| Token counting                 | Basic (from API usage)     | Running count + context % display    |
