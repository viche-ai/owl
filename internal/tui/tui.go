package tui

import (
	"encoding/json"
	"fmt"
	"net/rpc"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/viche-ai/owl/internal/config"
	"github.com/viche-ai/owl/internal/ipc"
)

// ── Everforest Dark Hard ────────────────────────────────────────────────────
var (
	AppBg       = lipgloss.Color("#1E2326")
	PaneBg      = lipgloss.Color("#2D353B")
	InputBg     = lipgloss.Color("#343F44")
	Fg          = lipgloss.Color("#D3C6AA")
	Green       = lipgloss.Color("#A7C080")
	Yellow      = lipgloss.Color("#DBBC7F")
	Blue        = lipgloss.Color("#7FBBB3")
	Purple      = lipgloss.Color("#D699B6")
	Grey        = lipgloss.Color("#859289")
	BorderColor = lipgloss.Color("#4A555B")
	Red         = lipgloss.Color("#E67E80")
)

// ── Messages ────────────────────────────────────────────────────────────────
type tickMsg time.Time
type thinkTickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func thinkTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg { return thinkTickMsg(t) })
}

// ── Daemon RPC ──────────────────────────────────────────────────────────────
func fetchAgents() ([]ipc.AgentState, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.ListReply
	err = c.Call("Daemon.ListAgents", &ipc.ListArgs{}, &reply)
	return reply.Agents, err
}

func sendUserMessage(agentIndex int, content string) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return
	}
	defer func() { _ = c.Close() }()
	workDir, _ := os.Getwd()
	var reply ipc.SendMessageReply
	_ = c.Call("Daemon.SendMessage", &ipc.SendMessageArgs{AgentIndex: agentIndex, Content: content, WorkDir: workDir}, &reply)
}

