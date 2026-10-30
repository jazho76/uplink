package tui

import (
	"fmt"
	"os/exec"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/stats"
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
	target target.Target
	key    rune
}

func (i item) name() string { return i.target.Name }

type screen int

const (
	screenList screen = iota
	screenLogs
)

type model struct {
	targets   []target.Target
	backend   target.Backend
	panes     []pane
	focus     int
	modeIdx   int
	width     int
	height    int
	notice    string
	screen    screen
	logTarget target.Target
	logRound  int
	logView   string
	logPeek   peek
	live      map[string]liveEntry
	load      func() ([]target.Target, error)
	edit      func() (*exec.Cmd, error)
	exec      func(*exec.Cmd, tea.ExecCallback) tea.Cmd

	connectAfterStart string
}

type peek struct {
	name string
	text string
	at   time.Time
}

type liveEntry struct {
	state     status
	action    status
	readings  []stats.Reading
	statsOK   bool
	statsAt   time.Time
	checkedAt time.Time
	pending   bool
	epoch     int
}

func Run(load func() ([]target.Target, error), edit func() (*exec.Cmd, error), backend target.Backend) error {
	targets, problems := load()
	m := newModel(targets, backend)
	m.load, m.edit = load, edit
	m.notice = configNotice(problems)

	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(targets []target.Target, backend target.Backend) model {
	m := model{
		targets: targets,
		backend: backend,
		live:    map[string]liveEntry{},
		exec:    tea.ExecProcess,
	}
	m.rebuild()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.refreshSelected(), m.checkWatched(), liveTickCmd())
}

func (m *model) rebuild() {
	var panes []pane
	for _, t := range m.targets {
		i := slices.IndexFunc(panes, func(p pane) bool { return p.group == t.Group })
		if i < 0 {
			panes = append(panes, pane{group: t.Group})
			i = len(panes) - 1
		}
		panes[i].items = append(panes[i].items, item{target: t})
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)

	previouslyFocused := m.focusedGroup()
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
		cursors[p.group] = p.cursor
	}
	for i := range fresh {
		fresh[i].cursor = max(min(cursors[fresh[i].group], len(fresh[i].items)-1), 0)
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

func (m model) mode() target.Mode {
	modes := m.selected().target.Modes
	if m.modeIdx < 0 || m.modeIdx >= len(modes) {
		return m.selected().target.DefaultMode()
	}
	return modes[m.modeIdx]
}

func (m *model) cycleMode(delta int) {
	n := len(m.selected().target.Modes)
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
	return m.refreshSelected()
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
	readings []stats.Reading
	statsOK  bool
}

type hookDoneMsg struct {
	name   string
	action status
	err    error
}

type configEditedMsg struct{ notice string }

type execDoneMsg struct {
	notice string
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

func (m model) checkSelectedIfStale() tea.Cmd {
	if time.Since(m.live[m.selected().name()].statsAt) < liveInterval {
		return nil
	}
	return m.checkSelected()
}

func (m model) checkSelected() tea.Cmd {
	it := m.selected()
	if it.name() == "" || m.live[it.name()].pending {
		return nil
	}
	return m.check(it.target, true)
}

func (m model) checkWatched() tea.Cmd {
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
		if !t.Watch || t.Name == m.selected().name() || e.pending || time.Since(e.checkedAt) < watchInterval {
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
	e.pending, e.checkedAt = false, time.Now()
	e.state = statusDown
	if msg.up {
		e.state = statusUp
	}
	if msg.full {
		e.statsAt = e.checkedAt
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

type hook struct {
	command string
	settled status
	verb    string
	run     func(target.Target) error
}

func (m model) hookFor(t target.Target, action status) hook {
	if action == statusStarting {
		return hook{command: t.Up, settled: statusUp, verb: "start", run: m.backend.Up}
	}
	return hook{command: t.Down, settled: statusDown, verb: "stop", run: m.backend.Down}
}

func (m model) canHook(t target.Target, action status) bool {
	h, e := m.hookFor(t, action), m.live[t.Name]
	return h.command != "" && e.action == "" && e.state != h.settled
}

func (m model) runHook(action status) (tea.Model, tea.Cmd) {
	t := m.selected().target
	if !m.canHook(t, action) {
		return m, nil
	}
	e := m.live[t.Name]
	e.action = action
	m.live[t.Name] = e
	m.notice = ""
	run := m.hookFor(t, action).run
	return m, func() tea.Msg { return hookDoneMsg{name: t.Name, action: action, err: run(t)} }
}

func (m model) targetNamed(name string) (target.Target, bool) {
	i := slices.IndexFunc(m.targets, func(t target.Target) bool { return t.Name == name })
	if i < 0 {
		return target.Target{}, false
	}
	return m.targets[i], true
}

func (m model) refreshSelected() tea.Cmd {
	return tea.Batch(m.checkSelectedIfStale(), m.peekSelectedLogsIfStale())
}

func (m model) peekSelectedLogsIfStale() tea.Cmd {
	if m.logPeek.name == m.selected().name() && time.Since(m.logPeek.at) < liveInterval {
		return nil
	}
	return m.peekSelectedLogs()
}

func (m model) peekSelectedLogs() tea.Cmd {
	it := m.selected()
	if it.target.Logs == "" || m.screen == screenLogs {
		return nil
	}
	return peekLogsCmd(m.backend, it.target)
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

func configNotice(problems error) string {
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
