package tui

import (
	"os"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
)

const (
	pollInterval = 2 * time.Second
	liveInterval = 5 * time.Second
	maxLogPeek   = 300
)

type caps struct {
	lifecycle bool
	autostart bool
	tail      bool
	probe     bool
}

type item struct {
	t         target.Target
	provider  target.Provider
	autostart bool
	launcher  bool
	key       rune
	caps      caps
}

func (i item) name() string { return i.t.Name }

func (i item) label() string {
	if i.launcher {
		return i.t.DefaultMode().Name
	}
	return i.t.Name
}

func (i item) running() bool { return i.t.Running() }

func (i item) worthProbing() bool {
	return i.caps.probe && i.t.Status != target.StatusStopped
}

type screen int

const (
	screenList screen = iota
	screenConfirm
	screenLogs
)

type model struct {
	self        string
	reg         target.Registry
	panes       []pane
	focus       int
	modeIdx     int
	width       int
	height      int
	status      string
	spinner     spinner.Model
	screen      screen
	input       textinput.Model
	logName     string
	logView     string
	logTailer   target.Tailer
	logPeek     peek
	hostName    string
	hostStats   probe.Stats
	hostHistory []float64
	live        map[string]liveEntry
	tasks       map[string]string
}

const (
	verbStop    = "stopping"
	verbRestart = "restarting"
	verbAuto    = "autostart"
	verbDelete  = "deleting"
)

func (m model) hasTask(name string) bool { return m.tasks[name] != "" }

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

func Run(reg target.Registry, configWarning error) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 32

	host, _ := os.Hostname()

	m := model{
		self:     exe,
		reg:      reg,
		spinner:  sp,
		input:    in,
		hostName: host,
		live:     map[string]liveEntry{},
		tasks:    map[string]string{},
	}
	if configWarning != nil {
		m.status = configWarning.Error()
	}
	m.rebuild(nil)

	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(), tickCmd(), liveTickCmd())
}

func (m *model) rebuild(targets []target.Target) {
	items := make([]item, 0, len(targets))
	for _, t := range targets {
		provider := m.reg.Provider(t.Provider)
		it := item{t: t, provider: provider}

		_, it.caps.lifecycle = provider.(target.Lifecycle)
		_, it.caps.tail = provider.(target.Tailer)
		_, it.caps.probe = provider.(target.Prober)
		if auto, ok := provider.(target.Autostarter); ok {
			it.caps.autostart = true
			it.autostart = auto.Autostart(t.Name)
		}
		items = append(items, launcherRows(it)...)
	}

	previouslyFocused := m.focusedSection()
	m.panes = carryOverCursors(groupPanes(items), m.panes)
	m.focus = paneIndex(m.panes, previouslyFocused)
}

func launcherRows(it item) []item {
	if it.t.Provider != target.ProviderLocal || len(it.t.Modes) == 0 {
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

func paneIndex(panes []pane, section string) int {
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

func (m *model) startTask(name, verb string, task tea.Cmd) tea.Cmd {
	m.tasks[name], m.status = verb, ""
	return tea.Batch(task, m.spinner.Tick)
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
	stats probe.Stats
	err   bool
}

type loadedMsg struct {
	targets   []target.Target
	hostStats probe.Stats
	err       error
}

type actionMsg struct {
	name   string
	status string
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

func (m model) loadCmd() tea.Cmd {
	reg := m.reg
	return func() tea.Msg {
		targets, err := reg.All()
		msg := loadedMsg{targets: targets, err: err}
		if prober, ok := reg.Provider(target.ProviderLocal).(target.Prober); ok {
			if stats, err := prober.Probe(""); err == nil {
				msg.hostStats = stats
			}
		}
		return msg
	}
}

func fetchLiveCmd(prober target.Prober, name string) tea.Cmd {
	return func() tea.Msg {
		stats, err := prober.Probe(name)
		return liveStatsMsg{name: name, stats: stats, err: err != nil}
	}
}

func (m model) liveFetch() tea.Cmd {
	it := m.selected()
	if !it.worthProbing() {
		return nil
	}
	if e, ok := m.live[it.name()]; ok && time.Since(e.at) < liveInterval {
		return nil
	}
	prober, ok := it.provider.(target.Prober)
	if !ok {
		return nil
	}
	return fetchLiveCmd(prober, it.name())
}

func (m model) onSelectionChange() tea.Cmd {
	return tea.Batch(m.liveFetch(), m.peekLogs())
}

func (m model) peekLogs() tea.Cmd {
	it := m.selected()
	tailer, ok := it.provider.(target.Tailer)
	if !ok {
		return nil
	}
	if m.logPeek.name == it.name() && time.Since(m.logPeek.at) < liveInterval {
		return nil
	}
	return peekLogsCmd(tailer, it.name())
}

func peekLogsCmd(tailer target.Tailer, name string) tea.Cmd {
	return func() tea.Msg {
		return logPeekMsg{name: name, text: tailer.Tail(name, maxLogPeek)}
	}
}

func viewLogsCmd(tailer target.Tailer, name string) tea.Cmd {
	return func() tea.Msg {
		return logViewMsg{name: name, text: tailer.Tail(name, logScreenLines)}
	}
}
