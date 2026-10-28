package tui

import (
	"os"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
)

const (
	pollInterval = 2 * time.Second
	liveInterval = 5 * time.Second
	maxLogPeek   = 300
)

var kindTitles = []struct {
	kind  target.Kind
	title string
}{
	{target.KindLocal, "local"},
	{target.KindVM, "vms"},
	{target.KindRemote, "remotes"},
}

type caps struct {
	tail  bool
	probe bool
}

type item struct {
	t        target.Target
	launcher bool
	key      rune
	caps     caps
}

func (i item) name() string { return i.t.Name }

func (i item) label() string {
	if i.launcher {
		return i.t.DefaultMode().Name
	}
	return i.t.Name
}

func (i item) up() bool { return i.t.Up() }

func (i item) worthProbing() bool { return i.caps.probe }

type screen int

const (
	screenList screen = iota
	screenLogs
)

type model struct {
	self        string
	targets     []target.Target
	backend     target.Backend
	statuses    map[string]target.Status
	panes       []pane
	focus       int
	modeIdx     int
	width       int
	height      int
	status      string
	screen      screen
	logTarget   target.Target
	logView     string
	logPeek     peek
	hostName    string
	hostStats   probe.Stats
	hostHistory []float64
	live        map[string]liveEntry
}

type peek struct {
	name string
	text string
	at   time.Time
}

type liveEntry struct {
	stats   probe.Stats
	history []float64
	at      time.Time
	err     bool
}

func Run(targets []target.Target, backend target.Backend, configWarning error) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	host, _ := os.Hostname()

	m := newModel(targets, backend)
	m.self, m.hostName = exe, host
	if configWarning != nil {
		m.status = configWarning.Error()
	}

	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(targets []target.Target, backend target.Backend) model {
	m := model{
		targets:  targets,
		backend:  backend,
		statuses: map[string]target.Status{},
		live:     map[string]liveEntry{},
	}
	m.rebuild()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(), m.onSelectionChange(), tickCmd(), liveTickCmd())
}

func (m *model) rebuild() {
	groups := make([]group, 0, len(kindTitles))
	for _, k := range kindTitles {
		g := group{section: k.title}
		for _, t := range m.targets {
			if t.Kind != k.kind {
				continue
			}
			if status, ok := m.statuses[t.Name]; ok {
				t.Status = status
			}
			g.items = append(g.items, rowsFor(t)...)
		}
		groups = append(groups, g)
	}

	previouslyFocused := m.focusedSection()
	m.panes = carryOverCursors(groupPanes(groups), m.panes)
	m.focus = paneToFocus(m.panes, previouslyFocused)
}

func rowsFor(t target.Target) []item {
	it := item{t: t, caps: caps{tail: t.Logs != "", probe: t.Stats}}
	return launcherRows(it)
}

func launcherRows(it item) []item {
	if !it.t.Local() || len(it.t.Modes) == 0 {
		return []item{it}
	}

	rows := make([]item, 0, len(it.t.Modes))
	for _, mode := range it.t.Modes {
		row := it
		row.t.Modes = []target.Mode{mode}
		row.launcher = true
		rows = append(rows, row)
	}
	return rows
}

func carryOverCursors(fresh, previous []pane) []pane {
	cursors := map[string]int{}
	for _, p := range previous {
		cursors[p.section] = p.cursor
	}
	for i := range fresh {
		fresh[i].cursor = max(min(cursors[fresh[i].section], len(fresh[i].items)-1), 0)
	}
	return fresh
}

func paneToFocus(panes []pane, section string) int {
	if i := slices.IndexFunc(panes, func(p pane) bool {
		return p.section == section && len(p.items) > 0
	}); i >= 0 {
		return i
	}
	return max(slices.IndexFunc(panes, func(p pane) bool { return len(p.items) > 0 }), 0)
}

func (m model) focusedPane() (pane, bool) {
	if m.focus < 0 || m.focus >= len(m.panes) {
		return pane{}, false
	}
	return m.panes[m.focus], true
}

func (m model) focusedSection() string {
	p, _ := m.focusedPane()
	return p.section
}

func (m model) selected() item {
	p, ok := m.focusedPane()
	if !ok || p.cursor < 0 || p.cursor >= len(p.items) {
		return item{}
	}
	return p.items[p.cursor]
}

func (m model) mode() target.Mode {
	modes := m.selected().t.Modes
	if m.modeIdx < 0 || m.modeIdx >= len(modes) {
		return m.selected().t.DefaultMode()
	}
	return modes[m.modeIdx]
}

