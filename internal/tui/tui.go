package tui

import (
	"fmt"
	"os"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
)

const (
	liveInterval   = 5 * time.Second
	watchInterval  = 30 * time.Second
	maxWatchChecks = 4
	maxLogPeek     = 300
)

type status string

const (
	statusUnknown  status = "unknown"
	statusUp       status = "up"
	statusDown     status = "down"
	statusStarting status = "starting"
	statusStopping status = "stopping"
)

type item struct {
	t   target.Target
	key rune
}

func (i item) name() string { return i.t.Name }

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
	live      map[string]liveEntry
	load      func() ([]target.Target, error)
}

type peek struct {
	name string
	text string
	at   time.Time
}

type liveEntry struct {
	state    status
	action   status
	readings []probe.Reading
	statsOK  bool
	at       time.Time
	checked  time.Time
	pending  bool
	epoch    int
}

func Run(load func() ([]target.Target, error), backend target.Backend) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	targets, problems := load()
	m := newModel(targets, backend)
	m.self, m.load = exe, load
	m.status = configStatus(problems)

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
	return tea.Batch(m.onSelectionChange(), m.watchFetch(), liveTickCmd())
}

func (m *model) rebuild() {
	var panes []pane
	for _, t := range m.targets {
		i := slices.IndexFunc(panes, func(p pane) bool { return p.section == t.Group })
		if i < 0 {
			panes = append(panes, pane{section: t.Group})
			i = len(panes) - 1
		}
		panes[i].items = append(panes[i].items, item{t: t})
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)

	previouslyFocused := m.focusedSection()
	m.panes = carryOverCursors(panes, m.panes)
	m.focus = paneToFocus(m.panes, previouslyFocused)
}

func (m model) statusOf(t target.Target) status {
	e := m.live[t.Name]
	switch {
	case e.action != "":
		return e.action
	case e.state != "":
		return e.state
	default:
		return statusUnknown
	}
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

type liveMsg struct {
	name     string
	epoch    int
	up       bool
	full     bool
	readings []probe.Reading
	statsOK  bool
}

type hookDoneMsg struct {
	name string
	verb string
	err  error
}

type configEditedMsg struct{ status string }

type execDoneMsg struct {
	status string
}

func liveTickCmd() tea.Cmd {
	return tea.Tick(liveInterval, func(time.Time) tea.Msg { return liveTickMsg{} })
}

func logTickCmd(round int) tea.Cmd {
	return tea.Tick(logInterval, func(time.Time) tea.Msg { return logTickMsg{round: round} })
}

func checkCmd(backend target.Backend, t target.Target, full bool, epoch int) tea.Cmd {
	return func() tea.Msg {
		msg := liveMsg{name: t.Name, epoch: epoch, full: full}
		msg.up = backend.Status(t) == nil
		if msg.up && full && len(t.Stats) > 0 {
			readings, err := backend.Stats(t)
			msg.readings, msg.statsOK = readings, err == nil
		}
		return msg
	}
}

func (m model) check(t target.Target, full bool) tea.Cmd {
	e := m.live[t.Name]
	e.pending = true
	m.live[t.Name] = e
	return checkCmd(m.backend, t, full, e.epoch)
}

func (m model) liveFetch() tea.Cmd {
	if time.Since(m.live[m.selected().name()].at) < liveInterval {
		return nil
	}
	return m.probeSelected()
}

func (m model) probeSelected() tea.Cmd {
	it := m.selected()
	if it.name() == "" || m.live[it.name()].pending {
		return nil
	}
	return m.check(it.t, true)
}

func (m model) watchFetch() tea.Cmd {
	inFlight := 0
	for _, e := range m.live {
		if e.pending {
			inFlight++
		}
	}

	var cmds []tea.Cmd
	for _, t := range m.targets {
		if inFlight >= maxWatchChecks {
			break
		}
		e := m.live[t.Name]
		if !t.Watch || t.Name == m.selected().name() || e.pending || time.Since(e.checked) < watchInterval {
			continue
		}
		cmds = append(cmds, m.check(t, false))
		inFlight++
	}
	return tea.Batch(cmds...)
}

func (m model) settle(msg liveMsg) liveEntry {
	e := m.live[msg.name]
	if msg.epoch != e.epoch {
		return e
	}
	e.pending, e.checked = false, time.Now()
	e.state = statusDown
	if msg.up {
		e.state = statusUp
	}
	if msg.full {
		e.at = e.checked
		e.statsOK = msg.statsOK
		if msg.statsOK {
			e.readings = msg.readings
		}
	}
	if !msg.up {
		e.statsOK = false
	}
	return e
}

func (m model) canHook(t target.Target, start bool) bool {
	command, settled := t.Down, statusDown
	if start {
		command, settled = t.Up, statusUp
	}
	e := m.live[t.Name]
	return command != "" && e.action == "" && e.state != settled
}

func (m model) runHook(start bool) (tea.Model, tea.Cmd) {
	t := m.selected().t
	if !m.canHook(t, start) {
		return m, nil
	}
	action, verb, hook := statusStopping, "stop", m.backend.Down
	if start {
		action, verb, hook = statusStarting, "start", m.backend.Up
	}
	e := m.live[t.Name]
	e.action = action
	m.live[t.Name] = e
	m.status = ""
	return m, func() tea.Msg { return hookDoneMsg{name: t.Name, verb: verb, err: hook(t)} }
}

func (m model) targetNamed(name string) (target.Target, bool) {
	i := slices.IndexFunc(m.targets, func(t target.Target) bool { return t.Name == name })
	if i < 0 {
		return target.Target{}, false
	}
	return m.targets[i], true
}

func (m model) onSelectionChange() tea.Cmd {
	return tea.Batch(m.liveFetch(), m.peekLogs())
}

func (m model) peekLogs() tea.Cmd {
	if m.logPeek.name == m.selected().name() && time.Since(m.logPeek.at) < liveInterval {
		return nil
	}
	return m.peekSelected()
}

func (m model) peekSelected() tea.Cmd {
	it := m.selected()
	if it.t.Logs == "" || m.screen == screenLogs {
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

func configStatus(problems error) string {
	switch count := countProblems(problems); count {
	case 0:
		return ""
	case 1:
		return "config: " + problems.Error()
	default:
		return fmt.Sprintf("config: %d problems, see uplink config check", count)
	}
}

func countProblems(err error) int {
	if err == nil {
		return 0
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return 1
	}
	count := 0
	for _, inner := range joined.Unwrap() {
		count += countProblems(inner)
	}
	return count
}
