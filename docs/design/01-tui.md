# TUI Design

## Overview

The Owl TUI is a terminal-based interface for managing and interacting with AI agents. It connects to the `owld` daemon over a Unix socket and renders agent state in real-time.

## Layout

```
┌──────────────────────────────────────────┬──────────────────────────┐
│                                          │  OWL NEST                │
│  ⚡ Agent Name [flying]            📋    │                          │
│                                          │  ┃ 💻 Console             │
│  Agent output streams here.              │  ┃   system               │
│  Scrollable viewport with full           │  ┃   ● ready              │
│  conversation history.                   │                          │
│                                          │  ┃ ⚡ Agent One           │
│  Includes:                               │  ┃   coder  4k/128k      │
│  - Boot logs (viche, model)              │  ┃   ● flying             │
│  - LLM streaming output                 │  ┃   ⚡ a1b2c3d4...       │
│  - Tool call invocations + results       │                          │
│  - Inbound messages (user + viche)       │  ┃ 🥚 Agent Two          │
│  - All text wraps within the viewport    │  ┃   auto  0/128k        │
│                                          │  ┃   ● hatching           │
│                                          │                          │
│                                          │  ┃ ○ Agent Three         │
│                                          │  ┃   review  8k/128k     │
│                                          │  ┃   ● idle              │
│                                          │                          │
│  ┌──────────────────────────────────┐    │                          │
│  │  > [input field]                 │    │                          │
│  └──────────────────────────────────┘    │                          │
└──────────────────────────────────────────┴──────────────────────────┘
```

### Main Pane (Left)

- Occupies `width - sidebarWidth` columns.
- Background: `PaneBg` (`#2D353B`).
- Three fixed vertical sections, top to bottom:
  1. **Header line**: Agent icon + name + state badge. Clipboard icon (`📋`) right-aligned.
  2. **Scrollable viewport**: Fills all remaining vertical space. Uses Bubbletea's `viewport` component. Auto-scrolls to bottom when new content arrives. User can scroll up with mouse wheel to review history.
  3. **Input bar**: Fixed to the bottom of the pane. Takes the full width of the main pane with horizontal margins on each side. Has visible padding around the text input area. Styled with a distinct background (`InputBg`) and a rounded border so it reads as a discrete "text box" element.

### Sidebar (Right)

- Fixed width: 40 columns.
- Background: `AppBg` (`#1E2326`).
- Contains:
  - **Header**: "OWL NEST" in green bold.
  - **Console tab** (always index 0): Yellow `💻` icon, "system", "● ready". This is the management console for running commands.
  - **Agent tabs** (index 1+): One per hatched agent. Displays:
    - State icon + name (truncated at 22 chars)
    - Role + context usage (`4k / 128k`)
    - State indicator (`● flying`, `● hatching`, `● idle`)
    - Viche ID (first 8 chars + `...`) if connected

### The "Connected Tab" Effect

The active tab's background color matches `PaneBg`, creating a visual merge with the main pane. Inactive tabs use `AppBg`. A green left border accent stripe (`BorderLeftForeground(Green)`) marks the active tab. This creates the illusion that the tab and the main pane are one continuous surface.

## Theme: Everforest Dark Hard

All colors are drawn from the Everforest Dark Hard palette:

| Name        | Hex       | Usage                                    |
|-------------|-----------|------------------------------------------|
| `AppBg`     | `#1E2326` | Deepest background (sidebar, inactive)   |
| `PaneBg`    | `#2D353B` | Main pane + active tab background        |
| `InputBg`   | `#343F44` | Input bar background                     |
| `Fg`        | `#D3C6AA` | Default text color                       |
| `Green`     | `#A7C080` | Active elements, success, headers        |
| `Yellow`    | `#DBBC7F` | Console, warnings, thinking state        |
| `Blue`      | `#7FBBB3` | Idle state, Viche IDs, info              |
| `Purple`    | `#D699B6` | Hatching state                           |
| `Red`       | `#E67E80` | Errors                                   |
| `Grey`      | `#859289` | Muted text, metadata, clipboard icon     |
| `BorderColor` | `#4A555B` | Inactive borders, input border         |

### Background Color Consistency

**All text rendered inside a pane must explicitly inherit the pane's background color.** This is a critical rule.

Lipgloss applies background colors per-style, not per-region. If a styled text block does not explicitly set its background, it inherits the terminal's default background — which creates visible "dark strips" behind text that contrast against the pane's `PaneBg`.

Rules:
- Every text style rendered inside the main pane must include `.Background(PaneBg)` or be rendered inside a parent that sets it.
- The input bar uses `.Background(InputBg)` to visually distinguish it from the content area.
- Sidebar text must use `.Background(AppBg)` for inactive tabs and `.Background(PaneBg)` for the active tab.

## Scrolling

The main pane viewport is scrollable and follows these rules:

1. **Auto-scroll**: When new content arrives (agent output, tool results, messages), the viewport scrolls to the bottom automatically. This is tracked by comparing the current log length against the last known length per agent.
2. **Manual scroll**: The user can scroll up using the mouse wheel to review earlier output. When scrolled up, auto-scroll is temporarily suspended until new content arrives.
3. **Input bar stays fixed**: The input field is rendered outside and below the viewport, so it is always visible regardless of scroll position. The viewport height is calculated as `totalHeight - headerHeight - inputBarHeight`.
4. **Console tab**: Uses a simple line-based history (not the viewport component). Automatically shows the most recent lines that fit. Scrollback is not currently supported for the console — this may change.

