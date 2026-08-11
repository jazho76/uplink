package tui

import (
	"os"
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
	caps      caps
}

func (i item) name() string  { return i.t.Name }
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
	self      string
	reg       target.Registry
	items     []item
	cursor    int
	modeIdx   int
	width     int
	height    int
	status    string
	spinner   spinner.Model
	screen    screen
	input     textinput.Model
	logName   string
	logView   string
	logTailer target.Tailer
	hostName  string
	hostStats probe.Stats
	live      map[string]liveEntry
	logPeek   string
	tasks     map[string]string
}

const (
	verbStop    = "stopping"
	verbRestart = "restarting"
	verbAuto    = "autostart"
	verbDelete  = "deleting"
)

func (m model) hasTask(name string) bool { return m.tasks[name] != "" }

type liveEntry struct {
	stats probe.Stats
	at    time.Time
	err   bool
}

func Run(reg target.Registry, configWarning error) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

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
	return tea.Batch(m.loadCmd(), tickCmd(), liveTickCmd(), m.spinner.Tick)
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
		items = append(items, it)
	}
	m.items = items
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m model) selected() item {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return item{}
	}
	return m.items[m.cursor]
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
	m.modeIdx = ((m.modeIdx+delta)%n + n) % n
}

func (m *model) moveCursor(to int) tea.Cmd {
	if to < 0 || to >= len(m.items) || to == m.cursor {
		return nil
	}
	m.cursor = to
	m.modeIdx = 0
	return m.onSelectionChange()
}

type tickMsg struct{}
type liveTickMsg struct{}
type logTickMsg struct{}

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

func (m *model) onSelectionChange() tea.Cmd {
	it := m.selected()
	if tailer, ok := it.provider.(target.Tailer); ok {
		m.logPeek = tailer.Tail(it.name(), maxLogPeek)
	} else {
		m.logPeek = ""
	}
	return m.liveFetch()
}