func sendHatch(desc string) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.HatchReply
	err = c.Call("Daemon.Hatch", &ipc.HatchArgs{Description: desc}, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

func sendKill(agentIndex int) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.KillReply
	err = c.Call("Daemon.Kill", &ipc.KillArgs{AgentIndex: agentIndex}, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

func sendClone(agentIndex int) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.CloneResponse
	err = c.Call("Daemon.CloneAgent", &ipc.CloneRequest{AgentIndex: agentIndex}, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

func sendStop(runID string, force bool) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.StopReply
	err = c.Call("Daemon.StopAgent", &ipc.StopArgs{RunID: runID, Force: force}, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

func sendRemove(runID string, archive bool) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.RemoveReply
	err = c.Call("Daemon.RemoveAgent", &ipc.RemoveArgs{RunID: runID, Archive: archive, Force: !archive}, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

//lint:ignore U1000 unused
func sendAgentConfig(agentIndex int, config ipc.SetConfigArgs) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	config.AgentIndex = agentIndex
	var reply ipc.SetConfigReply
	err = c.Call("Daemon.SetAgentConfig", &config, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

//lint:ignore U1000 unused
func sendAgentModel(agentIndex int, modelID string) (string, error) {
	c, err := rpc.Dial("unix", "/tmp/owld.sock")
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	var reply ipc.SetModelReply
	err = c.Call("Daemon.SetAgentModel", &ipc.SetModelArgs{AgentIndex: agentIndex, ModelID: modelID}, &reply)
	if err != nil {
		return "", err
	}
	return reply.Message, nil
}

//lint:ignore U1000 unused
func (m *model) handleAgentCommand(agentIndex int, cmdStr string) {
	parts := strings.SplitN(cmdStr, " ", 2)
	cmd := parts[0]

	switch cmd {
	case "/model":
		if len(parts) < 2 {
			return
		}
		modelID := strings.TrimSpace(parts[1])
		_, _ = sendAgentModel(agentIndex, modelID)
	case "/thinking":
		if len(parts) < 2 {
			return
		}
		arg := strings.TrimSpace(parts[1])
		val := arg == "on" || arg == "true"
		_, _ = sendAgentConfig(agentIndex, ipc.SetConfigArgs{Thinking: &val})
	case "/effort":
		if len(parts) < 2 {
			return
		}
		val := strings.TrimSpace(parts[1])
		_, _ = sendAgentConfig(agentIndex, ipc.SetConfigArgs{Effort: &val})
	case "/verbosity":
		if len(parts) < 2 {
			return
		}
		val := strings.TrimSpace(parts[1])
		_, _ = sendAgentConfig(agentIndex, ipc.SetConfigArgs{Verbosity: &val})
	}
}

// ── Model ───────────────────────────────────────────────────────────────────
type model struct {
	width, height  int
	agents         []ipc.AgentState
	activeTab      int // 0 = Console, 1+ = Agents
	daemonErr      error
	textInput      textinput.Model
	agentInput     textinput.Model
	consoleHistory []string
	viewport       viewport.Model
	thinkDots      int
	lastLogLen     map[string]int
}

// isMetaPresent returns true when the Owl meta-agent occupies agents[0].
func (m *model) isMetaPresent() bool {
	return len(m.agents) > 0 && m.agents[0].Name == "owl"
}

// activeAgentIndex maps the current activeTab to the correct index in m.agents.
// When the meta-agent is present: Tab 0 → agents[0], Tab 1 → agents[1], etc.
// Without meta-agent (legacy): Tab 1 → agents[0], Tab 2 → agents[1], etc.
func (m *model) activeAgentIndex() int {
	if m.isMetaPresent() {
		return m.activeTab
	}
	return m.activeTab - 1
}

// maxTab returns the highest valid tab index.
func (m *model) maxTab() int {
	if m.isMetaPresent() {
		return len(m.agents) - 1
	}
	return len(m.agents)
}

// isConsoleShortcut returns true if val starts with a recognized console shortcut command.
// Shortcuts still work at Tab 0 even when the meta-agent is present.
func isConsoleShortcut(val string) bool {
	parts := strings.SplitN(val, " ", 2)
	switch parts[0] {
	case "hatch", "kill", "clone", "viche", "help", "clear":
		return true
	}
	return false
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Talk to Owl, or use 'hatch <desc>' to spawn an agent"
	ti.Focus()
	ti.CharLimit = 4096
	ti.PromptStyle = lipgloss.NewStyle().Foreground(Green).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(Fg)

	ai := textinput.New()
	ai.Placeholder = "Send a message to this agent..."
	ai.CharLimit = 4096
	ai.PromptStyle = lipgloss.NewStyle().Foreground(Blue).Bold(true)
	ai.TextStyle = lipgloss.NewStyle().Foreground(Fg)

	agents, err := fetchAgents()
	vp := viewport.New(80, 20)

	return model{
		activeTab:      0,
		agents:         agents,
		daemonErr:      err,
		textInput:      ti,
		agentInput:     ai,
		consoleHistory: []string{"Owl Nest Console initialized.", "Type 'help' to see available commands."},
		viewport:       vp,
		lastLogLen:     make(map[string]int),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.SetWindowTitle("Owl"), textinput.Blink, tick(), thinkTick())
}

// ── Console command handler ─────────────────────────────────────────────────
func (m *model) handleCommand(cmdStr string) {
	parts := strings.SplitN(cmdStr, " ", 2)
	cmd := parts[0]

	switch cmd {
	case "hatch":
		if len(parts) < 2 {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Usage: hatch <description>"))
			return
		}
		desc := strings.Trim(parts[1], "\"' ")
		msg, err := sendHatch(desc)
		if err != nil {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
		} else {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render(msg))
		}

	case "kill":
		if len(parts) < 2 {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Usage: kill <index or name>"))
			return
		}
		arg := strings.TrimSpace(parts[1])
		agentIndex := -1
		if n, err := strconv.Atoi(arg); err == nil {
			agentIndex = n - 1
		} else {
			for i, ag := range m.agents {
				if strings.EqualFold(ag.Name, arg) {
					agentIndex = i
					break
				}
			}
		}
		if agentIndex < 0 || agentIndex >= len(m.agents) {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Agent not found: "+arg))
			return
		}
		// Protect the meta-agent from being killed via the shortcut
		if agentIndex == 0 && m.isMetaPresent() {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Cannot kill the meta-agent (owl)."))
			return
		}
		msg, err := sendKill(agentIndex)
		if err != nil {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
		} else {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render(msg))
			// Compute the tab index for the killed agent
			killedTab := agentIndex + 1
			if m.isMetaPresent() {
				killedTab = agentIndex
			}
			if m.activeTab == killedTab {
				m.activeTab = 0
				m.textInput.Focus()
				m.agentInput.Blur()
			} else if m.activeTab > killedTab {
				m.activeTab--
			}
		}

	case "clone":
		if len(parts) < 2 {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Usage: clone <index or name>"))
			return
		}
		arg := strings.TrimSpace(parts[1])
		agentIndex := -1
		if n, err := strconv.Atoi(arg); err == nil {
			agentIndex = n - 1
		} else {
			for i, ag := range m.agents {
				if strings.EqualFold(ag.Name, arg) {
					agentIndex = i
					break
				}
			}
		}
		if agentIndex < 0 || agentIndex >= len(m.agents) {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Agent not found: "+arg))
			return
		}
		msg, err := sendClone(agentIndex)
		if err != nil {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
		} else {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render(msg))
		}

	case "viche":
		if len(parts) < 2 {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Usage: viche <add-registry|set-default|status> [token]"))
			return
		}
		subparts := strings.SplitN(parts[1], " ", 2)
		subcmd := subparts[0]
		switch subcmd {
		case "add-registry":
			if len(subparts) < 2 {
				m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Usage: viche add-registry <token>"))
				return
			}
			token := strings.TrimSpace(subparts[1])
			if err := config.AddRegistry(token, ""); err != nil {
				m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
			} else {
				short := token
				if len(short) > 8 {
					short = short[:8] + "..."
				}
				m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render("✓ Registry added: "+short))
			}
		case "set-default":
			if len(subparts) < 2 {
				m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Usage: viche set-default <token>"))
				return
			}
			token := strings.TrimSpace(subparts[1])
			if err := config.SetDefaultRegistry(token); err != nil {
				m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
			} else {
				short := token
				if len(short) > 8 {
					short = short[:8] + "..."
				}
				m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render("✓ Default: "+short))
			}
		case "status":
			cfg, _ := config.Load()
			if len(cfg.Viche.Registries) == 0 {
				m.consoleHistory = append(m.consoleHistory, "No private registries. Using public registry.")
			} else {
				for _, r := range cfg.Viche.Registries {
					url := r.URL
					if url == "" {
						url = "https://viche.ai"
					}
					short := r.Token
					if len(short) > 8 {
						short = short[:8] + "..."
					}
					mark := ""
					if r.Token == cfg.Viche.DefaultRegistry {
						mark = " ← default"
					}
					m.consoleHistory = append(m.consoleHistory, "  "+short+" ("+url+")"+mark)
				}
			}
		default:
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Unknown: viche "+subcmd))
		}

	case "help":
		m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Yellow).Render("Commands:"))
		m.consoleHistory = append(m.consoleHistory, "  hatch <desc>               Spawn a new agent")
		m.consoleHistory = append(m.consoleHistory, "  clone <index or name>      Clone an existing agent")
		m.consoleHistory = append(m.consoleHistory, "  kill <index or name>       Stop and remove an agent")
		m.consoleHistory = append(m.consoleHistory, "  viche add-registry <token> Add private registry")
		m.consoleHistory = append(m.consoleHistory, "  viche set-default <token>  Set default registry")
		m.consoleHistory = append(m.consoleHistory, "  viche status               Show registries")
		m.consoleHistory = append(m.consoleHistory, "  clear                      Clear console")
		m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Yellow).Render("Agent tab shortcuts:"))
		m.consoleHistory = append(m.consoleHistory, "  alt+c   Clone selected agent")
		m.consoleHistory = append(m.consoleHistory, "  alt+s   Graceful stop")
		m.consoleHistory = append(m.consoleHistory, "  alt+k   Force stop (immediate kill)")
		m.consoleHistory = append(m.consoleHistory, "  alt+x   Remove/archive agent")

	case "clear":
		m.consoleHistory = []string{}

	default:
		m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Unknown: "+cmd))
	}
}

