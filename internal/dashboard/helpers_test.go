package dashboard

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/system"
	"github.com/jazho76/uplink/internal/target"
)

type fakeBackend struct {
	logs        map[string]string
	down        map[string]bool
	hooks       []string
	statuses    int
	inspections int
	sessions    []string
	tools       map[string][]string
	logCalls    int
}

func (f *fakeBackend) Inspect(_ context.Context, t target.Target) (target.Environment, error) {
	f.inspections++
	if f.down[t.Name] {
		return target.Environment{}, errBoom{}
	}
	readings := make([]target.Reading, len(t.Metrics))
	for i, m := range t.Metrics {
		readings[i] = reading(m, 0.1, 4)
	}
	return target.Environment{Readings: readings, Tools: f.tools[t.Name]}, nil
}

func (f *fakeBackend) Status(_ context.Context, t target.Target) (target.Status, error) {
	f.statuses++
	if f.down[t.Name] {
		return target.StatusDown, errBoom{}
	}
	return target.StatusUp, nil
}

func (f *fakeBackend) Up(_ context.Context, t target.Target) error {
	f.hooks = append(f.hooks, "up "+t.Name)
	if t.Name == "jammed" {
		return errBoom{}
	}
	f.down[t.Name] = false
	return nil
}

func (f *fakeBackend) Down(_ context.Context, t target.Target) error {
	f.hooks = append(f.hooks, "down "+t.Name)
	if t.Name == "stuck" {
		return errBoom{}
	}
	f.down[t.Name] = true
	return nil
}

func (f *fakeBackend) Logs(_ context.Context, t target.Target, _ int) string {
	f.logCalls++
	return f.logs[t.Name]
}

type fakeCatalog struct {
	plan     inventory.Plan
	found    map[string][]inventory.Discovered
	failures map[string]error
}

func (f *fakeCatalog) ReadPlan() inventory.Plan { return f.plan }

func (f *fakeCatalog) Discover(_ context.Context, name string) ([]inventory.Discovered, error) {
	return f.found[name], f.failures[name]
}

func planOf(targets []target.Target, problems error) *fakeCatalog {
	return &fakeCatalog{plan: inventory.Plan{Declared: targets, Problems: problems}}
}

var linuxMetrics = []target.Metric{
	{Type: target.TypeGauge, Label: "load", Value: "x", Max: "x"},
	{Type: target.TypeGauge, Label: "memory", Value: "x", Max: "x", Unit: target.UnitBytes},
	{Type: target.TypeGauge, Label: "disk", Value: "x", Max: "x", Unit: target.UnitBytes},
	{Type: target.TypeText, Label: "up", Value: "x", Unit: target.UnitSeconds},
}

func reading(m target.Metric, value, max float64) target.Reading {
	return target.Reading{Metric: m, Value: value, Max: max, OK: true}
}

func loadReading(load, cores float64) target.Reading {
	return reading(linuxMetrics[0], load, cores)
}

func linuxReadings(load, cores, used, total float64, uptime time.Duration) []target.Reading {
	return []target.Reading{
		loadReading(load, cores),
		reading(linuxMetrics[1], used, total),
		reading(linuxMetrics[2], used, total),
		reading(linuxMetrics[3], uptime.Seconds(), 0),
	}
}

func hostTarget() target.Target {
	return target.Target{
		ID:      configID("host"),
		Name:    "host",
		Group:   "local",
		Logs:    target.LogSource{Command: "journalctl"},
		Metrics: linuxMetrics,
		Modes: []target.Mode{
			{Name: "tmux", Argv: []string{"tmux"}, Multiplexer: "tmux"},
			{Name: "shell", Argv: []string{"/bin/sh"}},
			{Name: "top", Argv: []string{"/bin/sh", "-c", "htop"}},
		},
	}
}

func vm(name string) target.Target {
	via := []string{"ssh", "lima-" + name}
	return target.Target{
		ID:      configID(name),
		Name:    name,
		Group:   "vms",
		Reach:   target.Reach{Interactive: via},
		Logs:    target.LogSource{Command: "tail serial.log"},
		Metrics: linuxMetrics,
		Modes: []target.Mode{
			{Name: "tmux", Argv: append(via, "tmux"), Multiplexer: "tmux"},
			{Name: "shell", Argv: via},
			{Name: "top", Argv: append(via, "htop")},
		},
	}
}

