package tui

import (
	"fmt"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		if m.screen != screenLogs {
			return m, nil
		}
		return m, tea.Batch(viewLogsCmd(m.backend, m.logTarget), logTickCmd())

	case logPeekMsg:
		if msg.name == m.selected().name() {
			m.logPeek = peek{name: msg.name, text: msg.text, at: time.Now()}
		}
		return m, nil

	case logViewMsg:
		if m.screen == screenLogs && msg.name == m.logTarget.Name {
			m.logView = msg.text
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadCmd(), tickCmd())

	case loadedMsg:
		m.hostStats = msg.hostStats
		m.hostHistory = appendSample(m.hostHistory, loadFraction(msg.hostStats.Load, msg.hostStats.Cores))
		return m, nil

	case liveTickMsg:
		return m, tea.Batch(m.liveFetch(), liveTickCmd())

	case liveStatsMsg:
		entry := liveEntry{stats: msg.stats, at: time.Now(), err: msg.err, history: m.live[msg.name].history}
		if !msg.err {
			entry.history = appendSample(entry.history, loadFraction(msg.stats.Load, msg.stats.Cores))
		}
		m.live[msg.name] = entry
		if !msg.local {
			m.recordStatus(msg.name, !msg.err)
		}
		return m, nil

	case reachMsg:
		m.live[msg.name] = liveEntry{at: time.Now(), err: !msg.reached}
		m.recordStatus(msg.name, msg.reached)
		return m, nil

	case execDoneMsg:
		m.status = msg.status
		if msg.quit {
			return m, tea.Quit
		}
		return m, m.onSelectionChange()

	case tea.KeyMsg:
		if m.screen == screenLogs {
			return m.updateLogs(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit

	case "up", "k":
		cmd := m.moveCursor(-1)
		return m, cmd
	case "down", "j":
		cmd := m.moveCursor(1)
		return m, cmd

	case "tab":
		m.cycleMode(1)
		return m, nil
	case "shift+tab":
		m.cycleMode(-1)
		return m, nil

	case "enter":
		return m.connect()

	case "ctrl+l":
		if it := m.selected(); it.caps.tail {
			m.screen = screenLogs
			m.logTarget = it.t
			m.status = ""
			m.logView = m.backend.Logs(it.t, logScreenLines)
			return m, logTickCmd()
		}

	default:
		if i, ok := paneForKey(m.panes, msg.String()); ok && i != m.focus {
			cmd := m.focusItem(i, m.panes[i].cursor)
			return m, cmd
		}
		focused, _ := m.focusedPane()
		if i, ok := itemForKey(focused, msg.String()); ok {
			cmd := m.focusItem(m.focus, i)
			return m, cmd
		}
	}
	return m, nil
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