// refreshViewport updates the viewport content, dimensions, and scroll position.
// Must be called from Update() (not View()) so changes persist on the real model.
func (m *model) refreshViewport() {
	if m.width == 0 {
		return
	}

	// Determine which agent's logs to display.
	// With meta-agent: Tab 0 → agents[0], Tab 1 → agents[1], …
	// Without meta-agent: Tab 0 has no agent (console history), Tab 1 → agents[0], …
	var agIdx int
	if m.activeTab == 0 {
		if !m.isMetaPresent() {
			return // console history view, no viewport needed
		}
		agIdx = 0
	} else {
		agIdx = m.activeAgentIndex()
		if agIdx < 0 || agIdx >= len(m.agents) {
			return
		}
	}

	ag := m.agents[agIdx]
	sidebarWidth := 40
	mainWidth := m.width - sidebarWidth
	contentHeight := m.height - 8

	m.viewport.Width = mainWidth - 4
	m.viewport.Height = contentHeight

	logs := formatAgentLogs(ag.Logs)

	// Thinking animation: only animate the LAST [Thinking] if it's the trailing content.
	// All earlier [Thinking] markers become static since they already have responses.
	staticThink := lipgloss.NewStyle().Foreground(Yellow).Background(PaneBg).Render("❖ Thinking")
	trimmed := strings.TrimRight(logs, " \n.")
	isThinking := strings.HasSuffix(trimmed, "[Thinking]")

	if isThinking {
		dots := []string{"·", "· ·", "· · ·"}[m.thinkDots]
		animThink := lipgloss.NewStyle().Foreground(Yellow).Background(PaneBg).Render("Thinking " + dots)
		lastIdx := strings.LastIndex(logs, "[Thinking]")
		before := logs[:lastIdx]
		after := logs[lastIdx+len("[Thinking]"):]
		before = strings.ReplaceAll(before, "[Thinking]...", staticThink)
		// 'after' contains the rest of the string starting AFTER '[Thinking]'. If it starts with '...', strip it.
		after = strings.TrimPrefix(after, "...")
		logs = before + animThink + after
	} else {
		logs = strings.ReplaceAll(logs, "[Thinking]...", staticThink)
		logs = strings.ReplaceAll(logs, "[Thinking]", staticThink)
	}

	logs = strings.ReplaceAll(logs, "[Error]", lipgloss.NewStyle().Foreground(Red).Background(PaneBg).Render("✗ Error"))
	logs = strings.ReplaceAll(logs, "[Warning]", lipgloss.NewStyle().Foreground(Yellow).Background(PaneBg).Render("⚠ Warning"))

	// Split logs by line and explicitly render each line with the background
	// so that lipgloss properly reapplies the background after any nested ANSI resets.
	logLines := strings.Split(logs, "\n")
	for i, l := range logLines {
		logLines[i] = lipgloss.NewStyle().Foreground(Fg).Background(PaneBg).Width(mainWidth - 4).Render(l)
	}
	m.viewport.SetContent(strings.Join(logLines, "\n"))

	// Auto-scroll to bottom when new content arrives
	logLen := len(ag.Logs)
	if prev, ok := m.lastLogLen[ag.ID]; !ok || logLen != prev {
		m.viewport.GotoBottom()
		m.lastLogLen[ag.ID] = logLen
	}
}