## Text Wrapping

All text inside the main pane viewport must wrap within the available width. No horizontal overflow.

- The viewport width is set to `mainPaneWidth - horizontalPadding`.
- Long lines from agent output, tool results, and messages must soft-wrap at the viewport boundary.
- Lipgloss `Width()` should be applied to content blocks to enforce wrapping.
- The sidebar enforces its own width constraints via `Width(sidebarWidth - padding)` on each tab.

## Interaction Model

### Navigation

| Action           | Keys              | Mouse                |
|------------------|-------------------|----------------------|
| Switch tabs      | `Tab` / `Shift+Tab` | Click sidebar tab  |
| Scroll viewport  | —                 | Mouse wheel          |
| Submit input     | `Enter`           | —                    |
| Exit             | `Ctrl+C`          | —                    |
| Copy last block  | —                 | Click 📋 icon        |

### Sidebar Click Detection

Click detection maps mouse Y-coordinates to tab indices. This calculation **must be derived from the actual rendered heights** of the sidebar elements, not hardcoded constants.

The correct approach:
1. Track the Y-offset where each tab starts during rendering.
2. Store these offsets in an array (e.g., `tabYOffsets []int`).
3. On mouse click, find which tab's Y-range contains `msg.Y`.

This eliminates drift caused by:
- Variable tab heights (e.g., tabs with/without Viche ID lines).
- Changes to header padding or margin.
- Different terminal font sizes or scaling.

**Do not use division-based index calculation** (e.g., `clickedIndex = yOffset / 6`). It is fragile and breaks when tab heights vary.

### Input Field

- **Styling**: Rounded border (`lipgloss.RoundedBorder()`), `BorderForeground(BorderColor)`, `Background(InputBg)`. Horizontal margins on each side. Internal padding around the text area.
- **Full width**: The input bar spans the full width of the main pane minus its margins.
- **Console tab**: Accepts management commands. Commands are plain text, no prefix needed.
- **Agent tabs**: Accepts messages to send to the active agent. Messages route to the agent's inbox via daemon RPC.

### Console Commands

| Command                        | Description                    |
|--------------------------------|--------------------------------|
| `hatch <description>`         | Spawn a new agent              |
| `kill <agent index or name>`  | Stop and remove an agent       |
| `viche add-registry <token>`  | Add a private Viche registry   |
| `viche set-default <token>`   | Set default registry           |
| `viche status`                | Show registry configuration    |
| `help`                        | Show available commands        |
| `clear`                       | Clear console history          |

## Thinking Animation

When an agent is processing (waiting for LLM response), the TUI displays an animated indicator rather than static text:

- Display: `Thinking ·`, `Thinking · ·`, `Thinking · · ·` (cycling).
- The animation is driven by a `tea.Tick` at ~300ms intervals.
- The animation only runs while the agent's state is `"thinking"` or while a streaming response is in progress.
- Once output begins streaming, the animation is replaced by the actual text.

This gives the user clear feedback that the agent is working, not frozen.

## Agent Session Lifecycle (TUI perspective)

- **Hatch**: User runs `hatch <desc>` in the console. A new tab appears in the sidebar in the `hatching` state.
- **Active**: Agent transitions to `flying`. User can click the tab, view output, and send messages.
- **Kill**: User runs `kill <index>` or `kill <name>` in the console. The agent is stopped, its tab is removed from the sidebar, and its resources are cleaned up on the daemon side. The agent should also be deregistered from Viche.
- **Persistence**: Agent message history and state persist in daemon memory for the lifetime of `owld`. If `owld` restarts, all agent state is lost (disk persistence is a future feature).

## State Polling

The TUI polls the daemon every 500ms via `Daemon.ListAgents` RPC. This is deliberate:
- Keeps the TUI stateless and simple — it's purely a viewer.
- Avoids the complexity of push-based state synchronization.
- 500ms is fast enough for responsive UI but light enough to avoid load.

## Clipboard

The 📋 icon in the header copies the **last output block** (not the full history) to the system clipboard:
- Agent tabs: last block separated by double newlines.
- Console tab: output from the last command executed.

ANSI escape codes are stripped before copying. A brief `✓` flash confirms the copy succeeded.

## Future Considerations

- **Text selection**: Mouse capture (`WithMouseCellMotion`) blocks native terminal text selection. Needs a toggle (e.g., a key to temporarily disable mouse capture) or an alternative copy mechanism.
- **Resize handling**: Viewport and layout should recompute cleanly on `WindowSizeMsg`. Needs testing at small terminal sizes and graceful degradation.
- **Agent grouping**: As agent count grows, the sidebar may need collapsible groups, search/filter, or pagination.
- **Theming**: Colors are currently hardcoded. Could be configurable via `~/.owl/config.json`.
- **Console scrollback**: The console tab doesn't support scrolling back through history. Should use a viewport like agent tabs.
- **Split view**: Viewing two agents side by side could be useful for observing inter-agent communication.
