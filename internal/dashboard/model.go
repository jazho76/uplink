package dashboard

import (
	"os/exec"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

type item struct {
	target target.Target
	key    rune
}

func (i item) id() target.ID { return i.target.ID }

func (i item) name() string { return i.target.Name }

func (i item) empty() bool { return i.id() == target.ID{} }

type model struct {
	state   launcher.State
	runner  *launcher.Runner
	startup []launcher.Effect
	panes   []pane
	focus   int
	width   int
	height  int
	edit    func() (*exec.Cmd, error)
	exec    func(*exec.Cmd, tea.ExecCallback) tea.Cmd
}

func Run(load func() ([]target.Target, error), edit func() (*exec.Cmd, error), backend launcher.Backend) error {
	targets, problems := load()
	m := newModel(targets, backend)
	m.runner.Load, m.edit = load, edit
	defer m.runner.Stop()
	m.state, m.startup = launcher.New(targets, problems).Select(m.selected().id()).Apply(time.Now(), launcher.Started{})

	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(targets []target.Target, backend launcher.Backend) model {
	m := model{
		state:  launcher.New(targets, nil),
		runner: launcher.NewRunner(backend, nil),
		exec:   tea.ExecProcess,
	}
	m.rebuild(targets)
	m.state = m.state.Select(m.selected().id())
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.perform(m.startup), liveTickCmd())
}

func (m *model) rebuild(targets []target.Target) {
	var panes []pane
	for _, t := range targets {
		i := slices.IndexFunc(panes, func(p pane) bool { return p.group == t.Group })
		if i < 0 {
			panes = append(panes, pane{group: t.Group, compact: true})
			i = len(panes) - 1
		}
		panes[i].items = append(panes[i].items, item{target: t})
		panes[i].compact = panes[i].compact && !t.Watch
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)

	previouslyFocused := m.focusedGroup()
	m.panes = carryOverCursors(panes, m.panes)
	m.focus = paneToFocus(m.panes, previouslyFocused)
}

func (m model) statusOf(t target.Target) string {
	return m.state.Observation(t.ID).Label()
}

func carryOverCursors(fresh, previous []pane) []pane {
	cursors := map[string]int{}
	selected := map[string]target.ID{}
	for _, p := range previous {
		cursors[p.group] = p.cursor
		if p.cursor >= 0 && p.cursor < len(p.items) {
			selected[p.group] = p.items[p.cursor].id()
		}
	}
	for i := range fresh {
		p := &fresh[i]
		p.cursor = slices.IndexFunc(p.items, func(it item) bool { return it.id() == selected[p.group] })
		if p.cursor < 0 {
			p.cursor = max(min(cursors[p.group], len(p.items)-1), 0)
		}
	}
	return fresh
}

func paneToFocus(panes []pane, group string) int {
	return max(slices.IndexFunc(panes, func(p pane) bool { return p.group == group }), 0)
}

func (m model) focusedPane() (pane, bool) {
	if m.focus < 0 || m.focus >= len(m.panes) {
		return pane{}, false
	}
	return m.panes[m.focus], true
}

func (m model) focusedGroup() string {
	p, _ := m.focusedPane()
	return p.group
}

func (m model) selected() item {
	p, ok := m.focusedPane()
	if !ok || p.cursor < 0 || p.cursor >= len(p.items) {
		return item{}
	}
	return p.items[p.cursor]
}

func (m *model) moveCursor(delta int) tea.Cmd {
	p, ok := m.focusedPane()
	if !ok {
		return nil
	}
	if to := p.cursor + delta; to >= 0 && to < len(p.items) {
		return m.focusItem(m.focus, to)
	}

	next := wrap(m.focus+delta, len(m.panes))
	if delta < 0 {
		return m.focusItem(next, len(m.panes[next].items)-1)
	}
	return m.focusItem(next, 0)
}

func (m *model) focusItem(inPane, cursor int) tea.Cmd {
	if inPane < 0 || inPane >= len(m.panes) {
		return nil
	}
	landed := m.panes[inPane]
	landed.cursor = min(max(cursor, 0), max(len(landed.items)-1, 0))
	if inPane == m.focus && landed.cursor == m.panes[inPane].cursor {
		return nil
	}

	m.panes = append([]pane(nil), m.panes...)
	m.panes[inPane] = landed
	m.focus = inPane
	return m.send(launcher.Selected{ID: m.selected().id()})
}

func wrap(i, n int) int {
	return ((i % n) + n) % n
}

func (m *model) send(event launcher.Event) tea.Cmd {
	state, effects := m.state.Apply(time.Now(), event)
	m.state = state
	return m.perform(effects)
}

func (m model) perform(effects []launcher.Effect) tea.Cmd {
	var cmds []tea.Cmd
	for _, effect := range effects {
		if cmd := m.command(effect); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

func (m model) command(effect launcher.Effect) tea.Cmd {
	switch e := effect.(type) {
	case launcher.Connect:
		return m.exec(exec.Command(e.Argv[0], e.Argv[1:]...), func(err error) tea.Msg {
			return launcher.ConnectionEnded{Name: e.Name, Err: err}
		})
	case launcher.ScheduleLogTick:
		return tea.Tick(launcher.LogInterval, func(time.Time) tea.Msg { return launcher.LogTicked{Round: e.Round} })
	}
	runner := m.runner
	return func() tea.Msg { return runner.Run(effect) }
}

type liveTickMsg struct{}

func liveTickCmd() tea.Cmd {
	return tea.Tick(launcher.LiveInterval, func(time.Time) tea.Msg { return liveTickMsg{} })
}
