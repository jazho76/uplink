package tui

import (
	"cmp"
	"errors"
	"fmt"
	"os/exec"
	"slices"
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
		if m.screen != screenLogs || msg.round != m.logRound {
			return m, nil
		}
		return m, m.viewLogsCmd()

	case logPeekMsg:
		if msg.name == m.selected().name() {
			m.logPeek = peek{name: msg.name, text: msg.text, at: time.Now()}
		}
		return m, nil

	case logViewMsg:
		if m.screen != screenLogs || msg.round != m.logRound {
			return m, nil
		}
		m.logView = msg.text
		return m, logTickCmd(m.logRound)

	case tickMsg:
		return m, tea.Batch(hostCmd(), tickCmd())

	case liveTickMsg:
		return m, tea.Batch(m.probeSelected(), m.peekSelected(), m.watchFetch(), liveTickCmd())

	case hostStatsMsg:
		m.host.stats = msg.stats
		m.host.history = appendSample(m.host.history, loadFraction(msg.stats.Load, msg.stats.Cores))
		return m, nil

	case liveMsg:
		m.live[msg.name] = m.settle(msg)
		return m, nil

	case hookDoneMsg:
		e := m.live[msg.name]
		e.action, e.at = "", time.Time{}
		e.epoch++
		m.live[msg.name] = e
		if msg.err != nil {
			m.status = fmt.Sprintf("%s %s failed: %v", msg.verb, msg.name, msg.err)
		}
		t, ok := m.targetNamed(msg.name)
		if !ok {
			return m, nil
		}
		return m, m.check(t, t.Name == m.selected().name())

	case configEditedMsg:
		return m.reload(msg.status)

	case execDoneMsg:
		m.status = msg.status
		m.forgetProbe(m.selected().name())
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

	case "ctrl+s":
		return m.runHook(true)
	case "ctrl+x":
		return m.runHook(false)

	case "ctrl+e":
		return m, tea.ExecProcess(exec.Command(m.self, "config", "edit"), configEdited)

	case "ctrl+l":
		if it := m.selected(); it.t.Logs != "" {
			m.screen = screenLogs
			m.logTarget = it.t
			m.logRound++
			m.status = ""
			m.logView = m.logsFor(it)
			return m, m.viewLogsCmd()
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

	name := it.name()
	return m, tea.ExecProcess(exec.Command(m.self, args...), func(err error) tea.Msg {
		return sessionEnded(name, err)
	})
}

func sessionEnded(name string, err error) execDoneMsg {
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return execDoneMsg{}
	case errors.As(err, &exitErr):
		return execDoneMsg{status: fmt.Sprintf("%s: session ended with exit %d", name, exitErr.ExitCode())}
	default:
		return execDoneMsg{status: fmt.Sprintf("%s: %v", name, err)}
	}
}

func configEdited(err error) tea.Msg {
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return configEditedMsg{status: fmt.Sprintf("config edit: %v", err)}
	}
	return configEditedMsg{}
}

func (m model) reload(editStatus string) (tea.Model, tea.Cmd) {
	targets, problems := m.load()
	for _, t := range targets {
		if previous, ok := m.targetNamed(t.Name); ok && !slices.Equal(previous.Stats, t.Stats) {
			e := m.live[t.Name]
			e.readings, e.statsOK = nil, false
			m.live[t.Name] = e
		}
	}
	m.targets, m.modeIdx = targets, 0
	m.rebuild()
	m.forgetProbe(m.selected().name())
	m.status = cmp.Or(editStatus, configStatus(problems))
	return m, tea.Batch(m.onSelectionChange(), m.watchFetch())
}

func (m model) forgetProbe(name string) {
	if e, ok := m.live[name]; ok {
		e.at = time.Time{}
		m.live[name] = e
	}
}
