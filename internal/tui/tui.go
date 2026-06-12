package tui

import (
	"os"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/vmm/internal/host"
	"github.com/jazho76/vmm/internal/lima"
	"github.com/jazho76/vmm/internal/run"
)

const pollInterval = 2 * time.Second

type itemKind int

const (
	kindHost itemKind = iota
	kindVM
)

type item struct {
	kind      itemKind
	name      string
	status    string
	autostart bool
	inst      lima.Instance
}

func (i item) running() bool { return i.status == "Running" }

type mode int

const (
	modeNormal mode = iota
	modeConfirmDelete
	modeLogs
)

type model struct {
	self      string
	items     []item
	cursor    int
	width     int
	height    int
	status    string
	spinner   spinner.Model
	mode      mode
	input     textinput.Model
	host      hostInfo
	logName   string
	logView   string
	hostStats host.Stats
	guest     map[string]guestEntry
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

type guestEntry struct {
	stats lima.GuestStats
	at    time.Time
	err   bool
}

type hostInfo struct {
	name   string
	os     string
	uptime string
}

func gatherHost() hostInfo {
	h := hostInfo{}
	h.name, _ = os.Hostname()
	if v, err := run.Output("uname", "-sr"); err == nil {
		h.os = v
	}
	if v, err := run.Output("uptime", "-p"); err == nil {
		h.uptime = v
	}
	return h
}

func Run() error {
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

	m := model{
		self:    exe,
		spinner: sp,
		input:   in,
		host:    gatherHost(),
		guest:   map[string]guestEntry{},
		tasks:   map[string]string{},
	}
	m.rebuild(nil)

	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadCmd, tickCmd(), guestTickCmd(), m.spinner.Tick)
}

func (m *model) rebuild(instances map[string]lima.Instance) {
	items := []item{{kind: kindHost, name: "host"}}
	names := make([]string, 0, len(instances))
	for name := range instances {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		inst := instances[name]
		items = append(items, item{
			kind:      kindVM,
			name:      name,
			status:    inst.Status,
			inst:      inst,
			autostart: lima.AutostartEnabled(name),
		})
	}
	m.items = items
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
}

func (m model) selected() item {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return item{}
	}
	return m.items[m.cursor]
}

type tickMsg struct{}
type guestTickMsg struct{}
type guestStatsMsg struct {
	name  string
	stats lima.GuestStats
	err   bool
}
type loadedMsg struct {
	instances map[string]lima.Instance
	hostStats host.Stats
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

const guestInterval = 5 * time.Second

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func guestTickCmd() tea.Cmd {
	return tea.Tick(guestInterval, func(time.Time) tea.Msg { return guestTickMsg{} })
}

func loadCmd() tea.Msg {
	list, err := lima.List()
	if err != nil {
		return loadedMsg{err: err}
	}
	byName := make(map[string]lima.Instance, len(list))
	for _, i := range list {
		byName[i.Name] = i
	}
	return loadedMsg{instances: byName, hostStats: host.Read()}
}

func fetchGuestCmd(name string) tea.Cmd {
	return func() tea.Msg {
		s, err := lima.Guest(name)
		return guestStatsMsg{name: name, stats: s, err: err != nil}
	}
}

func (m model) guestFetch() tea.Cmd {
	it := m.selected()
	if it.kind != kindVM || !it.running() {
		return nil
	}
	if e, ok := m.guest[it.name]; ok && time.Since(e.at) < guestInterval {
		return nil
	}
	return fetchGuestCmd(it.name)
}

func (m *model) onSelectionChange() tea.Cmd {
	it := m.selected()
	if m.hasLogs(it) {
		m.logPeek = readLogPeek(it.name, maxLogPeek)
	} else {
		m.logPeek = ""
	}
	return m.guestFetch()
}

func (m model) hasLogs(it item) bool {
	return it.kind == kindVM
}

const maxLogPeek = 300