func (m *model) cycleMode(delta int) {
	n := len(m.selected().t.Modes)
	if n < 2 {
		return
	}
	m.modeIdx = wrap(m.modeIdx+delta, n)
}

func (m *model) moveCursor(delta int) tea.Cmd {
	p, ok := m.focusedPane()
	if !ok {
		return nil
	}
	if to := p.cursor + delta; to >= 0 && to < len(p.items) {
		return m.focusItem(m.focus, to)
	}

	next, ok := nextPaneWithItems(m.panes, m.focus, delta)
	if !ok {
		return nil
	}
	if delta < 0 {
		return m.focusItem(next, len(m.panes[next].items)-1)
	}
	return m.focusItem(next, 0)
}

func nextPaneWithItems(panes []pane, from, step int) (int, bool) {
	for moved := 1; moved <= len(panes); moved++ {
		next := wrap(from+moved*step, len(panes))
		if len(panes[next].items) > 0 {
			return next, true
		}
	}
	return 0, false
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
	m.focus, m.modeIdx = inPane, 0
	return m.onSelectionChange()
}

func (m *model) recordStatus(name string, reached bool) {
	status := target.StatusDown
	if reached {
		status = target.StatusUp
	}
	if m.statuses[name] == status {
		return
	}
	m.statuses[name] = status
	m.rebuild()
}

func wrap(i, n int) int {
	return ((i % n) + n) % n
}

type tickMsg struct{}
type liveTickMsg struct{}
type logTickMsg struct{}

type logPeekMsg struct {
	name string
	text string
}

type logViewMsg struct {
	name string
	text string
}

type liveStatsMsg struct {
	name  string
	local bool
	stats probe.Stats
	err   bool
}

type reachMsg struct {
	name    string
	reached bool
}

type loadedMsg struct {
	hostStats probe.Stats
}

type execDoneMsg struct {
	status string
	quit   bool
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func liveTickCmd() tea.Cmd {
	return tea.Tick(liveInterval, func(time.Time) tea.Msg { return liveTickMsg{} })
}

func logTickCmd() tea.Cmd {
	return tea.Tick(logInterval, func(time.Time) tea.Msg { return logTickMsg{} })
}

func (m model) localTarget() (target.Target, bool) {
	i := slices.IndexFunc(m.targets, target.Target.Local)
	if i < 0 {
		return target.Target{}, false
	}
	return m.targets[i], true
}

func (m model) loadCmd() tea.Cmd {
	host, ok := m.localTarget()
	if !ok {
		return nil
	}
	backend := m.backend
	return func() tea.Msg {
		stats, _ := backend.Stats(host)
		return loadedMsg{hostStats: stats}
	}
}

func fetchLiveCmd(backend target.Backend, t target.Target) tea.Cmd {
	return func() tea.Msg {
		stats, err := backend.Stats(t)
		return liveStatsMsg{name: t.Name, local: t.Local(), stats: stats, err: err != nil}
	}
}

func reachCmd(backend target.Backend, t target.Target) tea.Cmd {
	return func() tea.Msg {
		return reachMsg{name: t.Name, reached: backend.Reachable(t) == nil}
	}
}

func (m model) liveFetch() tea.Cmd {
	it := m.selected()
	if it.name() == "" {
		return nil
	}
	if e, ok := m.live[it.name()]; ok && time.Since(e.at) < liveInterval {
		return nil
	}
	if it.worthProbing() {
		return fetchLiveCmd(m.backend, it.t)
	}
	if !it.t.Local() {
		return reachCmd(m.backend, it.t)
	}
	return nil
}

func (m model) onSelectionChange() tea.Cmd {
	return tea.Batch(m.liveFetch(), m.peekLogs())
}

func (m model) peekLogs() tea.Cmd {
	it := m.selected()
	if !it.caps.tail {
		return nil
	}
	if m.logPeek.name == it.name() && time.Since(m.logPeek.at) < liveInterval {
		return nil
	}
	return peekLogsCmd(m.backend, it.t)
}

func peekLogsCmd(backend target.Backend, t target.Target) tea.Cmd {
	return func() tea.Msg {
		return logPeekMsg{name: t.Name, text: backend.Logs(t, maxLogPeek)}
	}
}

func viewLogsCmd(backend target.Backend, t target.Target) tea.Cmd {
	return func() tea.Msg {
		return logViewMsg{name: t.Name, text: backend.Logs(t, logScreenLines)}
	}
}
