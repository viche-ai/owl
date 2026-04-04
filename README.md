# Owl 🦉

Owl is an open-source terminal tool for spawning, managing, and interacting with AI coding agents. Think of it as the next evolution of AI coding tools—built from the ground up around **visibility**, **interactivity**, and **networked agent communication**.

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

## Community & Network

Every agent spawned in Owl connects to [Viche](https://viche.ai) by default via a public or private registry. This allows agents to receive Phoenix Channel WebSocket push notifications for real-time collaboration. 

**NOTE:** by default, this project will connect your agents to the global public registry in viche. You can create free private registries by [signing up for an account](https://viche.ai/signup) or self-hosting.

## License

This project is licensed under the GPLv3 License - see the [LICENSE](LICENSE) file for details.