func remote(name string) target.Target {
	t := vm(name)
	t.Group = "remotes"
	return t
}

func withDefaultModes(name string) target.Target {
	t := vm(name)
	t.Modes = []target.Mode{
		{Name: "tmux", Argv: append(slices.Clone(t.Reach.Interactive), "tmux"), Requires: "tmux", Multiplexer: "tmux"},
		{Name: "shell", Argv: t.Reach.Interactive},
	}
	return t
}

const (
	statusUnknown  = string(target.StatusUnknown)
	statusUp       = string(target.StatusUp)
	statusDown     = string(target.StatusDown)
	statusStarting = string(launcher.OperationStarting)
	statusStopping = string(launcher.OperationStopping)
)

type errBoom struct{}

func (errBoom) Error() string { return "boom" }

func named(name string) item {
	return item{target: target.Target{ID: configID(name), Name: name}}
}

func panesOf(items ...item) []pane {
	panes := []pane{{group: "pane", items: items}}
	assignPaneKeys(panes)
	assignItemKeys(panes)
	return panes
}

func newTestModel() (model, *fakeBackend) {
	backend := &fakeBackend{
		logs: map[string]string{"host": "journal line"},
		down: map[string]bool{},
	}
	m := testModel([]target.Target{hostTarget(), vm("forge"), vm("tokyo")}, backend)
	m.edit = func() (*exec.Cmd, error) { return exec.Command("vi", "/tmp/config.yaml"), nil }
	m.exec = func(cmd *exec.Cmd, _ tea.ExecCallback) tea.Cmd {
		backend.sessions = append(backend.sessions, strings.Join(cmd.Args, " "))
		return nil
	}
	return m, backend
}

func testModel(targets []target.Target, backend system.Backend) model {
	return newModel(targets, system.NewRunner(backend, nil))
}

func sized(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	return next.(model)
}

func withTargets(m model, targets ...target.Target) model {
	return replaceTargets(m, append([]target.Target{hostTarget()}, targets...))
}

func replaceTargets(m model, targets []target.Target) model {
	m.state = m.state.WithTargets(targets)
	m.rebuild(targets)
	m.state = m.state.Select(m.selected().id())
	return m
}

func focus(m model, name string) model {
	for i, p := range m.panes {
		for j, it := range p.items {
			if it.name() == name {
				m.focus, m.panes[i].cursor = i, j
				m.pinned = it.id()
				m.state = m.state.Select(it.id())
				return m
			}
		}
	}
	return m
}

func focusedOn(name string) (model, *fakeBackend) {
	m, backend := newTestModel()
	m = sized(withTargets(m, withDefaultModes(name)))
	return focus(m, name), backend
}

func (m model) withSelectionRefreshed() model {
	cmd := m.send(launcher.Selected{ID: m.selected().id()})
	return applyCmd(m, cmd)
}

func observed(m model, name string, env target.Environment) model {
	return reported(m, configID(name), env)
}

func reported(m model, id target.ID, env target.Environment) model {
	up, _ := m.Update(launcher.Checked{ID: id, Status: target.StatusUp, Full: true})
	next, _ := up.(model).Update(launcher.Inspected{ID: id, Env: env})
	return next.(model)
}

func failedInspection(m model, id target.ID) model {
	up, _ := m.Update(launcher.Checked{ID: id, Status: target.StatusUp, Full: true})
	next, _ := up.(model).Update(launcher.Inspected{ID: id, Err: errBoom{}})
	return next.(model)
}

func inspected(m model, name string, tools ...string) model {
	return reported(m, configID(name), target.Environment{Tools: tools})
}

func withLogs(m model, id target.ID, text string) model {
	next, _ := m.Update(launcher.LogsPeeked{ID: id, Text: text})
	return next.(model)
}

