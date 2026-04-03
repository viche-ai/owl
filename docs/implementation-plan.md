# Implementation Plan: Agent Lifecycle & Configuration

This plan outlines the steps required to bring the codebase in line with `docs/design/02-agent-lifecycle.md`.

## 1. Update Core Data Structures
**File:** `internal/ipc/api.go`
- Add new fields to `AgentState`:
  - `ModelID string`
  - `Thinking bool`
  - `Effort string`
  - `Verbosity string`
- Update `HatchArgs` to include:
  - `ModelID string`
  - `Template string`
  - `Thinking bool`
  - `Effort string`
  - `Name string` (override name)
- Add new RPC arguments and methods in `Service` for runtime updates:
  - `SetAgentModel(args *SetModelArgs, reply *SetModelReply)`
  - `SetAgentConfig(args *SetConfigArgs, reply *SetConfigReply)` (handles thinking, effort, verbosity)

## 2. Refactor `AgentEngine.Run` Lifecycle (Hatching Phase)
**File:** `internal/engine/engine.go`
- **Reorder Initialization:**
  - Setup LLM client first using the model resolved from `e.State.ModelID` (or config default).
  - Inject a specific system prompt instructing the model to scaffold its identity. We will request JSON output containing `name`, `capabilities` (list of strings), and `plan`.
  - Execute the scaffolding LLM call.
  - Parse the scaffolding JSON response.
- **Viche Registration:**
  - Use the newly scaffolded `name` and `capabilities` to register the agent on the Viche network.
  - Establish the WebSocket connection and setup Viche tools.
- **State Updates:** Update agent state appropriately from `hatching` to `flying` and finally `idle`.

## 3. Implement Runtime Configuration & Verbosity
**File:** `internal/engine/engine.go`
- **Model Loading:** The engine should dynamically read `e.State.ModelID`, `e.State.Thinking`, and `e.State.Effort` before each `runWithTools` call to allow runtime swapping.
- **Verbosity Levels:**
  - Define logic for `e.State.Verbosity` (`normal`, `verbose`, `debug`).
  - Modify `e.appendLog` usage or introduce `e.logVerbosity` so that only allowed levels are written to the `State.Logs` (the TUI buffer), while the disk log (`e.logFile`) continues to capture everything.
- **Thinking State:** During LLM stream waiting, ensure `e.setState("thinking")` is called to drive the TUI animation.

## 4. Implement TUI Commands
**File:** `internal/tui/input.go` (and related TUI input handling code)
- Intercept slash commands before sending messages:
  - `/model <id>` -> Calls `SetAgentModel` RPC
  - `/thinking <on|off>` -> Calls `SetAgentConfig` RPC
  - `/effort <low|medium|high>` -> Calls `SetAgentConfig` RPC
  - `/verbosity <normal|verbose|debug>` -> Calls `SetAgentConfig` RPC
  - `/models` -> Lists available models via a new or existing RPC.
- Update `internal/tui/sidebar.go` or similar to support the `"thinking"` state visualization.

## 5. Implement Templates & CLI Flags
**File:** `cmd/owl/main.go` and `cmd/owld/main.go`
- **Templates:** Implement a parser for `~/.owl/templates/*.json` (or `.yaml`).
- Add flags to the `hatch` command: `--model`, `--template`, `--registry`, `--thinking`, `--effort`, `--name`.
- When `--template` is provided, merge the template configuration with any explicit CLI flags and the user's description, then pass the final unified configuration to `owld` via `HatchArgs`.

## 6. Graceful Shutdown & Cleanup
**File:** `cmd/owld/main.go` and `internal/ipc/api.go`
- Modify the `Kill` RPC method to ensure proper cleanup:
  - The `inbox` channel is closed (already done).
  - Invoke a cleanup callback or directly deregister from Viche if possible.
  - Ensure the WebSocket channel `ch.Close()` is called.
- In `cmd/owld/main.go`, trap SIGINT/SIGTERM. On signal, iterate over all agents in `ipc.Service` and call the same cleanup logic before exiting.