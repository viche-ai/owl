package tui

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
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
type clearFlashMsg struct{}
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
	var reply ipc.SendMessageReply
	_ = c.Call("Daemon.SendMessage", &ipc.SendMessageArgs{AgentIndex: agentIndex, Content: content}, &reply)
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
	copiedFlash    bool
	thinkDots      int
	lastLogLen     map[string]int
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Run 'hatch <description>' or 'help'"
	ti.Focus()
	ti.CharLimit = 200
	ti.PromptStyle = lipgloss.NewStyle().Foreground(Green).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(Fg)

	ai := textinput.New()
	ai.Placeholder = "Send a message to this agent..."
	ai.CharLimit = 500
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
		msg, err := sendKill(agentIndex)
		if err != nil {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Error: "+err.Error()))
		} else {
			m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Green).Render(msg))
			if m.activeTab == agentIndex+1 {
				m.activeTab = 0
				m.textInput.Focus()
				m.agentInput.Blur()
			} else if m.activeTab > agentIndex+1 {
				m.activeTab--
			}
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
		m.consoleHistory = append(m.consoleHistory, "  kill <index or name>       Stop and remove an agent")
		m.consoleHistory = append(m.consoleHistory, "  viche add-registry <token> Add private registry")
		m.consoleHistory = append(m.consoleHistory, "  viche set-default <token>  Set default registry")
		m.consoleHistory = append(m.consoleHistory, "  viche status               Show registries")
		m.consoleHistory = append(m.consoleHistory, "  clear                      Clear console")

	case "clear":
		m.consoleHistory = []string{}

	default:
		m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Red).Render("Unknown: "+cmd))
	}
}

// refreshViewport updates the viewport content, dimensions, and scroll position.
// Must be called from Update() (not View()) so changes persist on the real model.
func (m *model) refreshViewport() {
	if m.width == 0 || m.activeTab <= 0 || m.activeTab-1 >= len(m.agents) {
		return
	}

	ag := m.agents[m.activeTab-1]
	sidebarWidth := 40
	mainWidth := m.width - sidebarWidth
	contentHeight := m.height - 8

	m.viewport.Width = mainWidth - 4
	m.viewport.Height = contentHeight

	logs := ag.Logs

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
		before = strings.ReplaceAll(before, "[Thinking]", staticThink)
		logs = before + animThink + after
	} else {
		logs = strings.ReplaceAll(logs, "[Thinking]", staticThink)
	}

	logs = strings.ReplaceAll(logs, "[Error]", lipgloss.NewStyle().Foreground(Red).Background(PaneBg).Render("✗ Error"))
	logs = strings.ReplaceAll(logs, "[Warning]", lipgloss.NewStyle().Foreground(Yellow).Background(PaneBg).Render("⚠ Warning"))

	m.viewport.SetContent(lipgloss.NewStyle().Foreground(Fg).Background(PaneBg).Width(mainWidth - 4).Render(logs))

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
			if m.activeTab < len(m.agents) {
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
				m.activeTab = len(m.agents)
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
		case "enter":
			if m.activeTab == 0 {
				val := strings.TrimSpace(m.textInput.Value())
				if val != "" {
					m.consoleHistory = append(m.consoleHistory, lipgloss.NewStyle().Foreground(Grey).Render("❯ ")+val)
					m.handleCommand(val)
					m.textInput.SetValue("")
				}
			} else {
				val := strings.TrimSpace(m.agentInput.Value())
				if val != "" {
					m.agentInput.SetValue("")
					sendUserMessage(m.activeTab-1, val)
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

			clipIconX := mainWidth - 5
			clipIconY := 1
			if msg.X >= clipIconX-1 && msg.X <= clipIconX+3 && msg.Y >= clipIconY-1 && msg.Y <= clipIconY+1 {
				var text string
				if m.activeTab == 0 {
					text = lastConsoleBlock(m.consoleHistory)
				} else if m.activeTab-1 < len(m.agents) {
					text = lastAgentBlock(m.agents[m.activeTab-1].Logs)
				}
				_ = clipboard.WriteAll(stripAnsi(text))
				m.copiedFlash = true
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return clearFlashMsg{} })
			}

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

	case clearFlashMsg:
		m.copiedFlash = false

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
			if m.activeTab > len(m.agents) {
				m.activeTab = len(m.agents)
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
	default:
		return Grey, "○"
	}
}