func withNotice(m model, text string) model {
	next, _ := m.Update(launcher.NoticeSet{Text: text})
	return next.(model)
}

func reload(m model, notice string) (tea.Model, tea.Cmd) {
	next, load := m.Update(launcher.ReloadRequested{Notice: notice})
	return next.(model).Update(load())
}

func runCmd(m model, make func(*model) tea.Cmd) model {
	cmd := make(&m)
	return applyCmd(m, cmd)
}

func only[E launcher.Effect](effects []launcher.Effect) []launcher.Effect {
	var kept []launcher.Effect
	for _, effect := range effects {
		if _, ok := effect.(E); ok {
			kept = append(kept, effect)
		}
	}
	return kept
}

func peekCmd(m *model) tea.Cmd {
	state, effects := m.state.Apply(time.Now(), launcher.Ticked{})
	m.state = state
	return m.perform(only[launcher.PeekLogs](effects))
}

func probing(m model) bool {
	_, effects := m.state.Apply(time.Now(), launcher.Ticked{})
	return len(only[launcher.Check](effects)) == 0
}

func probed(m model, name string, up bool) model {
	if up {
		return reported(m, configID(name), target.Environment{Readings: linuxReadings(0, 4, 0, 0, 0)})
	}
	next, _ := m.Update(launcher.Checked{ID: configID(name), Full: true, Status: target.StatusDown})
	return next.(model)
}

func configID(name string) target.ID {
	return target.ID{Source: target.ConfigSource, Key: name}
}

func itemNamed(m model, name string) item {
	for _, p := range m.panes {
		for _, it := range p.items {
			if it.name() == name {
				return it
			}
		}
	}
	return item{}
}

const termenvANSIProfile = 2

func stylingEnabled(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenvANSIProfile)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func applyCmd(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, queued := range msg {
			m = applyCmd(m, queued)
		}
	default:
		next, followUp := m.Update(msg)
		m = next.(model)
		if !m.state.LogsOpen() {
			m = applyCmd(m, followUp)
		}
	}
	return m
}

func keyThenApply(m model, s string) model {
	next, cmd := m.Update(tea.KeyMsg{Type: specialKeys[s]})
	return applyCmd(next.(model), cmd)
}

func key(m model, s string) model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	if special, ok := specialKeys[s]; ok {
		msg = tea.KeyMsg{Type: special}
	}
	next, _ := m.Update(msg)
	return next.(model)
}

var specialKeys = map[string]tea.KeyType{
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"tab":       tea.KeyTab,
	"shift+tab": tea.KeyShiftTab,
	"ctrl+e":    tea.KeyCtrlE,
	"ctrl+l":    tea.KeyCtrlL,
	"ctrl+s":    tea.KeyCtrlS,
	"ctrl+x":    tea.KeyCtrlX,
}

func discovered(key string, status target.Status) inventory.Discovered {
	return inventory.Discovered{Status: status, Target: target.Target{
		ID:    target.ID{Key: key},
		Name:  key,
		Group: "vms",
		Watch: true,
		Modes: []target.Mode{{Name: "shell", Argv: []string{"sh"}}},
	}}
}

func arrive(m model, provider string, found ...inventory.Discovered) model {
	next, _ := m.Update(launcher.DiscoveryDone{Provider: provider, Found: found})
	return next.(model)
}

func startedWithProviders(t *testing.T, providers ...string) model {
	t.Helper()
	backend := &fakeBackend{down: map[string]bool{}}
	m := testModel(nil, backend)
	m.runner.(*system.Runner).Catalog = &fakeCatalog{plan: inventory.Plan{Providers: providers}}
	m.edit = func() (*exec.Cmd, error) { return exec.Command("true"), nil }
	m.exec = func(*exec.Cmd, tea.ExecCallback) tea.Cmd { return nil }
	state, effects := m.state.Apply(time.Now(), launcher.Started{})
	m.state = state
	read := effects[0].(launcher.ReadConfig)
	next, _ := m.Update(launcher.ConfigRead{Generation: read.Generation, Plan: inventory.Plan{Providers: providers}})
	return sized(next.(model))
}
