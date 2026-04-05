# Owl 🦉

Owl is an open-source, terminal-native workspace to **design, run, and monitor your AI agents**. 

Rather than acting as just another coding assistant, Owl is the infrastructure that hosts them. It uses a persistent background daemon and a beautiful terminal UI (TUI) to turn isolated agents (like OpenCode, Claude Code, and OpenClaw) into a resilient, multiplayer swarm connected via the [Viche](https://viche.ai) network.

## Why Owl?

Today's agent workflows suffer from the **"3-Terminal Fallacy"**. If you run three agents in three terminal panes, they are completely isolated. They can't coordinate, you have to manually ferry context between them, and if you accidentally close the terminal, they die instantly. 

Owl changes the paradigm:
- **Design:** Craft reusable agent templates and enforce project-level guardrails.
- **Run:** Agents run inside `owld` (the background daemon) and survive terminal disconnects.
- **Monitor:** A unified TUI visualizes everything—from internal LLM loops to external sub-agents spawned by other frameworks.

## Workflows & Use Cases

Owl is built for scenarios where single-shot, isolated agents break down.

### 1. Observe and interact with external sub-agents
Tools like OpenCode and Claude Code spawn sub-agents that are often invisible to the user. Owl acts as a **Universal Visualizer**. By running your favorite orchestrators *inside* Owl, their hidden sub-agents stream their logs directly to `owld`, exposing them as native TUI tabs. You get full observability and the ability to intervene at any time.

### 2. Spawn worker swarms on the network
Need to process a backlog of issues or refactor a massive directory? Use Owl to hatch a fleet of identical worker agents (e.g., `owl hatch "Fix lint errors" -n 5`). They sit on the Viche network, pull work, and process it in parallel.

### 3. Design, manage, and port agents between repos
Stop rewriting the same system prompts. Owl allows you to define reusable Agent Templates globally (`~/.owl/config.json`), while enforcing strict, repository-specific guardrails locally (`.owl/config.json` — e.g., *"Never run `git push`"*). Your specialized reviewers and coders travel with you, adapting safely to whatever codebase they are hatched in.

### 4. Cross-machine collaboration (Multiplayer)
Because every agent hatched in Owl automatically binds to the Viche network, they aren't constrained to `localhost`. Your local frontend agent can hit an API blocker and seamlessly send a Viche message to your co-founder's backend agent running on a completely different laptop across the world to negotiate a contract change.

### 5. Persistent background execution
Kick off a massive codebase refactor, close your laptop, and go catch a train. Because agents are managed by the `owld` daemon, they keep running in the background. When you reopen your terminal and type `owl`, the TUI instantly reconnects to the live action.

---

## Architecture

Owl splits the client and daemon to allow agents to survive after you close the terminal:

- **`owld` (The Nest Daemon)**: A background process that manages agent state, LLM API connections, and real-time WebSocket channels to Viche.
- **`owl` (The TUI Client)**: A pure visualizer that connects to `owld` via Unix socket RPC, allowing you to seamlessly monitor, log, and command your agents.

## Installation

Owl is built in Go and ships as two binaries. *(Note: A one-line install script is coming soon!)*

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

**NOTE:** By default, this project will connect your agents to the global public registry in Viche. You can create free private registries by [signing up for an account](https://viche.ai/signup) or self-hosting.

## License

This project is licensed under the GPLv3 License - see the [LICENSE](LICENSE) file for details.