// ── Update ──────────────────────────────────────────────────────────────────
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.activeTab < m.maxTab() {
				m.activeTab++
			} else {
				m.activeTab = 0
			}
			if m.activeTab == 0 {
				m.textInput.Focus()
				m.agentInput.Blur()
			} else {
				m.agentInput.Focus()
				m.textInput.Blur()
			}
			m.refreshViewport()
			return m, nil
		case "shift+tab":
			if m.activeTab > 0 {
				m.activeTab--
			} else {
				m.activeTab = m.maxTab()
			}
			if m.activeTab == 0 {
				m.textInput.Focus()
				m.agentInput.Blur()
			} else {
				m.agentInput.Focus()
				m.textInput.Blur()
			}
			m.refreshViewport()
			return m, nil
		case "alt+c":
			// Clone selected agent (alt+c)
			if m.activeTab > 0 {
				agentIndex := m.activeAgentIndex()
				msg, err := sendClone(agentIndex)
				if err != nil {
					m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
				} else {
					m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render(msg))
				}
			}
			return m, nil

		case "alt+s":
			// Graceful stop of selected agent (alt+s)
			if m.activeTab > 0 {
				agentIndex := m.activeAgentIndex()
				if agentIndex >= 0 && agentIndex < len(m.agents) {
					runID := m.agents[agentIndex].RunID
					if runID != "" {
						msg, err := sendStop(runID, false)
						if err != nil {
							m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
						} else {
							m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Yellow).Render(msg))
						}
					}
				}
			}
			return m, nil

		case "alt+k":
			// Force-stop of selected agent (alt+k)
			if m.activeTab > 0 {
				agentIndex := m.activeAgentIndex()
				if agentIndex >= 0 && agentIndex < len(m.agents) {
					runID := m.agents[agentIndex].RunID
					if runID != "" && m.agents[agentIndex].Name != "owl" {
						msg, err := sendStop(runID, true)
						if err != nil {
							m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
						} else {
							m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render(msg))
						}
					}
				}
			}
			return m, nil

		case "alt+x":
			// Remove/archive selected agent (alt+x)
			if m.activeTab > 0 {
				agentIndex := m.activeAgentIndex()
				if agentIndex >= 0 && agentIndex < len(m.agents) {
					ag := m.agents[agentIndex]
					if ag.RunID != "" && ag.Name != "owl" {
						msg, err := sendRemove(ag.RunID, true)
						if err != nil {
							m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
						} else {
							m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Grey).Render(msg))
							// Return to console tab
							m.activeTab = 0
							m.textInput.Focus()
							m.agentInput.Blur()
						}
					}
				}
			}
			return m, nil
		case "enter":
			if m.activeTab == 0 {
				val := strings.TrimSpace(m.textInput.Value())
				if val != "" {
					m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Grey).Render("❯ ")+val)
					// When the meta-agent is present: shortcuts still work locally;
					// everything else is forwarded to the meta-agent's inbox.
					if m.isMetaPresent() && !isConsoleShortcut(val) {
						sendUserMessage(0, val)
					} else {
						m.handleCommand(val)
					}
					m.textInput.SetValue("")
				}
			} else {
				val := strings.TrimSpace(m.agentInput.Value())
				if val != "" {
					m.agentInput.SetValue("")
					sendUserMessage(m.activeAgentIndex(), val)
				}
			}
			return m, nil
		}

		if m.activeTab == 0 {
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			var cmd tea.Cmd
			m.agentInput, cmd = m.agentInput.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			sidebarWidth := 40
			mainWidth := m.width - sidebarWidth

			if msg.X >= mainWidth {
				offsets := computeTabYOffsets(m.agents)
				for i, startY := range offsets {
					endY := m.height
					if i+1 < len(offsets) {
						endY = offsets[i+1]
					}
					if msg.Y >= startY && msg.Y < endY {
						m.activeTab = i
						if m.activeTab == 0 {
							m.textInput.Focus()
							m.agentInput.Blur()
						} else {
							m.agentInput.Focus()
							m.textInput.Blur()
						}
						m.refreshViewport()
						break
					}
				}
			}
		}

		// Forward scroll events to viewport
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

	case thinkTickMsg:
		m.thinkDots = (m.thinkDots + 1) % 3
		m.refreshViewport()
		cmds = append(cmds, thinkTick())

	case tickMsg:
		agents, err := fetchAgents()
		if err != nil {
			m.daemonErr = err
		} else {
			m.daemonErr = nil
			m.agents = agents
			if m.activeTab > m.maxTab() {
				m.activeTab = m.maxTab()
			}
		}
		m.refreshViewport()
		cmds = append(cmds, tick())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		sidebarWidth := 40
		mainWidth := m.width - sidebarWidth
		inputInnerWidth := mainWidth - 8
		if inputInnerWidth > 0 {
			m.textInput.Width = inputInnerWidth
			m.agentInput.Width = inputInnerWidth
		}
		m.refreshViewport()
	}

	return m, tea.Batch(cmds...)
}

