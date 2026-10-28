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

var kindTitles = map[target.Kind]string{
	target.KindLocal:  "local",
	target.KindVM:     "vms",
	target.KindRemote: "remotes",
}

type status string

const (
	statusUnknown status = "unknown"
	statusUp      status = "up"
	statusDown    status = "down"
)

type item struct {
	t        target.Target
	launcher bool
	key      rune
}

func (i item) name() string { return i.t.Name }

func (i item) label() string {
	if i.launcher {
		return i.t.DefaultMode().Name
	}
	return i.t.Name
}

type screen int

const (
	screenList screen = iota
	screenLogs
)

type model struct {
	self      string
	targets   []target.Target
	backend   target.Backend
	panes     []pane
	focus     int
	modeIdx   int
	width     int
	height    int
	status    string
	screen    screen
	logTarget target.Target
	logRound  int
	logView   string
	logPeek   peek
	hostName  string
	live      map[string]liveEntry
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
	pending bool
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
		targets: targets,
		backend: backend,
		live:    map[string]liveEntry{},
	}
	m.rebuild()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.hostCmd(), m.onSelectionChange(), tickCmd(), liveTickCmd())
}

func (m *model) rebuild() {
	var panes []pane
	for _, kind := range target.Kinds {
		p := pane{section: kindTitles[kind]}
		for _, t := range m.targets {
			if t.Kind == kind {
				p.items = append(p.items, launcherRows(item{t: t})...)
			}
		}
		if len(p.items) > 0 {
			panes = append(panes, p)
		}
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)

	previouslyFocused := m.focusedSection()
	m.panes = carryOverCursors(panes, m.panes)
	m.focus = paneToFocus(m.panes, previouslyFocused)
}

func (m model) statusOf(t target.Target) status {
	if t.Local() {
		return statusUp
	}
	e, ok := m.live[t.Name]
	switch {
	case !ok || e.at.IsZero():
		return statusUnknown
	case e.err:
		return statusDown
	default:
		return statusUp
	}
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
	return max(slices.IndexFunc(panes, func(p pane) bool { return p.section == section }), 0)
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
	m.focus, m.modeIdx = inPane, 0
	return m.onSelectionChange()
}

func wrap(i, n int) int {
	return ((i % n) + n) % n
}

type tickMsg struct{}
type liveTickMsg struct{}
type logTickMsg struct{ round int }

type logPeekMsg struct {
	name string
	text string
}

type logViewMsg struct {
	round int
	text  string
}

type liveStatsMsg struct {
	name  string
	stats probe.Stats
	err   bool
}

type reachMsg struct {
	name    string
	reached bool
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

func logTickCmd(round int) tea.Cmd {
	return tea.Tick(logInterval, func(time.Time) tea.Msg { return logTickMsg{round: round} })
}

func (m model) localTarget() (target.Target, bool) {
	i := slices.IndexFunc(m.targets, target.Target.Local)
	if i < 0 {
		return target.Target{}, false
	}
	return m.targets[i], true
}

func (m model) hostCmd() tea.Cmd {
	host, ok := m.localTarget()
	if !ok {
		return nil
	}
	return fetchLiveCmd(m.backend, host)
}

func (m model) hostEntry() liveEntry {
	host, _ := m.localTarget()
	return m.live[host.Name]
}

func fetchLiveCmd(backend target.Backend, t target.Target) tea.Cmd {
	return func() tea.Msg {
		stats, err := backend.Stats(t)
		return liveStatsMsg{name: t.Name, stats: stats, err: err != nil}
	}
}

func reachCmd(backend target.Backend, t target.Target) tea.Cmd {
	return func() tea.Msg {
		return liveStatsMsg{name: t.Name, err: backend.Reachable(t) != nil}
	}
}

func (m model) liveFetch() tea.Cmd {
	it := m.selected()
	if it.name() == "" || it.t.Local() {
		return nil
	}
	e := m.live[it.name()]
	if e.pending || time.Since(e.at) < liveInterval {
		return nil
	}
	e.pending = true
	m.live[it.name()] = e
	if it.t.Stats {
		return fetchLiveCmd(m.backend, it.t)
	}
	return reachCmd(m.backend, it.t)
}

func (m model) onSelectionChange() tea.Cmd {
	return tea.Batch(m.liveFetch(), m.peekLogs())
}

func (m model) peekLogs() tea.Cmd {
	it := m.selected()
	if it.t.Logs == "" {
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

func (m model) viewLogsCmd() tea.Cmd {
	backend, t, round := m.backend, m.logTarget, m.logRound
	return func() tea.Msg {
		return logViewMsg{round: round, text: backend.Logs(t, logScreenLines)}
	}
}
