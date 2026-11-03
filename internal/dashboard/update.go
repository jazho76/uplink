package dashboard

import (
	"errors"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/launcher"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case liveTickMsg:
		cmd := m.send(launcher.Ticked{})
		return m, tea.Batch(cmd, liveTickCmd())

	case launcher.ConfigRead:
		return m.inventoryChanged(msg)

	case launcher.DiscoveryDone:
		return m.inventoryChanged(msg)

	case launcher.Event:
		cmd := m.send(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.state.LogsOpen() {
			return m.updateLogs(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) inventoryChanged(event launcher.Event) (tea.Model, tea.Cmd) {
	cmd := m.send(event)
	m.rebuild(m.state.Targets())
	if id := m.selected().id(); id != m.state.Selection() {
		cmd = tea.Batch(cmd, m.send(launcher.Selected{ID: id}))
	}
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "esc":
		if !m.state.Connection().Cancellable() {
			return m, tea.Quit
		}
		cmd = m.send(launcher.ConnectionCancelled{})

	case "up", "k":
		cmd = m.moveCursor(-1)
	case "down", "j":
		cmd = m.moveCursor(1)

	case "tab":
		cmd = m.send(launcher.ModeCycled{Delta: 1})
	case "shift+tab":
		cmd = m.send(launcher.ModeCycled{Delta: -1})

	case "enter":
		m.pinned = m.selected().id()
		cmd = m.send(launcher.ConnectRequested{})

	case "ctrl+s":
		cmd = m.send(launcher.PowerRequested{Operation: launcher.OperationStarting})
	case "ctrl+x":
		cmd = m.send(launcher.PowerRequested{Operation: launcher.OperationStopping})

	case "ctrl+e":
		if m.state.Connection().Phase != launcher.PhaseNone {
			break
		}
		editor, err := m.edit()
		if err != nil {
			cmd = m.send(launcher.NoticeSet{Text: fmt.Sprintf("config edit: %v", err)})
			break
		}
		cmd = m.exec(editor, configEdited)

	case "ctrl+l":
		cmd = m.send(launcher.LogsOpened{})

	default:
		if i, ok := paneForKey(m.panes, msg.String()); ok && i != m.focus {
			cmd = m.focusItem(i, m.panes[i].cursor)
			break
		}
		focused, _ := m.focusedPane()
		if i, ok := itemForKey(focused, msg.String()); ok {
			cmd = m.focusItem(m.focus, i)
		}
	}
	return m, cmd
}

func (m model) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		cmd := m.send(launcher.LogsClosed{})
		return m, cmd
	}
	return m, nil
}

func configEdited(err error) tea.Msg {
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return launcher.ReloadRequested{Notice: fmt.Sprintf("config edit: %v", err)}
	}
	return launcher.ReloadRequested{}
}
