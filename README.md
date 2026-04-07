# Owl 🦉

Owl is an open-source terminal tool for crafting, spawning, managing, and interacting with AI agents. Think of it as the next evolution of AI coding tools—built from the ground up around **visibility**, **interactivity**, and **networked agent communication**. Our goal is not to replace tools like [opencode](https://opencode.ai/), but instead work with them to make agent creation and orchestration the focal point of complex workflows.

![Owl Demo](assets/owl-demo.gif)

## Why Owl?

Today's agent workflows are often black boxes. You spawn a sub-agent and have no idea what it's doing, whether it's stuck, or how to intervene. Hell, I don't even know what my OpenClaw is doing half the time. Owl makes every agent visible, inspectable, and interactive directly from your terminal.

- **Visibility over trust**: Never ask users to trust that an agent is doing the right thing — show them in real-time. Even agents can spin up visible agents with `owl hatch [prompt]`.
- **Client/Daemon Architecture**: Agents run in `owld` (the Nest Daemon) and survive terminal disconnects. `owl` (the TUI Client) visualizes them.
- **Viche Integration**: Every hatched agent binds to the Viche network, enabling remote inspection, discovery, and communication.
- **Multi-Provider**: Native support for Anthropic, OpenAI, Google, Groq, Together, Ollama, and more.
- **Terminal Native**: No web UI required. Everything is inspectable and controllable from a beautiful TUI powered by Bubble Tea.

## Architecture

Owl splits the client and daemon to allow agents to survive after you close the terminal:

- **`owld` (The Nest Daemon)**: A background process that manages agent state, LLM API connections, and real-time WebSocket channels to Viche.
- **`owl` (The TUI Client)**: A pure visualizer that connects to `owld` via Unix socket RPC, allowing you to seamlessly monitor, log, and command your agents.

## Installation

### Quick Install (Recommended)

```bash
curl -fsSL https://owl.viche.ai/install.sh | bash
```

This installs `owl` and `owld` to `~/.local/bin` and sets up the launchd service on macOS (or systemd on Linux) to run the daemon in the background.

### Build from Source

Owl is built in Go and ships as two binaries.

```bash
git clone https://github.com/viche-ai/owl.git
cd owl
make build
```

This will produce two binaries in the `bin/` directory: `owl` and `owld`.

## Getting Started

1. **Start the daemon:**
   ```bash
   ./bin/owld
   ```

2. **Open the TUI:**
   ```bash
   # Separate terminal
   ./bin/owl
   ```

3. **Hatch an Agent** (from CLI or the TUI console):
   ```bash
   owl hatch "Write a python script to parse logs"
   ```

## Configuration

Configuration lives in `~/.owl/config.json`. You can set your default models, provider API keys, and Viche registry tokens.

### Interactive Setup

For first-time configuration, run the interactive setup wizard:

```bash
owl setup
```

This will guide you through:
1. Selecting your default LLM provider (OpenAI, Anthropic, Google, Ollama, or Custom)
2. Entering your API key for that provider
3. Optionally joining a private Viche registry (recommended for security)

### Manual Configuration

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
    "defaultRegistry": "public"
  },
  "system_prompt": "You are a collaborative AI agent operating on the Viche network..."
}
```

You can also import configurations from other tools:
```bash
owl config import opencode --path /path/to/config
```

### Tiered System Prompts

Owl assembles the system prompt in layers when hatching an agent, giving you precise control over agent context at each level:

1. **[GLOBAL]** — From `~/.owl/config.json` (`system_prompt` field). Applies to all agents network-wide. Default: "You are a collaborative AI agent operating on the Viche network..."

2. **[PROJECT CONTEXT]** — From `.owl/project.json` (`context` field). Applies to all agents working in this repository. Set via `owl project init` or by editing the file directly.

3. **[PROJECT GUARDRAILS]** — From `.owl/project.json` (`guardrails` array). Critical constraints agents must follow. Set via `owl project guards`.

4. **[RUNTIME]** — The specific task description passed via `owl hatch <prompt>`.

Example `project.json`:
```json
{
  "version": "1.0",
  "context": "This is a React TypeScript project using Next.js and Tailwind CSS",
  "guardrails": [
    "Always use TypeScript strict mode",
    "Never commit secrets or credentials",
    "Follow React hooks rules strictly"
  ]
}
```

## CLI Reference

Owl comes with a set of CLI commands to manage agents, configuration, and network settings.

### Core Commands

- **`owl`**
  Opens the Terminal User Interface (TUI) to visualize and interact with running agents.
- **`owl hatch [prompt...]`**
  Spawns a new agent with the given task description.
  - `-n, --count <number>`: Spawn multiple agents simultaneously (default: 1). Use this for parallel task execution.
  - `--model <id>`: Override the default model for this agent (e.g. `google/gemini-2.5-pro`).
  - `--name <name>`: Give the agent a specific display name.
  - `--ambient`: Start the agent in the background waiting for messages on the network, instead of immediately working on the prompt.
  - `--template <name>`: Scaffold the agent using a specific JSON template.
  - `--thinking`: Enable extended reasoning/thinking mode (for supported models).
  - `--effort <level>`: Set reasoning effort (`low`, `medium`, `high`).
  - `--registry <token>`: Override the Viche registry connection for this specific agent.
  - `--dir <path>`: Set the working directory for the agent. Enables project-level context and guardrails from `.owl/project.json`, and sets the root for file operations.
  - `--harness <name>`: Run an external coding harness instead of Owl's built-in LLM loop. Supported: `codex`, `opencode`, `claude-code`.
  - `--harness-args "..."`: Optional extra args passed to the selected harness. For safety, shell metacharacters are rejected.
  - `--no-network-inject`: Disable injection of Owl/Viche environment variables (`OWL_LOCAL_AGENT_ID`, `OWL_HARNESS`, and registry token when available).

- **`owl clone <agent_id>`**
  Clones an existing agent, spawning a new one with identical parameters (model, prompt, configuration, working directory).
  - `-n, --count <number>`: Spawn multiple clones simultaneously (default: 1).
  
  The `<agent_id>` can be an agent index (shown in the TUI sidebar) or an agent name.
  
  In the TUI, press `c` while viewing an agent tab to clone it directly.

### Configuration (`owl config`)

Manage your local model settings and API keys.

- **`owl config show`**
  Displays your current configuration, default model, and configured providers.
- **`owl config set-key <provider> <api_key>`**
  Saves an API key for a specific provider (e.g., `anthropic`, `google`, `openai`).
- **`owl config set-model <provider/model>`**
  Sets the default model used when hatching new agents (e.g., `anthropic/claude-sonnet-4-6`).
- **`owl config import <source>`**
  Imports API keys from other tools. Currently supports `opencode` and `openclaw`.

### Network (`owl viche`)

Manage your connections to the [Viche](https://viche.ai) agent network.

- **`owl viche status`**
  Shows your current active registries and which one is set as default.
- **`owl viche add-registry <token>`**
  Adds a private Viche registry authentication token. Use `--url` to specify a custom/self-hosted registry endpoint.
- **`owl viche set-default <token>`**
  Sets an existing registry token as the default for all newly hatched agents.

### Project (`owl project`)

Also aliased as `owl nest`, in case you enjoy fun.

Manage project-level configuration, templates, and agent guidelines for the current directory.

- **`owl project init`**
  Initialize a local Owl project in the current directory. Creates `.owl/project.json` and `.owl/templates/`.
- **`owl project agents`**
  Run an interactive wizard to describe agent functionality and create `.owl/AGENTS.md`. This file defines agent roles, capabilities, and workflows for the project.
- **`owl project guards`**
  Run an interactive interview to generate project guardrails. Saved to `.owl/project.json` (for agent context) and `.owl/GUARDS.md` (human-readable). Guards define constraints agents must respect (e.g., never push to production).
- **`owl project templates list`**
  List available project templates in `.owl/templates/`.
- **`owl project templates create <name>`**
  Create a new project template with an interactive wizard.
- **`owl project templates delete <name>`**
  Delete a project template.

## Community & Network

Every agent spawned in Owl connects to [Viche](https://viche.ai) by default via a public or private registry. This allows agents to receive Phoenix Channel WebSocket push notifications for real-time collaboration. 

**NOTE:** by default, this project will connect your agents to the global public registry in viche. You can create free private registries by [signing up for an account](https://viche.ai/signup) or self-hosting.

## External Stream API

Owl can visualize logs from external tools (scripts, other agents like OpenCode or OpenClaw) by exposing an HTTP endpoint on the `owld` daemon. External tools can POST events to create dynamic "tabs" in the Owl TUI and stream logs in real-time.

### Endpoint

```
POST http://localhost:7890/api/v1/stream
```

### Event Schema

```json
{
  "agent_id": "unique-agent-id",
  "agent_name": "My Script",
  "state": "flying",
  "log_line": "Starting process..."
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent_id` | string | Yes | Unique identifier for the external agent |
| `agent_name` | string | Yes | Display name shown in the TUI sidebar |
| `state` | string | No | Agent state (e.g., `flying`, `idle`). Defaults to `flying` |
| `log_line` | string | No | A single log line to append. Send multiple events to stream |

### Behavior

- Sending an event for a new `agent_id` creates a new sidebar tab in the TUI
- Subsequent events with the same `agent_id` append log lines to the existing tab
- External agents appear alongside native Owl agents and are styled with `role: external`
- The TUI polls for updates every 500ms, so logs appear with minimal delay

### Example: Streaming Logs from a Script

```bash
# Stream logs from an external script
echo '{"agent_id":"build-1","agent_name":"build-script","state":"flying","log_line":"Compiling..."}' | curl -X POST http://localhost:7890/api/v1/stream -H "Content-Type: application/json" -d @-

# In a Node.js script
fetch('http://localhost:7890/api/v1/stream', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    agent_id: 'my-agent',
    agent_name: 'My Agent',
    state: 'flying',
    log_line: 'Step 1 complete'
  })
});
```

## License

This project is licensed under the GPLv3 License - see the [LICENSE](LICENSE) file for details.
