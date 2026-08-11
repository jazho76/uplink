package tui

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/lima"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case logTickMsg:
		if m.mode != modeLogs {
			return m, nil
		}
		m.logView = readLogPeek(m.logName, logScreenLines)
		return m, logTickCmd()

	case tickMsg:
		return m, tea.Batch(loadCmd, tickCmd())

	case loadedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.rebuild(msg.instances)
		m.hostStats = msg.hostStats
		return m, m.onSelectionChange()

	case guestTickMsg:
		return m, tea.Batch(m.guestFetch(), guestTickCmd())

	case guestStatsMsg:
		m.guest[msg.name] = guestEntry{stats: msg.stats, at: time.Now(), err: msg.err}
		return m, nil

	case actionMsg:
		delete(m.tasks, msg.name)
		m.status = msg.status
		return m, loadCmd

	case execDoneMsg:
		m.status = msg.status
		if msg.quit {
			return m, tea.Quit
		}
		return m, loadCmd

	case tea.KeyMsg:
		switch m.mode {
		case modeConfirmDelete:
			return m.updateConfirm(msg)
		case modeLogs:
			return m.updateLogs(msg)
		default:
			return m.updateNormal(msg)
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			return m, m.onSelectionChange()
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
			return m, m.onSelectionChange()
		}

	case "enter":
		return m.connect()

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if idx := int(msg.String()[0] - '1'); idx < len(m.items) {
			m.cursor = idx
			return m.connect()
		}

	case "ctrl+l":
		if it := m.selected(); m.hasLogs(it) {
			m.mode = modeLogs
			m.logName = it.name
			m.status = ""
			m.logView = readLogPeek(it.name, logScreenLines)
			return m, logTickCmd()
		}
	case "ctrl+s":
		if it := m.selected(); it.kind == kindVM && !m.hasTask(it.name) {
			m.tasks[it.name], m.status = verbStop, ""
			return m, stopCmd(it.name)
		}
	case "ctrl+r":
		if it := m.selected(); it.kind == kindVM && !m.hasTask(it.name) {
			m.tasks[it.name], m.status = verbRestart, ""
			return m, restartCmd(it.name)
		}
	case "ctrl+a":
		if it := m.selected(); it.kind == kindVM && !m.hasTask(it.name) {
			m.tasks[it.name], m.status = verbAuto, ""
			return m, autostartCmd(it.name, !it.autostart)
		}
	case "ctrl+x":
		if it := m.selected(); it.kind == kindVM && !m.hasTask(it.name) {
			m.mode = modeConfirmDelete
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
		m.mode = modeNormal
		m.input.Blur()
		m.status = "aborted"
		return m, nil
	case "enter":
		name := m.selected().name
		confirmed := m.input.Value() == name
		m.mode = modeNormal
		m.input.Blur()
		if !confirmed {
			m.status = "aborted"
			return m, nil
		}
		m.tasks[name], m.status = verbDelete, ""
		return m, deleteCmd(name)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) connect() (tea.Model, tea.Cmd) {
	it := m.selected()

	var cmd *exec.Cmd
	if it.kind == kindHost {
		cmd = hostShellCmd()
	} else {
		cmd = exec.Command(m.self, "connect", it.name)
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return execDoneMsg{status: fmt.Sprintf("connect failed: %v", err)}
		}
		return execDoneMsg{quit: true}
	})
}

func (m model) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.mode = modeNormal
	}
	return m, nil
}

func hostShellCmd() *exec.Cmd {
	if os.Getenv("TMUX") != "" {
		return exec.Command("tmux", "new-session", "-dA", "-s", "host", ";", "switch-client", "-t", "host")
	}
	return exec.Command("tmux", "new-session", "-A", "-s", "host")
}

func stopCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if err := lima.Stop(name); err != nil {
			return actionMsg{name, fmt.Sprintf("stop failed: %v", err)}
		}
		return actionMsg{name, "stopped " + name}
	}
}

func restartCmd(name string) tea.Cmd {
	return func() tea.Msg {
		_ = lima.Stop(name)
		if err := lima.StartSilent(name); err != nil {
			return actionMsg{name, fmt.Sprintf("restart failed: %v", err)}
		}
		return actionMsg{name, "restarted " + name}
	}
}

func autostartCmd(name string, enabled bool) tea.Cmd {
	return func() tea.Msg {
		if err := lima.SetAutostart(name, enabled); err != nil {
			return actionMsg{name, fmt.Sprintf("autostart failed: %v", err)}
		}
		state := "off"
		if enabled {
			state = "on"
		}
		return actionMsg{name, fmt.Sprintf("autostart %s for %s", state, name)}
	}
}

func deleteCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if lima.AutostartEnabled(name) {
			_ = lima.SetAutostart(name, false)
		}
		_ = lima.Stop(name)
		if err := lima.Delete(name); err != nil {
			return actionMsg{name, fmt.Sprintf("delete failed: %v", err)}
		}
		return actionMsg{name, "deleted " + name}
	}
}
