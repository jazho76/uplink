package tui

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/target"
)

const (
	logInterval    = time.Second
	logScreenLines = 500
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case logTickMsg:
		if m.screen != screenLogs || m.logTailer == nil {
			return m, nil
		}
		m.logView = m.logTailer.Tail(m.logName, logScreenLines)
		return m, logTickCmd()

	case tickMsg:
		return m, tea.Batch(m.loadCmd(), tickCmd())

	case loadedMsg:
		m.rebuild(msg.targets)
		m.hostStats = msg.hostStats
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, m.onSelectionChange()

	case liveTickMsg:
		return m, tea.Batch(m.liveFetch(), liveTickCmd())

	case liveStatsMsg:
		m.live[msg.name] = liveEntry{stats: msg.stats, at: time.Now(), err: msg.err}
		return m, nil

	case actionMsg:
		delete(m.tasks, msg.name)
		m.status = msg.status
		return m, m.loadCmd()

	case execDoneMsg:
		m.status = msg.status
		if msg.quit {
			return m, tea.Quit
		}
		return m, m.loadCmd()

	case tea.KeyMsg:
		switch m.screen {
		case screenConfirm:
			return m.updateConfirm(msg)
		case screenLogs:
			return m.updateLogs(msg)
		default:
			return m.updateList(msg)
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit

	case "up", "k":
		return m, m.moveCursor(m.cursor - 1)
	case "down", "j":
		return m, m.moveCursor(m.cursor + 1)

	case "tab":
		m.cycleMode(1)
		return m, nil
	case "shift+tab":
		m.cycleMode(-1)
		return m, nil

	case "enter":
		return m.connect()

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if idx := int(msg.String()[0] - '1'); idx < len(m.items) {
			m.moveCursor(idx)
			return m.connect()
		}

	case "ctrl+l":
		if it := m.selected(); it.caps.tail {
			tailer, ok := it.provider.(target.Tailer)
			if !ok {
				return m, nil
			}
			m.screen = screenLogs
			m.logName = it.name()
			m.logTailer = tailer
			m.status = ""
			m.logView = tailer.Tail(it.name(), logScreenLines)
			return m, logTickCmd()
		}
	case "ctrl+s":
		if it := m.selected(); it.caps.lifecycle && !m.hasTask(it.name()) {
			m.tasks[it.name()], m.status = verbStop, ""
			return m, stopCmd(it.provider, it.name())
		}
	case "ctrl+r":
		if it := m.selected(); it.caps.lifecycle && !m.hasTask(it.name()) {
			m.tasks[it.name()], m.status = verbRestart, ""
			return m, restartCmd(it.provider, it.name())
		}
	case "ctrl+a":
		if it := m.selected(); it.caps.autostart && !m.hasTask(it.name()) {
			m.tasks[it.name()], m.status = verbAuto, ""
			return m, autostartCmd(it.provider, it.name(), !it.autostart)
		}
	case "ctrl+x":
		if it := m.selected(); it.caps.lifecycle && !m.hasTask(it.name()) {
			m.screen = screenConfirm
			m.input.SetValue("")
			m.input.Placeholder = ""
			m.input.Focus()
			m.status = ""
		}
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.screen = screenList
		m.input.Blur()
		m.status = "aborted"
		return m, nil
	case "enter":
		it := m.selected()
		confirmed := m.input.Value() == it.name()
		m.screen = screenList
		m.input.Blur()
		if !confirmed {
			m.status = "aborted"
			return m, nil
		}
		m.tasks[it.name()], m.status = verbDelete, ""
		return m, deleteCmd(it.provider, it.name())
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.screen = screenList
	}
	return m, nil
}

func (m model) connect() (tea.Model, tea.Cmd) {
	it := m.selected()
	if it.name() == "" {
		return m, nil
	}

	mode := m.mode()
	args := []string{"connect", it.name()}
	if mode.Name != "" {
		args = append(args, "--mode", mode.Name)
	}

	back := mode.Back
	return m, tea.ExecProcess(exec.Command(m.self, args...), func(err error) tea.Msg {
		if err != nil {
			return execDoneMsg{status: fmt.Sprintf("connect failed: %v", err)}
		}
		return execDoneMsg{quit: !back}
	})
}

func stopCmd(provider target.Provider, name string) tea.Cmd {
	return func() tea.Msg {
		lifecycle, ok := provider.(target.Lifecycle)
		if !ok {
			return actionMsg{name, "stop unsupported for " + name}
		}
		if err := lifecycle.Stop(name); err != nil {
			return actionMsg{name, fmt.Sprintf("stop failed: %v", err)}
		}
		return actionMsg{name, "stopped " + name}
	}
}

func restartCmd(provider target.Provider, name string) tea.Cmd {
	return func() tea.Msg {
		lifecycle, ok := provider.(target.Lifecycle)
		if !ok {
			return actionMsg{name, "restart unsupported for " + name}
		}
		_ = lifecycle.Stop(name)
		if err := lifecycle.Start(name, nil); err != nil {
			return actionMsg{name, fmt.Sprintf("restart failed: %v", err)}
		}
		return actionMsg{name, "restarted " + name}
	}
}

func autostartCmd(provider target.Provider, name string, on bool) tea.Cmd {
	return func() tea.Msg {
		auto, ok := provider.(target.Autostarter)
		if !ok {
			return actionMsg{name, "autostart unsupported for " + name}
		}
		if err := auto.SetAutostart(name, on); err != nil {
			return actionMsg{name, fmt.Sprintf("autostart failed: %v", err)}
		}
		state := "off"
		if on {
			state = "on"
		}
		return actionMsg{name, fmt.Sprintf("autostart %s for %s", state, name)}
	}
}

func deleteCmd(provider target.Provider, name string) tea.Cmd {
	return func() tea.Msg {
		if auto, ok := provider.(target.Autostarter); ok && auto.Autostart(name) {
			_ = auto.SetAutostart(name, false)
		}
		lifecycle, ok := provider.(target.Lifecycle)
		if !ok {
			return actionMsg{name, "delete unsupported for " + name}
		}
		_ = lifecycle.Stop(name)
		if err := lifecycle.Delete(name); err != nil {
			return actionMsg{name, fmt.Sprintf("delete failed: %v", err)}
		}
		return actionMsg{name, "deleted " + name}
	}
}