func computeTabYOffsets(agents []ipc.AgentState) []int {
	offsets := make([]int, 0, 1+len(agents))
	y := 2 // "  OWL NEST" + blank line

	// Console tab: 3 content lines + 2 padding = 5
	offsets = append(offsets, y)
	y += 5

	for _, ag := range agents {
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

	// Clipboard icon
	clipIcon := lipgloss.NewStyle().Foreground(Grey).Background(PaneBg).Render("📋")
	if m.copiedFlash {
		clipIcon = lipgloss.NewStyle().Foreground(Green).Bold(true).Background(PaneBg).Render(" ✓ ")
	}

	var mainContent string
	var rawInput string

	if m.activeTab == 0 {
		// ── Console Tab ──
		header := lipgloss.NewStyle().Foreground(Yellow).Bold(true).Background(PaneBg).Render("Console")
		spacer := lipgloss.NewStyle().Background(PaneBg).Render(
			strings.Repeat(" ", max(0, mainWidth-lipgloss.Width(header)-lipgloss.Width(clipIcon)-6)))
		headerLine := header + spacer + clipIcon

		hist := strings.Join(m.consoleHistory, "\n")
		lines := strings.Split(hist, "\n")
		if len(lines) > contentHeight {
			lines = lines[len(lines)-contentHeight:]
		}

		mainContent = headerLine + "\n\n" + strings.Join(lines, "\n")
		rawInput = m.textInput.View()
	} else {
		// ── Agent Tab ──
		idx := m.activeTab - 1
		if idx < len(m.agents) {
			ag := m.agents[idx]
			col, icon := stateStyle(ag.State)
			stateBadge := lipgloss.NewStyle().Foreground(Grey).Background(PaneBg).Render(fmt.Sprintf(" [%s]", ag.State))
			header := lipgloss.NewStyle().Foreground(col).Bold(true).Background(PaneBg).Render(fmt.Sprintf("%s %s", icon, ag.Name)) + stateBadge
			spacer := lipgloss.NewStyle().Background(PaneBg).Render(
				strings.Repeat(" ", max(0, mainWidth-lipgloss.Width(header)-lipgloss.Width(clipIcon)-6)))
			headerLine := header + spacer + clipIcon

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
		BorderForeground(BorderColor).
		Padding(0, 1).
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

	// Console tab
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

	// Agent tabs
	for i, ag := range m.agents {
		isActive := m.activeTab == i+1
		col, icon := stateStyle(ag.State)

		bg := AppBg
		if isActive {
			bg = PaneBg
		}

		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(Fg).Background(bg)
		if isActive {
			nameStyle = nameStyle.Foreground(Green)
		}

		name := ag.Name
		if len(name) > 22 {
			name = name[:19] + "..."
		}

		line1 := lipgloss.NewStyle().Foreground(col).Background(bg).Render(icon) +
			lipgloss.NewStyle().Background(bg).Render("  ") +
			nameStyle.Render(name)
		line2 := lipgloss.NewStyle().Foreground(Grey).Background(bg).Render(fmt.Sprintf("%-6s %s", ag.Role, ag.Ctx))
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
		Render(sidebarHeader + "\n\n" + strings.Join(tabs, "\n"))

	return lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidebar)
}

// ── Utilities ───────────────────────────────────────────────────────────────
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEscape = true
			continue
		}
		if inEscape {
			if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
				inEscape = false
			}
			continue
		}
		result.WriteByte(s[i])
	}
	return result.String()
}

func lastConsoleBlock(history []string) string {
	lastIdx := -1
	for i, line := range history {
		if strings.Contains(stripAnsi(line), "❯ ") {
			lastIdx = i
		}
	}
	if lastIdx >= 0 && lastIdx < len(history)-1 {
		return strings.Join(history[lastIdx+1:], "\n")
	}
	if len(history) > 0 {
		return history[len(history)-1]
	}
	return ""
}

func lastAgentBlock(logs string) string {
	blocks := strings.Split(logs, "\n\n")
	for i := len(blocks) - 1; i >= 0; i-- {
		b := strings.TrimSpace(blocks[i])
		if b != "" {
			return b
		}
	}
	return logs
}

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