// ── View helpers ────────────────────────────────────────────────────────────
func stateStyle(state string) (lipgloss.Color, string) {
	switch state {
	case "flying":
		return Green, "⚡"
	case "thinking":
		return Yellow, "·"
	case "idle":
		return Blue, "○"
	case "hatching":
		return Purple, "🥚"
	case "stopped":
		return Grey, "■"
	case "archived":
		return Grey, "□"
	case "error":
		return Red, "✗"
	default:
		return Grey, "○"
	}
}

func computeTabYOffsets(agents []ipc.AgentState) []int {
	offsets := make([]int, 0, 1+len(agents))
	y := 3 // top padding + "  OWL NEST" + blank line

	hasMeta := len(agents) > 0 && agents[0].Name == "owl"

	// Tab 0: meta-agent or console (3 content lines + 2 padding = 5)
	offsets = append(offsets, y)
	y += 5

	// User agent tabs: skip agents[0] when meta-agent is present
	for i, ag := range agents {
		if i == 0 && hasMeta {
			continue
		}
		offsets = append(offsets, y)
		lines := 3
		if ag.VicheID != "" {
			lines++
		}
		y += lines + 2
	}

	return offsets
}

func renderTab(content string, active bool, width int) string {
	s := lipgloss.NewStyle().
		Width(width-2).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		Padding(1, 2).
		MarginBottom(0)

	if active {
		return s.Background(PaneBg).BorderLeftForeground(Green).Render(content)
	}
	return s.Background(AppBg).BorderLeftForeground(BorderColor).Render(content)
}

