package tui

import (
	"cmp"
	"errors"
	"fmt"
	"os/exec"
	"slices"
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
		if m.screen != screenLogs || msg.round != m.logRound {
			return m, nil
		}
		return m, m.viewLogsCmd()

	case logPeekMsg:
		if msg.id == m.selected().id() {
			m.logPeek = peek{id: msg.id, text: msg.text, at: time.Now()}
		}
		return m, nil

	case logViewMsg:
		if m.screen != screenLogs || msg.round != m.logRound {
			return m, nil
		}
		m.logView = msg.text
		return m, logTickCmd(m.logRound)

	case liveTickMsg:
		return m, tea.Batch(m.checkSelected(), m.peekSelectedLogs(), m.checkWatched(), liveTickCmd())

	case liveMsg:
		m.live[msg.id] = m.settle(msg)
		return m, nil

	case hookDoneMsg:
		connect := msg.action == statusStarting && msg.err == nil &&
			m.connectAfterStart == msg.id && m.selected().id() == msg.id
		if m.connectAfterStart == msg.id {
			m.connectAfterStart = target.ID{}
		}
		e := m.live[msg.id]
		e.action, e.statsAt = "", time.Time{}
		e.epoch++
		m.live[msg.id] = e
		t, ok := m.targetByID(msg.id)
		if msg.err != nil {
			m.notice = fmt.Sprintf("%s %s failed: %v", m.hookFor(t, msg.action).verb, msg.id, msg.err)
		}
		if !ok {
			return m, nil
		}
		recheck := m.check(t, t.ID == m.selected().id())
		if connect {
			return m, tea.Batch(recheck, m.session())
		}
		return m, recheck

	case configEditedMsg:
		return m.reload(msg.notice)

	case execDoneMsg:
		m.notice = msg.notice
		m.markStale(m.selected().id())
		return m, m.refreshSelected()

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
		return m.runHook(statusStarting)
	case "ctrl+x":
		return m.runHook(statusStopping)

	case "ctrl+e":
		cmd, err := m.edit()
		if err != nil {
			m.notice = fmt.Sprintf("config edit: %v", err)
			return m, nil
		}
		return m, m.exec(cmd, configEdited)

	case "ctrl+l":
		if it := m.selected(); it.target.Logs != "" {
			m.screen = screenLogs
			m.logTarget = it.target
			m.logRound++
			m.notice = ""
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
	if it.empty() {
		return m, nil
	}

	switch e := m.live[it.id()]; {
	case e.action != "":
		return m, nil
	case e.state == statusDown && it.target.Up != "":
		m.connectAfterStart = it.id()
		return m.runHook(statusStarting)
	}
	return m, m.session()
}

func (m model) session() tea.Cmd {
	mode := m.mode()
	if len(mode.Argv) == 0 {
		return nil
	}
	name := m.selected().name()
	return m.exec(exec.Command(mode.Argv[0], mode.Argv[1:]...), func(err error) tea.Msg {
		return sessionEnded(name, err)
	})
}

func sessionEnded(name string, err error) execDoneMsg {
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return execDoneMsg{}
	case errors.As(err, &exitErr):
		return execDoneMsg{notice: fmt.Sprintf("%s: session ended with exit %d", name, exitErr.ExitCode())}
	default:
		return execDoneMsg{notice: fmt.Sprintf("%s: %v", name, err)}
	}
}

func configEdited(err error) tea.Msg {
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return configEditedMsg{notice: fmt.Sprintf("config edit: %v", err)}
	}
	return configEditedMsg{}
}

func (m model) reload(editStatus string) (tea.Model, tea.Cmd) {
	targets, problems := m.load()
	for _, t := range targets {
		if previous, ok := m.targetByID(t.ID); ok && !slices.Equal(previous.Stats, t.Stats) {
			e := m.live[t.ID]
			e.readings, e.statsOK = nil, false
			m.live[t.ID] = e
		}
	}
	m.targets, m.modeIdx = targets, 0
	m.rebuild()
	m.markStale(m.selected().id())
	m.notice = cmp.Or(editStatus, configNotice(problems))
	return m, tea.Batch(m.refreshSelected(), m.checkWatched())
}

func (m model) markStale(id target.ID) {
	if e, ok := m.live[id]; ok {
		e.statsAt = time.Time{}
		m.live[id] = e
	}
}
