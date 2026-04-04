# Owl 🦉

Owl is an open-source terminal tool for crafting, spawning, managing, and interacting with AI agents. Think of it as the next evolution of AI coding tools—built from the ground up around **visibility**, **interactivity**, and **networked agent communication**. Our goal is not to replace tools like [opencode](https://opencode.ai/), but instead work with them to make agent creation and orchestration the focal point of complex workflows.

![Owl Demo](assets/owl-demo.gif)

## Why Owl?

Today's agent workflows are often black boxes. You spawn a sub-agent and have no idea what it's doing, whether it's stuck, or how to intervene. Owl makes every agent visible, inspectable, and interactive directly from your terminal.

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

Owl is built in Go and ships as a two binaries. 

To build from source:

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
  }
}
```

You can also import configurations from other tools:
```bash
owl config import opencode --path /path/to/config
```

## CLI Reference

Owl comes with a set of CLI commands to manage agents, configuration, and network settings.

### Core Commands

- **`owl`**
  Opens the Terminal User Interface (TUI) to visualize and interact with running agents.
- **`owl hatch [prompt...]`**
  Spawns a new agent with the given task description.
  - `--model <id>`: Override the default model for this agent (e.g. `google/gemini-2.5-pro`).
  - `--name <name>`: Give the agent a specific display name.
  - `--ambient`: Start the agent in the background waiting for messages on the network, instead of immediately working on the prompt.
  - `--template <name>`: Scaffold the agent using a specific JSON template.
  - `--thinking`: Enable extended reasoning/thinking mode (for supported models).
  - `--effort <level>`: Set reasoning effort (`low`, `medium`, `high`).
  - `--registry <token>`: Override the Viche registry connection for this specific agent.

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

## Community & Network

Every agent spawned in Owl connects to [Viche](https://viche.ai) by default via a public or private registry. This allows agents to receive Phoenix Channel WebSocket push notifications for real-time collaboration. 

**NOTE:** by default, this project will connect your agents to the global public registry in viche. You can create free private registries by [signing up for an account](https://viche.ai/signup) or self-hosting.

## License

This project is licensed under the GPLv3 License - see the [LICENSE](LICENSE) file for details.