// ── View ────────────────────────────────────────────────────────────────────
func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	sidebarWidth := 40
	mainWidth := m.width - sidebarWidth
	contentHeight := m.height - 8

	if m.daemonErr != nil {
		return lipgloss.NewStyle().Width(m.width).Height(m.height).
			Background(AppBg).Foreground(Red).Align(lipgloss.Center).
			PaddingTop(m.height / 3).
			Render(fmt.Sprintf("Cannot reach owld daemon.\n\n%v\n\nRun: owld", m.daemonErr))
	}

	var mainContent string
	var rawInput string

	if m.activeTab == 0 && m.isMetaPresent() {
		// ── Meta-agent Tab (Tab 0 with owl at agents[0]) ──
		ag := m.agents[0]
		col, icon := stateStyle(ag.State)
		stateBadge := lipgloss.NewStyle().Foreground(Grey).Background(PaneBg).Render(fmt.Sprintf(" [%s]", ag.State))
		header := lipgloss.NewStyle().Foreground(col).Bold(true).Background(PaneBg).Render(fmt.Sprintf("%s owl", icon)) + stateBadge
		spacer := lipgloss.NewStyle().Background(PaneBg).Render(
			strings.Repeat(" ", max(0, mainWidth-lipgloss.Width(header)-4)))
		headerLine := header + spacer
		mainContent = headerLine + "\n\n" + m.viewport.View()
		rawInput = m.textInput.View()
	} else if m.activeTab == 0 {
		// ── Console Tab (no meta-agent) ──
		header := lipgloss.NewStyle().Foreground(Yellow).Bold(true).Background(PaneBg).Render("Console")
		spacer := lipgloss.NewStyle().Background(PaneBg).Render(
			strings.Repeat(" ", max(0, mainWidth-lipgloss.Width(header)-4)))
		headerLine := header + spacer

		hist := strings.Join(m.consoleHistory, "\n")
		lines := strings.Split(hist, "\n")
		if len(lines) > contentHeight {
			lines = lines[len(lines)-contentHeight:]
		}

		mainContent = headerLine + "\n\n" + strings.Join(lines, "\n")
		rawInput = m.textInput.View()
	} else {
		// ── Agent Tab ──
		idx := m.activeAgentIndex()
		if idx >= 0 && idx < len(m.agents) {
			ag := m.agents[idx]
			col, icon := stateStyle(ag.State)
			stateBadge := lipgloss.NewStyle().Foreground(Grey).Background(PaneBg).Render(fmt.Sprintf(" [%s]", ag.State))
			harnessBadge := ""
			if ag.Harness != "" {
				harnessBadge = lipgloss.NewStyle().Foreground(Blue).Background(PaneBg).Render(fmt.Sprintf(" [harness:%s]", ag.Harness))
			}
			header := lipgloss.NewStyle().Foreground(col).Bold(true).Background(PaneBg).Render(fmt.Sprintf("%s %s", icon, ag.Name)) + stateBadge + harnessBadge
			spacer := lipgloss.NewStyle().Background(PaneBg).Render(
				strings.Repeat(" ", max(0, mainWidth-lipgloss.Width(header)-4)))
			headerLine := header + spacer

			// Viewport content is managed by refreshViewport() in Update()
			mainContent = headerLine + "\n\n" + m.viewport.View()
			rawInput = m.agentInput.View()
		}
	}

	// Input bar: rounded border, InputBg background
	inputInnerWidth := mainWidth - 8
	if inputInnerWidth < 1 {
		inputInnerWidth = 1
	}
	inputBox := lipgloss.NewStyle().
		Width(inputInnerWidth).
		Background(InputBg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor).BorderBackground(PaneBg).
		Padding(0, 0, 0, 1).
		Render(rawInput)
	inputBar := lipgloss.NewStyle().Background(PaneBg).PaddingLeft(2).Render(inputBox)

	mainPane := lipgloss.NewStyle().
		Width(mainWidth).
		Height(m.height).
		Background(PaneBg).
		Foreground(Fg).
		Padding(1, 2).
		Render(mainContent + "\n\n" + inputBar)

	// ── Sidebar ──
	var tabs []string
	hasMeta := m.isMetaPresent()

	// Tab 0: meta-agent (when present) or console fallback
	if hasMeta {
		ag := m.agents[0]
		col, _ := stateStyle(ag.State)
		bg := AppBg
		if m.activeTab == 0 {
			bg = PaneBg
		}
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(Fg).Background(bg)
		if m.activeTab == 0 {
			nameStyle = nameStyle.Foreground(Green)
		}
		line1 := lipgloss.NewStyle().Foreground(col).Background(bg).Render("🦉") +
			lipgloss.NewStyle().Background(bg).Render("  ") +
			nameStyle.Render("owl")
		line2 := lipgloss.NewStyle().Foreground(Grey).Background(bg).Render("meta-agent")
		line3 := lipgloss.NewStyle().Foreground(col).Background(bg).Render("● " + ag.State)
		metaContent := line1 + "\n" + line2 + "\n" + line3
		tabs = append(tabs, renderTab(metaContent, m.activeTab == 0, sidebarWidth))
	} else {
		consoleBg := AppBg
		if m.activeTab == 0 {
			consoleBg = PaneBg
		}
		cNameStyle := lipgloss.NewStyle().Bold(true).Foreground(Fg).Background(consoleBg)
		if m.activeTab == 0 {
			cNameStyle = cNameStyle.Foreground(Green)
		}
		cContent := lipgloss.NewStyle().Foreground(Yellow).Background(consoleBg).Render("💻") +
			lipgloss.NewStyle().Background(consoleBg).Render("  ") +
			cNameStyle.Render("Console") + "\n" +
			lipgloss.NewStyle().Foreground(Grey).Background(consoleBg).Render("system") + "\n" +
			lipgloss.NewStyle().Foreground(Yellow).Background(consoleBg).Render("● ready")
		tabs = append(tabs, renderTab(cContent, m.activeTab == 0, sidebarWidth))
	}

	// Agent tabs: skip the meta-agent (agents[0]) when it occupies Tab 0
	for i, ag := range m.agents {
		if i == 0 && hasMeta {
			continue // meta-agent is shown at Tab 0 above
		}

		// Compute the tab index for this agent
		tabIdx := i + 1
		if hasMeta {
			tabIdx = i // Tab 1 = agents[1] when meta-agent present
		}
		isActive := m.activeTab == tabIdx
		col, _ := stateStyle(ag.State)

		bg := AppBg
		if isActive {
			bg = PaneBg
		}

		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(Fg).Background(bg)
		if isActive {
			nameStyle = nameStyle.Foreground(Green)
		}

		nameIcon := "○"
		if ag.State == "hatching" {
			nameIcon = "🥚"
		} else if isActive {
			nameIcon = "●"
		}

		name := ag.Name
		if len(name) > 22 {
			name = name[:19] + "..."
		}

		line1 := lipgloss.NewStyle().Foreground(col).Background(bg).Render(nameIcon) +
			lipgloss.NewStyle().Background(bg).Render("  ") +
			nameStyle.Render(name)
		meta := strings.ReplaceAll(fmt.Sprintf("%-6s %s", ag.Role, ag.Ctx), "\n", " ")
		if ag.Harness != "" {
			meta = fmt.Sprintf("harness:%s", ag.Harness)
		}
		if len(meta) > sidebarWidth-6 {
			meta = meta[:sidebarWidth-9] + "..."
		}
		line2 := lipgloss.NewStyle().Foreground(Grey).Background(bg).Render(meta)
		line3 := lipgloss.NewStyle().Foreground(col).Background(bg).Render("● " + ag.State)
		content := line1 + "\n" + line2 + "\n" + line3

		if ag.VicheID != "" {
			short := ag.VicheID
			if len(short) > 8 {
				short = short[:8] + "..."
			}
			content += "\n" + lipgloss.NewStyle().Foreground(Blue).Background(bg).Render("⚡ "+short)
		}

		tabs = append(tabs, renderTab(content, isActive, sidebarWidth))
	}

	sidebarHeader := lipgloss.NewStyle().
		Foreground(Green).Bold(true).Background(AppBg).
		Render("  OWL NEST")

	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(m.height).
		Background(AppBg).
		Render("\n" + sidebarHeader + "\n\n" + strings.Join(tabs, "\n"))

	return lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidebar)
}

// ── Utilities ───────────────────────────────────────────────────────────────

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Run ─────────────────────────────────────────────────────────────────────
func Run() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

// ── Agent Log Formatting ────────────────────────────────────────────────────
var (
	jsonKeyRe = regexp.MustCompile(`"([^"]+)"(\s*:)`)
	jsonStrRe = regexp.MustCompile(`(:\s*)("([^"\\\\]|\\\\.)*")`)
	jsonNumRe = regexp.MustCompile(`(:\s*)([0-9\.\-]+|true|false|null)`)
)

func formatAgentLogs(raw string) string {
	toolRe := regexp.MustCompile(`(?m)^> \[((?:Native )?Tool): ([^\]]+)\]\s*(\{)`)

	var processed strings.Builder
	lastEnd := 0

	idxs := toolRe.FindAllStringSubmatchIndex(raw, -1)
	for _, idx := range idxs {
		startMatch := idx[0]
		prefixStart := idx[2]
		prefixEnd := idx[3]
		nameStart := idx[4]
		nameEnd := idx[5]
		startBrace := idx[6]

		isNative := false
		if prefixStart != -1 && prefixEnd != -1 {
			isNative = strings.Contains(raw[prefixStart:prefixEnd], "Native")
		}

		toolName := raw[nameStart:nameEnd]

		openCount := 0
		endBrace := -1
		for i := startBrace; i < len(raw); i++ {
			if raw[i] == '{' {
				openCount++
			} else if raw[i] == '}' {
				openCount--
				if openCount == 0 {
					endBrace = i
					break
				}
			}
		}

		if endBrace != -1 {
			processed.WriteString(raw[lastEnd:startMatch])

			jsonStr := raw[startBrace : endBrace+1]

			var obj map[string]interface{}
			prettyJSON := jsonStr
			if err := json.Unmarshal([]byte(jsonStr), &obj); err == nil {
				if formatted, err := json.MarshalIndent(obj, "", "  "); err == nil {
					prettyJSON = string(formatted)
				}
			}

			s := prettyJSON
			s = jsonKeyRe.ReplaceAllString(s, lipgloss.NewStyle().Foreground(Blue).Background(PaneBg).Render("\"$1\"")+"$2")
			s = jsonStrRe.ReplaceAllStringFunc(s, func(m string) string {
				parts := jsonStrRe.FindStringSubmatch(m)
				if len(parts) > 2 {
					return parts[1] + lipgloss.NewStyle().Foreground(Green).Background(PaneBg).Render(parts[2])
				}
				return m
			})
			s = jsonNumRe.ReplaceAllStringFunc(s, func(m string) string {
				parts := jsonNumRe.FindStringSubmatch(m)
				if len(parts) > 2 {
					return parts[1] + lipgloss.NewStyle().Foreground(Purple).Background(PaneBg).Render(parts[2])
				}
				return m
			})

			icon := "🔧"
			if isNative {
				icon = "⚙️"
			}

			outcome := "Running"
			borderColor := Blue

			lookahead := raw[endBrace:]
			// The engine prints e.g. "> Tool execution completed: Success\n"
			if idx := strings.Index(lookahead, "> Tool execution completed: "); idx != -1 {
				// verify it's for this tool call by ensuring no other tool call started before it
				nextToolIdx := strings.Index(lookahead, "> [Tool: ")
				if nextToolIdx == -1 || idx < nextToolIdx {
					nlIdx := strings.Index(lookahead[idx:], "\n")
					if nlIdx != -1 {
						statusStr := lookahead[idx+len("> Tool execution completed: ") : idx+nlIdx]
						if strings.Contains(statusStr, "Failed") {
							outcome = "Failed"
							borderColor = Red
						} else if strings.Contains(statusStr, "Success") {
							outcome = "Success"
							borderColor = Green
						}
					}
				}
			}

			headerLeft := lipgloss.NewStyle().Foreground(Fg).Background(PaneBg).Bold(true).Render(fmt.Sprintf("%s Tool call: %s", icon, toolName))
			var outcomeStyle lipgloss.Style
			switch outcome {
			case "Running":
				outcomeStyle = lipgloss.NewStyle().Foreground(Blue).Background(PaneBg).Italic(true)
			case "Success":
				outcomeStyle = lipgloss.NewStyle().Foreground(Green).Background(PaneBg)
			default:
				outcomeStyle = lipgloss.NewStyle().Foreground(Red).Background(PaneBg).Bold(true)
			}

			headerRight := outcomeStyle.Render(fmt.Sprintf("[%s]", outcome))
			header := headerLeft + " " + headerRight

			content := header + "\n" + s

			block := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(borderColor).
				BorderBackground(PaneBg).
				PaddingLeft(1).
				Background(PaneBg).
				Render(content)

			processed.WriteString(block + "\n")
			lastEnd = endBrace + 1
		}
	}
	processed.WriteString(raw[lastEnd:])

	rawLines := processed.String()
	lines := strings.Split(rawLines, "\n")
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "> Tool execution completed:") {
			continue
		}

		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
				formatted, err := json.MarshalIndent(obj, "", "  ")
				if err == nil {
					s := string(formatted)
					s = jsonKeyRe.ReplaceAllString(s, lipgloss.NewStyle().Foreground(Blue).Background(PaneBg).Render("\"$1\"")+"$2")
					s = jsonStrRe.ReplaceAllStringFunc(s, func(m string) string {
						parts := jsonStrRe.FindStringSubmatch(m)
						if len(parts) > 2 {
							return parts[1] + lipgloss.NewStyle().Foreground(Green).Background(PaneBg).Render(parts[2])
						}
						return m
					})
					s = jsonNumRe.ReplaceAllStringFunc(s, func(m string) string {
						parts := jsonNumRe.FindStringSubmatch(m)
						if len(parts) > 2 {
							return parts[1] + lipgloss.NewStyle().Foreground(Purple).Background(PaneBg).Render(parts[2])
						}
						return m
					})

					block := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(BorderColor).BorderBackground(PaneBg).PaddingLeft(1).Background(PaneBg).Render(s)
					out = append(out, block)
					continue
				}
			}
		}

		if strings.HasPrefix(trimmed, "=>") || strings.HasPrefix(trimmed, "> ") {
			if !strings.HasPrefix(trimmed, "\x1b[") {
				line = lipgloss.NewStyle().Foreground(Grey).Background(PaneBg).Render(line)
			}
		} else if strings.HasPrefix(trimmed, "User:") || strings.HasPrefix(trimmed, "Agent:") {
			if !strings.HasPrefix(trimmed, "\x1b[") {
				line = lipgloss.NewStyle().Foreground(Yellow).Bold(true).Background(PaneBg).Render(line)
			}
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}
