package dashboard

import (
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/launcher"
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
	readings := make([]target.Reading, len(t.Stats.Metrics))
	for i, m := range t.Stats.Metrics {
		readings[i] = reading(m, 0.1, 4)
	}
	return target.Environment{Readings: readings, Tools: f.tools[t.Name]}, nil
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

func hostTarget() target.Target {
	return target.Target{
		ID:    configID("host"),
		Name:  "host",
		Group: "local",
		Logs:  target.LogSource{Command: "journalctl"},
		Stats: target.StatsPolicy{Metrics: linuxMetrics},
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
		ID:    configID(name),
		Name:  name,
		Group: "vms",
		Reach: target.Reach{Interactive: via},
		Logs:  target.LogSource{Command: "tail serial.log"},
		Stats: target.StatsPolicy{Metrics: linuxMetrics},
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

func testModel(targets []target.Target, backend launcher.Backend) model {
	return newModel(targets, launcher.NewRunner(backend, nil))
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

func sized(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	return next.(model)
}

func TestRebuildOrdersHostFirst(t *testing.T) {
	m, _ := newTestModel()
	if len(m.panes) != 2 {
		t.Fatalf("want a local and a vms pane, got %d", len(m.panes))
	}
	local, vms := m.panes[0], m.panes[1]
	if local.group != "local" || local.items[0].name() != "host" {
		t.Fatalf("the first pane should hold the host, got %q", local.group)
	}
	if vms.items[0].name() != "forge" || vms.items[1].name() != "tokyo" {
		t.Fatalf("unexpected vm order: %q %q", vms.items[0].name(), vms.items[1].name())
	}
}

func TestCapabilitiesFollowTheConfig(t *testing.T) {
	m, _ := newTestModel()
	quiet := remote("dojo")
	quiet.Logs, quiet.Stats = target.LogSource{}, target.StatsPolicy{}
	m = withTargets(m, vm("forge"), quiet)

	m = sized(focus(m, "forge"))
	if !strings.Contains(m.renderFooter(), "logs") || !strings.Contains(ansi.Strip(m.previewBody(60, 30)), "live") {
		t.Errorf("a target with logs and stats offers both")
	}
	m = focus(m, "dojo")
	if strings.Contains(m.renderFooter(), "logs") || strings.Contains(ansi.Strip(m.previewBody(60, 30)), "live") {
		t.Errorf("a target without logs or stats offers neither")
	}
}

func TestFooterTracksCapabilities(t *testing.T) {
	m, _ := newTestModel()
	quiet := remote("dojo")
	quiet.Logs = target.LogSource{}
	m = sized(withTargets(m, vm("forge"), quiet))

	m = focus(m, "forge")
	vmFooter := m.renderFooter()
	for _, want := range []string{"connect", "logs", "quit"} {
		if !strings.Contains(vmFooter, want) {
			t.Errorf("vm footer missing %q: %s", want, vmFooter)
		}
	}
	for _, gone := range []string{"stop", "restart", "auto", "del"} {
		if strings.Contains(vmFooter, gone) {
			t.Errorf("lifecycle lives outside uplink, footer offers %q: %s", gone, vmFooter)
		}
	}

	m = focus(m, "dojo")
	if strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("a target without logs should not offer them: %s", m.renderFooter())
	}
}

func TestProbeResultSetsStatus(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	if got := m.statusOf(itemNamed(m, "forge").target); got != statusUnknown {
		t.Fatalf("an unprobed target is unknown, got %q", got)
	}
	m = probed(m, "forge", true)
	m = probed(m, "tokyo", false)
	if got := m.statusOf(itemNamed(m, "forge").target); got != statusUp {
		t.Fatalf("a successful probe marks forge up, got %q", got)
	}
	if got := m.statusOf(itemNamed(m, "tokyo").target); got != statusDown {
		t.Fatalf("a failed probe marks tokyo down, got %q", got)
	}

	m = focus(m, "forge")
	view := m.View()
	for _, want := range []string{"forge", "tokyo", "local", "up", "vms"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestReachabilityStandsInForStats(t *testing.T) {
	m, backend := newTestModel()
	quiet := remote("dojo")
	quiet.Stats = target.StatsPolicy{}
	m = sized(withTargets(m, quiet))
	m = focus(m, "dojo")

	m = m.withSelectionRefreshed()
	if got := m.statusOf(itemNamed(m, "dojo").target); got != statusUp {
		t.Fatalf("a reachable target without stats is up, got %q", got)
	}
	if strings.Contains(ansi.Strip(m.previewBody(60, 30)), "live") {
		t.Errorf("a target without stats has no live block")
	}

	backend.down["dojo"] = true
	m.state = launcher.New(m.state.Targets(), nil).Select(m.selected().id())
	m = m.withSelectionRefreshed()
	if got := m.statusOf(itemNamed(m, "dojo").target); got != statusDown {
		t.Errorf("an unreachable target is down, got %q", got)
	}
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

func withDefaultModes(name string) target.Target {
	t := vm(name)
	t.Modes = []target.Mode{
		{Name: "tmux", Argv: append(slices.Clone(t.Reach.Interactive), "tmux"), Requires: "tmux", Multiplexer: "tmux"},
		{Name: "shell", Argv: t.Reach.Interactive},
	}
	return t
}

func focusedOn(name string) (model, *fakeBackend) {
	m, backend := newTestModel()
	m = sized(withTargets(m, withDefaultModes(name)))
	return focus(m, name), backend
}

func inspected(m model, name string, tools ...string) model {
	return reported(m, configID(name), target.Environment{Tools: tools})
}

func TestUninspectedTargetOffersTmux(t *testing.T) {
	m, backend := focusedOn("box")
	if got := m.state.Mode().Name; got != "tmux" {
		t.Errorf("want tmux before anything is known, got %q", got)
	}
	key(m, "enter")
	if want := []string{"ssh lima-box tmux"}; !slices.Equal(backend.sessions, want) {
		t.Errorf("want %q, got %q", want, backend.sessions)
	}
}

func TestTargetWithoutTmuxOffersOnlyTheShell(t *testing.T) {
	m, backend := focusedOn("box")
	m = inspected(m, "box")

	if got := m.state.Mode().Name; got != "shell" {
		t.Errorf("want shell as the default, got %q", got)
	}
	if got := m.modeTag(m.selected()); got != "" {
		t.Errorf("a single mode needs no tag, got %q", got)
	}
	if strings.Contains(m.renderFooter(), "mode") {
		t.Errorf("a single mode has nothing to cycle: %s", m.renderFooter())
	}
	key(m, "enter")
	if want := []string{"ssh lima-box"}; !slices.Equal(backend.sessions, want) {
		t.Errorf("want %q, got %q", want, backend.sessions)
	}
}

func TestTargetWithTmuxDefaultsToIt(t *testing.T) {
	m, _ := focusedOn("box")
	m = inspected(m, "box", "tmux")

	if got := m.state.Mode().Name; got != "tmux" {
		t.Errorf("want tmux as the default, got %q", got)
	}
	if got := ansi.Strip(m.modeTag(m.selected())); !strings.Contains(got, "1/2") {
		t.Errorf("want both modes counted, got %q", got)
	}
}

func TestInspectionRunsAndFiltersThroughTheBackend(t *testing.T) {
	m, backend := focusedOn("box")
	backend.tools = map[string][]string{"box": {"tmux"}}
	if got := m.withSelectionRefreshed().state.Mode().Name; got != "tmux" {
		t.Errorf("a reported tool keeps its mode, got %q", got)
	}

	m, _ = focusedOn("box")
	if got := m.withSelectionRefreshed().state.Mode().Name; got != "shell" {
		t.Errorf("no reported tool drops its mode, got %q", got)
	}
}

func TestStoppedTargetKeepsItsLastAnswer(t *testing.T) {
	m, _ := focusedOn("box")
	m = probed(inspected(m, "box"), "box", false)
	if got := m.state.Mode().Name; got != "shell" {
		t.Errorf("a stopped target still has no tmux, got %q", got)
	}
}

func TestFailedInspectionStaysOptimistic(t *testing.T) {
	m, _ := focusedOn("box")
	if got := failedInspection(m, configID("box")).state.Mode().Name; got != "tmux" {
		t.Errorf("a failed inspection knows nothing about tmux, got %q", got)
	}
}

func TestDeclaredModesAreNeverFiltered(t *testing.T) {
	m, _ := newTestModel()
	m = focus(sized(m), "forge")
	m = inspected(m, "forge")
	if got := m.state.Modes(m.selected().target); len(got) != 3 {
		t.Errorf("modes from the config always stay, got %d", len(got))
	}
}

func TestModeChosenBeforeFilteringLeavesNoBadge(t *testing.T) {
	m, _ := focusedOn("box")
	m = key(m, "tab")
	if got := m.state.Mode().Name; got != "shell" {
		t.Fatalf("tab should reach shell, got %q", got)
	}

	m = inspected(m, "box")
	if got := m.state.Mode().Name; got != "shell" {
		t.Errorf("want shell, got %q", got)
	}
	if row := ansi.Strip(m.paneRow(m.selected(), true, true, 40)); strings.Contains(row, "[") {
		t.Errorf("the only mode is the default, got %q", row)
	}
}

func focusedOnAutomaticModes(names ...string) model {
	box := vm("box")
	box.Modes = nil
	for _, name := range names {
		mode := target.Mode{Name: name, Argv: box.Reach.Interactive}
		if name != "shell" {
			mode.Requires, mode.Multiplexer = name, name
		}
		box.Modes = append(box.Modes, mode)
	}
	m, _ := newTestModel()
	return focus(sized(withTargets(m, box)), "box")
}

func offered(m model) []string {
	var names []string
	for _, mode := range m.state.Modes(m.selected().target) {
		names = append(names, mode.Name)
	}
	return names
}

func TestSelectedModeSurvivesTheListChanging(t *testing.T) {
	m := key(focusedOnAutomaticModes("tmux", "zellij", "shell"), "tab")
	if got := m.state.Mode().Name; got != "shell" {
		t.Fatalf("tab should reach shell, got %q", got)
	}

	m = inspected(m, "box", "zellij")
	if got := m.state.Mode().Name; got != "shell" {
		t.Errorf("shell is still there, want it selected, got %q", got)
	}
}

func TestSelectedModeFallsBackToTheFirstWhenItDisappears(t *testing.T) {
	m := inspected(focusedOnAutomaticModes("tmux", "zellij", "shell"), "box", "tmux", "zellij")
	m = key(m, "tab")
	if got := m.state.Mode().Name; got != "zellij" {
		t.Fatalf("tab should reach zellij, got %q", got)
	}

	m = inspected(m, "box", "tmux")
	if got := m.state.Mode().Name; got != "tmux" {
		t.Errorf("zellij is gone, want the first mode, got %q", got)
	}
}

func TestUndiscoveredTargetOffersOnlyTheDefaultAndShell(t *testing.T) {
	box := configID("box")
	send := func(event launcher.Event) func(model) model {
		return func(m model) model {
			next, _ := m.Update(event)
			return next.(model)
		}
	}
	for name, arrange := range map[string]func(model) model{
		"never checked":     func(m model) model { return m },
		"down":              send(launcher.Checked{ID: box, Full: true, Status: target.StatusDown}),
		"up, not inspected": send(launcher.Checked{ID: box, Status: target.StatusUp}),
		"failed inspection": func(m model) model { return failedInspection(m, box) },
	} {
		m := arrange(focusedOnAutomaticModes("tmux", "zellij", "shell"))
		if got := offered(m); !slices.Equal(got, []string{"tmux", "shell"}) {
			t.Errorf("%s: want tmux and shell, got %q", name, got)
		}
		if got := key(key(m, "tab"), "tab").state.Mode().Name; got != "tmux" {
			t.Errorf("%s: two tabs should wrap to tmux, got %q", name, got)
		}
	}
}

func TestDiscoveryDecidesTheModes(t *testing.T) {
	m := focusedOnAutomaticModes("tmux", "zellij", "shell")

	if got := offered(inspected(m, "box", "tmux", "zellij")); !slices.Equal(got, []string{"tmux", "zellij", "shell"}) {
		t.Errorf("both tools: want all three, got %q", got)
	}
	if got := offered(inspected(m, "box", "zellij")); !slices.Equal(got, []string{"zellij", "shell"}) {
		t.Errorf("only zellij: want zellij and shell, got %q", got)
	}
}

func TestDiscoveredModesStayOnceDown(t *testing.T) {
	m := probed(inspected(focusedOnAutomaticModes("tmux", "zellij", "shell"), "box", "tmux", "zellij"), "box", false)

	if got := offered(m); !slices.Equal(got, []string{"tmux", "zellij", "shell"}) {
		t.Errorf("want what was found, got %q", got)
	}
	if got := ansi.Strip(m.modeTag(m.selected())); strings.Contains(got, "?") {
		t.Errorf("tmux was confirmed, got %q", got)
	}
}

func TestShellDefaultOffersOnlyShellUntilDiscovery(t *testing.T) {
	m := probed(focusedOnAutomaticModes("shell", "tmux", "zellij"), "box", false)

	if got := offered(m); !slices.Equal(got, []string{"shell"}) {
		t.Errorf("want only shell, got %q", got)
	}
	if got := m.modeTag(m.selected()); got != "" {
		t.Errorf("a single mode needs no tag, got %q", got)
	}
	if got := ansi.Strip(m.renderFooter()); strings.Contains(got, "mode") {
		t.Errorf("nothing to cycle, got %q", got)
	}
}

func TestMissingDefaultFallsToShellBeforeOtherMultiplexers(t *testing.T) {
	box := vm("box")
	box.Modes = []target.Mode{
		{Name: "tmux", Argv: box.Reach.Interactive, Requires: "tmux", Multiplexer: "tmux"},
		{Name: "shell", Argv: box.Reach.Interactive},
		{Name: "zellij", Argv: box.Reach.Interactive, Requires: "zellij", Multiplexer: "zellij"},
	}
	m, _ := newTestModel()
	m = focus(sized(withTargets(m, box)), "box")

	m = inspected(m, "box", "zellij")
	if got := m.state.Mode().Name; got != "shell" {
		t.Errorf("the default is missing, want shell first, got %q", got)
	}
	if got := key(m, "tab").state.Mode().Name; got != "zellij" {
		t.Errorf("zellij is one tab away, got %q", got)
	}
}

func TestModeCycling(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	if got := m.state.Mode().Name; got != "tmux" {
		t.Fatalf("a fresh row starts on its default mode, got %q", got)
	}

	m = key(m, "tab")
	if got := m.state.Mode().Name; got != "shell" {
		t.Errorf("tab should advance to shell, got %q", got)
	}
	m = key(m, "tab")
	if got := m.state.Mode().Name; got != "top" {
		t.Errorf("tab should advance to top, got %q", got)
	}
	m = key(m, "tab")
	if got := m.state.Mode().Name; got != "tmux" {
		t.Errorf("tab should wrap around to tmux, got %q", got)
	}

	m = key(m, "shift+tab")
	if got := m.state.Mode().Name; got != "top" {
		t.Errorf("shift+tab should wrap backwards to top, got %q", got)
	}
}

func TestModeResetsWhenCursorMoves(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	m = key(m, "tab")
	if m.state.ModeIndex() == 0 {
		t.Fatal("tab should leave the row off-default")
	}

	m = key(m, "down")
	if m.state.ModeIndex() != 0 {
		t.Errorf("moving the cursor must reset the mode, got index %d", m.state.ModeIndex())
	}

	m = key(m, "tab")
	m = key(m, "up")
	if m.state.ModeIndex() != 0 {
		t.Errorf("moving back must also reset, got index %d", m.state.ModeIndex())
	}
}

func TestModeSurfacedInListAndPreview(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	if strings.Contains(m.renderPanes(30, 12), "[tmux]") {
		t.Errorf("the default mode should stay out of the list row")
	}
	if !strings.Contains(m.renderFooter(), "tab") {
		t.Errorf("footer should advertise tab for a multi-mode target")
	}

	m = key(m, "tab")
	if !strings.Contains(m.renderPanes(30, 12), "[shell]") {
		t.Errorf("an off-default mode should be marked on the row: %s", m.renderPanes(30, 12))
	}
	preview := m.previewBody(50, 20)
	for _, want := range []string{"shell", "2/3"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview missing %q: %s", want, preview)
		}
	}
}

func TestGlyphDistinguishesState(t *testing.T) {
	shape := func(s string) string {
		mark, _ := glyphOf(s)
		return mark
	}

	up := shape(statusUp)
	down := shape(statusDown)
	unknown := shape(statusUnknown)
	starting := shape(statusStarting)

	distinct := map[string]string{
		"up vs down":          up + down,
		"down vs unknown":     down + unknown,
		"starting vs down":    starting + down,
		"starting vs unknown": starting + unknown,
	}
	for name, pair := range distinct {
		half := len(pair) / 2
		if pair[:half] == pair[half:] {
			t.Errorf("%s should be visually distinct, both render %q", name, pair[:half])
		}
	}

	for name, g := range map[string]string{"up": up, "down": down, "unknown": unknown, "starting": starting} {
		if w := lipgloss.Width(g); w != 1 {
			t.Errorf("%s glyph occupies %d columns, want 1", name, w)
		}
	}
}

func TestGroupsBecomePanesInConfigOrder(t *testing.T) {
	m, _ := newTestModel()
	stray := remote("stray")
	stray.Group = target.DefaultGroup
	m = withTargets(m, remote("berlin"), vm("forge"), stray, remote("dojo"))

	var groups []string
	for _, p := range m.panes {
		groups = append(groups, p.group)
	}
	if got := strings.Join(groups, " "); got != "local remotes vms ungrouped" {
		t.Errorf("panes follow first appearance, got %q", got)
	}
	if remotes := m.panes[1]; len(remotes.items) != 2 || remotes.items[1].name() != "dojo" {
		t.Errorf("a later member joins its group's pane, got %+v", remotes.items)
	}
}

func TestStartAndStopRunTheHooks(t *testing.T) {
	m, backend := newTestModel()
	box := vm("nakama")
	box.Power = &target.Power{Up: "limactl start nakama", Down: "limactl stop nakama"}
	m = sized(withTargets(m, box, vm("plain")))
	m = focus(m, "nakama").withSelectionRefreshed()

	if footer := m.renderFooter(); strings.Contains(footer, "start") || !strings.Contains(footer, "stop") {
		t.Errorf("an up target offers stop only: %s", footer)
	}
	if m = keyThenApply(m, "ctrl+s"); len(backend.hooks) != 0 {
		t.Errorf("starting an up target does nothing, ran %v", backend.hooks)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = next.(model)
	if got := m.statusOf(itemNamed(m, "nakama").target); got != statusStopping {
		t.Errorf("a running hook shows its transition, got %q", got)
	}
	if again, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX}); again.(model).state.Observation(configID("nakama")).Operation != launcher.OperationStopping {
		t.Errorf("a second press must not stack hooks")
	}
	if footer := m.renderFooter(); strings.Contains(footer, "start") || strings.Contains(footer, "stop") {
		t.Errorf("a target mid-transition offers neither: %s", footer)
	}
	m = finishHook(m, cmd)
	if got := m.statusOf(itemNamed(m, "nakama").target); got != statusDown {
		t.Errorf("after stopping, the target is checked again, got %q", got)
	}

	if footer := m.renderFooter(); !strings.Contains(footer, "start") || strings.Contains(footer, "stop") {
		t.Errorf("a down target offers start only: %s", footer)
	}

	m = pressHook(m, tea.KeyCtrlS)
	if got := m.statusOf(itemNamed(m, "nakama").target); got != statusUp {
		t.Errorf("after starting, the target is up, got %q", got)
	}
	if got := strings.Join(backend.hooks, ", "); got != "down nakama, up nakama" {
		t.Errorf("hooks ran %q", got)
	}

	m = focus(m, "plain")
	if footer := m.renderFooter(); strings.Contains(footer, "start") || strings.Contains(footer, "stop") {
		t.Errorf("a target without hooks offers neither: %s", footer)
	}
}

func pressHook(m model, key tea.KeyType) model {
	next, cmd := m.Update(tea.KeyMsg{Type: key})
	return finishHook(next.(model), cmd)
}

func finishHook(m model, cmd tea.Cmd) model {
	if cmd == nil {
		return m
	}
	next, recheck := m.Update(cmd())
	return applyCmd(next.(model), recheck)
}

func startable(name string) target.Target {
	t := vm(name)
	t.Power = &target.Power{Up: "limactl start " + name}
	return t
}

func downed(m model, backend *fakeBackend, name string) model {
	backend.down[name] = true
	return focus(m, name).withSelectionRefreshed()
}

func TestEnterRunsTheSelectedMode(t *testing.T) {
	m, backend := newTestModel()
	m = focus(sized(m), "forge").withSelectionRefreshed()

	m = key(m, "enter")
	next, _ := m.Update(launcher.ConnectionEnded{})
	m = key(key(next.(model), "tab"), "enter")
	if got := strings.Join(backend.sessions, ", "); got != "ssh lima-forge tmux, ssh lima-forge" {
		t.Errorf("enter runs the selected mode's argv, ran %q", got)
	}
	if len(backend.hooks) != 0 {
		t.Errorf("an up target needs no start, ran %v", backend.hooks)
	}
}

func TestEnterStartsADownTargetThenConnects(t *testing.T) {
	m, backend := newTestModel()
	m = downed(sized(withTargets(m, startable("nakama"))), backend, "nakama")

	next, start := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if got := m.statusOf(itemNamed(m, "nakama").target); got != statusStarting || len(backend.sessions) != 0 {
		t.Fatalf("a down target starts before anything runs, got %q and sessions %v", got, backend.sessions)
	}

	m = finishHook(m, start)
	if got := strings.Join(backend.sessions, ", "); got != "ssh lima-nakama tmux" {
		t.Errorf("once started, the selected mode runs once, ran %q", got)
	}
}

func TestMovingAwayCancelsTheConnect(t *testing.T) {
	m, backend := newTestModel()
	m = downed(sized(withTargets(m, startable("nakama"), vm("plain"))), backend, "nakama")

	next, start := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = key(next.(model), "down")
	if got := m.selected().name(); got != "plain" {
		t.Fatalf("want plain selected, got %q", got)
	}
	m = finishHook(m, start)
	if len(backend.sessions) != 0 {
		t.Errorf("a start finishing elsewhere connects nothing, ran %v", backend.sessions)
	}
	if got := m.statusOf(itemNamed(m, "nakama").target); got != statusUp {
		t.Errorf("the target still ends up started, got %q", got)
	}
}

func TestFailedStartConnectsNothing(t *testing.T) {
	m, backend := newTestModel()
	m = downed(sized(withTargets(m, startable("jammed"))), backend, "jammed")

	next, start := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = finishHook(next.(model), start)
	if len(backend.sessions) != 0 {
		t.Errorf("a failed start connects nothing, ran %v", backend.sessions)
	}
	if !strings.Contains(m.state.Notice(), "start jammed failed") {
		t.Errorf("the failure reaches the status line, got %q", m.state.Notice())
	}
}

func TestEnterOnADownTargetWithoutUpConnectsAnyway(t *testing.T) {
	m, backend := newTestModel()
	m = downed(sized(withTargets(m, vm("plain"))), backend, "plain")

	if m = key(m, "enter"); len(backend.sessions) != 1 || len(backend.hooks) != 0 {
		t.Errorf("without an up hook, enter just connects, ran %v and hooks %v", backend.sessions, backend.hooks)
	}
}

func TestEnterWhileStoppingDoesNothing(t *testing.T) {
	m, backend := newTestModel()
	box := startable("nakama")
	box.Power.Down = "limactl stop nakama"
	m = focus(sized(withTargets(m, box)), "nakama").withSelectionRefreshed()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if _, cmd := next.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || len(backend.sessions) != 0 {
		t.Errorf("a target mid-stop must not be connected to, ran %v", backend.sessions)
	}
}

func TestStatusAndStatsAreSeparateChecks(t *testing.T) {
	m, backend := newTestModel()
	m = sized(withTargets(m, remote("dojo"), remote("gone")))
	backend.down["gone"] = true

	m = focus(m, "dojo").withSelectionRefreshed()
	if backend.statuses != 1 || backend.inspections != 1 || m.statusOf(itemNamed(m, "dojo").target) != statusUp {
		t.Errorf("an up target runs one status check, then one inspection; ran %d checks and %d inspections", backend.statuses, backend.inspections)
	}

	m = focus(m, "gone").withSelectionRefreshed()
	if backend.statuses != 2 || backend.inspections != 1 || m.statusOf(itemNamed(m, "gone").target) != statusDown {
		t.Errorf("a down target runs its status check and no inspection; ran %d checks and %d inspections", backend.statuses, backend.inspections)
	}
}

func TestResultsFromBeforeAHookAreDropped(t *testing.T) {
	m, backend := newTestModel()
	box := vm("nakama")
	box.Power = &target.Power{Down: "limactl stop nakama"}
	m = sized(withTargets(m, box))
	m = focus(m, "nakama").withSelectionRefreshed()

	m = pressHook(m, tea.KeyCtrlX)
	backend.down["nakama"] = false
	next, _ := m.Update(launcher.Checked{ID: box.ID, Epoch: 0, Status: target.StatusUp})
	m = next.(model)
	if got := m.statusOf(itemNamed(m, "nakama").target); got != statusDown {
		t.Errorf("a check issued before the hook must not overwrite its outcome, got %q", got)
	}
}

func TestFailedHookReportsAndRechecks(t *testing.T) {
	m, _ := newTestModel()
	stuck := vm("stuck")
	stuck.Power = &target.Power{Down: "false"}
	m = sized(withTargets(m, stuck))
	m = focus(m, "stuck").withSelectionRefreshed()

	m = pressHook(m, tea.KeyCtrlX)
	if !strings.Contains(m.state.Notice(), "stop stuck failed") {
		t.Errorf("a failed hook should be reported, got %q", m.state.Notice())
	}
	if got := m.statusOf(itemNamed(m, "stuck").target); got != statusUp {
		t.Errorf("a failed stop leaves the target as it is, got %q", got)
	}
}

func TestPendingProbeShowsNoReadings(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")
	m.send(launcher.Selected{ID: configID("forge")})

	preview := ansi.Strip(m.previewBody(60, 30))
	if strings.Contains(preview, "0.00") || !strings.Contains(preview, "…") {
		t.Errorf("a probe in flight has no readings yet:\n%s", preview)
	}
	if got := m.statusOf(itemNamed(m, "forge").target); got != statusUnknown {
		t.Errorf("a probe in flight leaves the status unknown, got %q", got)
	}
}

func TestReopenedLogsDropTheEarlierRound(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "fresh"
	m = sized(m)
	m = focus(m, "forge")

	m = key(m, "ctrl+l")
	stale := 1
	m = key(key(m, "esc"), "ctrl+l")

	next, tick := m.Update(launcher.LogsViewed{Round: stale, Text: "stale"})
	m = next.(model)
	if m.state.LogText() == "stale" || tick != nil {
		t.Errorf("a reply from a closed log view must neither land nor reschedule")
	}
	if _, tick = m.Update(launcher.LogTicked{Round: stale}); tick != nil {
		t.Errorf("a tick from a closed log view must not fetch again")
	}
}

func TestMultiLineStatusStaysOnOneRow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = withNotice(m, errors.Join(errBoom{}, errBoom{}).Error())

	if lines := strings.Count(m.renderFooter(), "\n"); lines != 1 {
		t.Errorf("footer must stay two rows regardless of error count, got %d newlines", lines)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) > 24 {
		t.Errorf("a joined error overflowed the height budget: %d lines", len(lines))
	}
}

func TestCursorWrapsThroughPanes(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	for _, want := range []string{"forge", "tokyo"} {
		m = key(m, "down")
		if got := m.selected().name(); got != want {
			t.Fatalf("down should reach %q, got %q", want, got)
		}
	}
	if got := m.focusedGroup(); got != "vms" {
		t.Errorf("spilling should carry the focus with it, got %q", got)
	}

	m = key(m, "down")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("down from the last item wraps to the first, got %q", got)
	}
	if got := m.focusedGroup(); got != "local" {
		t.Errorf("wrapping should carry the focus too, got %q", got)
	}

	m = key(m, "up")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("up from the first item lands on the last, got %q", got)
	}
}

func TestSpillResetsTheMode(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	m = key(m, "tab")
	if m.state.ModeIndex() == 0 {
		t.Fatal("tab should leave the row off-default")
	}

	m = key(m, "up")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("up should spill to host, got %q", got)
	}
	if m.state.ModeIndex() != 0 {
		t.Errorf("landing on another pane must reset the mode, got index %d", m.state.ModeIndex())
	}
}

func TestPaneKeysDerivedFromGroup(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	if len(m.panes) != 2 {
		t.Fatalf("want one pane per group, got %d", len(m.panes))
	}
	if m.panes[0].group != "local" || m.panes[1].group != "vms" {
		t.Fatalf("unexpected groups: %q %q", m.panes[0].group, m.panes[1].group)
	}
	if m.panes[0].key != 'l' || m.panes[1].key != 'v' {
		t.Fatalf("unexpected pane keys: %q %q", m.panes[0].key, m.panes[1].key)
	}
	if !strings.Contains(m.View(), "local") {
		t.Errorf("the local target should land in a pane named for its group")
	}
}

func TestPaneFocusRemembersItsCursor(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	m = key(m, "v")
	m = key(m, "down")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("v then down should land on tokyo, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("re-pressing the focused pane's key must be a no-op, got %q", got)
	}

	m = key(m, "l")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("l should focus the local pane, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("a pane should keep its own cursor across a focus change, got %q", got)
	}
}

func TestPaneEdgeIgnoresInvisibleCaptions(t *testing.T) {
	const styledButEmpty = "\x1b[94m\x1b[0m"
	closed := "╰" + strings.Repeat("─", 22) + "╯"

	for _, c := range []struct{ name, left, right string }{
		{name: "no captions"},
		{name: "invisible right", right: styledButEmpty},
		{name: "invisible left", left: styledButEmpty},
		{name: "invisible both", left: styledButEmpty, right: styledButEmpty},
	} {
		edge := ansi.Strip(paneEdge("╰", "╯", c.left, c.right, 24, paneBorder))
		if edge != closed {
			t.Errorf("%s: edge did not close: %q", c.name, edge)
		}
	}

	edge := ansi.Strip(paneEdge("╭", "╮", "vms", "2 up", 24, paneBorder))
	if lipgloss.Width(edge) != 24 {
		t.Errorf("a captioned edge must still span the pane, got %d: %q", lipgloss.Width(edge), edge)
	}
	if !strings.HasPrefix(edge, "╭ vms ") || !strings.HasSuffix(edge, " 2 up ╮") {
		t.Errorf("captions should sit against the corners, got %q", edge)
	}
}

func TestFirstFreeLetterSkipsTaken(t *testing.T) {
	for _, c := range []struct {
		word, taken string
		want        rune
	}{
		{word: "vms", taken: reservedPaneKeys, want: 'v'},
		{word: "remotes", taken: reservedPaneKeys + "r", want: 'e'},
		{word: "Vms", taken: reservedPaneKeys, want: 'v'},
		{word: "kqj", taken: reservedPaneKeys, want: 0},
		{word: "42", taken: "", want: 0},
	} {
		if got := firstFreeLetter(c.word, c.taken); got != c.want {
			t.Errorf("firstFreeLetter(%q, %q) = %q, want %q", c.word, c.taken, got, c.want)
		}
	}
}

func TestPaneKeysFallThroughOnCollision(t *testing.T) {
	panes := []pane{
		{group: "remotes", items: []item{{}}},
		{group: "rigs", items: []item{{}}},
		{group: "queue", items: []item{{}}},
	}
	assignPaneKeys(panes)

	if panes[0].key != 'r' || panes[1].key != 'i' || panes[2].key != 'u' {
		t.Errorf("keys should fall through collisions and reserved letters, got %q %q %q",
			panes[0].key, panes[1].key, panes[2].key)
	}
}

func TestItemKeysAvoidOtherPaneKeys(t *testing.T) {
	panes := []pane{
		{group: "local", items: []item{named("host")}},
		{group: "vms", items: []item{named("vault")}},
		{group: "remotes", items: []item{named("riga"), named("lisbon")}},
	}
	assignPaneKeys(panes)
	assignItemKeys(panes)

	keys := map[string]rune{}
	for _, p := range panes {
		for _, it := range p.items {
			keys[it.name()] = it.key
		}
	}

	if keys["riga"] != 'r' || keys["vault"] != 'v' {
		t.Errorf("an item may claim its own pane's letter, got riga=%q vault=%q", keys["riga"], keys["vault"])
	}
	if keys["lisbon"] != 'i' {
		t.Errorf("l belongs to the local pane, so lisbon should fall through to i, got %q", keys["lisbon"])
	}
}

func TestItemKeysFallThroughWithinPane(t *testing.T) {
	panes := panesOf(named("dojo"), named("dublin"), named("denver"))

	want := []rune{'d', 'u', 'e'}
	for i, it := range panes[0].items {
		if it.key != want[i] {
			t.Errorf("%s took %q, want %q", it.name(), it.key, want[i])
		}
	}
}

func TestItemKeyExhaustionLeavesItemUnaddressable(t *testing.T) {
	panes := panesOf(named("ab"), named("ba"), named("aab"), named("jkq"))

	last := panes[0].items[2]
	if last.key != 0 {
		t.Errorf("an item with no free letter must stay unaddressable, got %q", last.key)
	}
	if reserved := panes[0].items[3]; reserved.key != 0 {
		t.Errorf("reserved letters are never handed out, got %q", reserved.key)
	}
	if got := accentLetter(last.name(), last.key, plainStyle); got != plainStyle.Render(last.name()) {
		t.Errorf("an unaddressable item carries no accent, got %q", got)
	}
	if _, ok := itemForKey(panes[0], "a"); !ok {
		t.Errorf("the first claimant of a letter should still resolve")
	}
}

func TestFieldGutterSizesToTheLongestKey(t *testing.T) {
	block := ansi.Strip(renderFields([]field{
		{"template", "kyoto_vm"},
		{"cpus", "6"},
		{"ssh", "127.0.0.1:38203"},
	}, 60))

	var starts []int
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		starts = append(starts, strings.Index(line, strings.TrimSpace(strings.SplitN(line, "  ", 2)[1])))
	}
	for i, at := range starts {
		if at != starts[0] {
			t.Errorf("line %d starts its value at column %d, want %d:\n%s", i, at, starts[0], block)
		}
	}
	if !strings.HasPrefix(block, "template  kyoto_vm") {
		t.Errorf("the longest key sets the gutter, got %q", block)
	}
}

func TestFieldValuesNeverOverrunTheirWidth(t *testing.T) {
	block := renderFields([]field{{"dir", "/home/jazho/.lima/kyoto/some/deep/path"}}, 20)
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("field line spans %d cells, want at most 20: %q", w, line)
		}
	}
}

func TestLiveGaugesAlignTheirPercentColumn(t *testing.T) {
	block := ansi.Strip(renderFields(gaugeFields([]gaugeRow{
		{label: "load", fraction: 0.34, reading: "4.10 / 12"},
		{label: "memory", fraction: 0.26, reading: "8/31GiB"},
		{label: "gpu", reading: "?", missing: true},
		{label: "disk", fraction: 0.75, reading: "112/150GiB"},
	}, 12), 60))

	var columns []int
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		columns = append(columns, strings.LastIndexAny(line, "%?"))
	}
	for i, at := range columns {
		if at < 0 || at != columns[0] {
			t.Errorf("row %d puts its percent at column %d, want %d:\n%s", i, at, columns[0], block)
		}
	}
}

func TestPreviewLeadsWithLiveData(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(probed(m, "forge", true), "forge")
	m = observed(m, "forge", target.Environment{Readings: linuxReadings(1, 4, 1<<30, 4<<30, 0)})

	preview := ansi.Strip(m.previewBody(60, 30))
	if !strings.Contains(preview, "live") {
		t.Fatalf("preview should carry the live block:\n%s", preview)
	}
	for _, want := range []string{"load", "memory", "disk"} {
		if !strings.Contains(preview, want) {
			t.Errorf("live block missing %q:\n%s", want, preview)
		}
	}
}

func TestDownTargetShowsNoStaleUptime(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(probed(m, "forge", true), "forge")
	m = observed(m, "forge", target.Environment{Readings: linuxReadings(0, 4, 0, 0, 76*time.Hour)})

	if preview := ansi.Strip(m.previewBody(60, 30)); !strings.Contains(preview, "3d 4h") {
		t.Fatalf("a reachable target should report uptime in seconds as a duration:\n%s", preview)
	}

	m = focus(probed(m, "forge", false), "forge")
	if preview := ansi.Strip(m.previewBody(60, 30)); strings.Contains(preview, "3d 4h") {
		t.Errorf("a down target must not show the uptime from its last probe:\n%s", preview)
	}
	if preview := ansi.Strip(m.previewBody(60, 20)); !strings.Contains(preview, "unavailable") || strings.Contains(preview, "load") {
		t.Errorf("a failed probe should say its readings are unavailable, naming no metric:\n%s", preview)
	}
}

func TestPaneSummaryWaitsForKnownStatuses(t *testing.T) {
	m, _ := newTestModel()
	up, off, unprobed := item{target: remote("up")}, item{target: remote("off")}, item{target: remote("new")}
	m = probed(probed(m, "up", true), "off", false)

	if got := m.paneSummary(pane{items: []item{up, off}}); got != "1 up" {
		t.Errorf("want %q, got %q", "1 up", got)
	}
	if got := m.paneSummary(pane{items: []item{up, off, unprobed}}); got != "" {
		t.Errorf("one unprobed member must suppress the count, got %q", got)
	}
	if got := m.paneSummary(pane{items: []item{up}}); got != "1 up" {
		t.Errorf("a lone member is counted like any other, got %q", got)
	}
	if got := m.paneSummary(pane{}); got != "" {
		t.Errorf("an empty pane has nothing to count, got %q", got)
	}
	if got := m.paneSummary(pane{items: []item{up}, compact: true}); got != "" {
		t.Errorf("the local pane shows no count, got %q", got)
	}
}

func TestClipMeasuresDisplayWidth(t *testing.T) {
	if got := truncate("日本語テスト", 4); lipgloss.Width(got) != 4 {
		t.Errorf("clip should count display cells, got %q spanning %d", got, lipgloss.Width(got))
	}
}

func TestPaneLetterThenItemLetterSelects(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)

	if got := m.selected().name(); got != "host" {
		t.Fatalf("focus should start on the first pane, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "forge" {
		t.Fatalf("v should focus the vms pane, got %q", got)
	}

	m = key(m, "t")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("t should select tokyo inside the focused pane, got %q", got)
	}

	m = key(m, "z")
	if got := m.selected().name(); got != "tokyo" {
		t.Fatalf("a letter no item holds must change nothing, got %q", got)
	}
	if m.state.Notice() != "" {
		t.Errorf("a miss must stay silent, got status %q", m.state.Notice())
	}
}

func TestFocusedPaneLetterFallsThroughToItsItems(t *testing.T) {
	m, _ := newTestModel()
	m = sized(withTargets(m, vm("forge"), vm("vault")))

	m = key(m, "v")
	if got := m.selected().name(); got != "forge" {
		t.Fatalf("v should focus the vms pane, got %q", got)
	}

	m = key(m, "v")
	if got := m.selected().name(); got != "vault" {
		t.Fatalf("the focused pane's own letter should reach its items, got %q", got)
	}
}

func TestItemLettersOnlyRenderInFocusedPane(t *testing.T) {
	stylingEnabled(t)

	m, _ := newTestModel()
	m = sized(m)

	forge := m.panes[1].items[0]
	if forge.key == 0 {
		t.Fatalf("forge should have claimed a letter")
	}
	plain := m.paneRow(item{target: forge.target}, false, true, 30)

	if accented := m.paneRow(forge, false, true, 30); accented == plain {
		t.Errorf("the focused pane should mark its item letters: %q", accented)
	}
	if unfocused := m.paneRow(forge, false, false, 30); unfocused != plain {
		t.Errorf("an unfocused pane must not advertise letters that would not work: %q", unfocused)
	}
}

func TestPaneScrollFollowsCursor(t *testing.T) {
	for _, c := range []struct{ cursor, count, rows, want int }{
		{cursor: 0, count: 10, rows: 4, want: 0},
		{cursor: 3, count: 10, rows: 4, want: 0},
		{cursor: 4, count: 10, rows: 4, want: 1},
		{cursor: 9, count: 10, rows: 4, want: 6},
		{cursor: 2, count: 3, rows: 5, want: 0},
	} {
		if got := scrollOffset(c.cursor, c.count, c.rows); got != c.want {
			t.Errorf("scrollOffset(%d, %d, %d) = %d, want %d", c.cursor, c.count, c.rows, got, c.want)
		}
	}
}

func TestPaneRowsSpreadSlackEvenly(t *testing.T) {
	panes := []pane{{items: make([]item, 1)}, {items: make([]item, 2)}, {items: make([]item, 2)}}

	rows := distributeRows(panes, 20)
	sum := 0
	for _, r := range rows {
		sum += r
	}
	if sum != 20 {
		t.Fatalf("panes should fill the column, got %d of 20", sum)
	}
	if rows[0] != 6 || rows[1] != 7 || rows[2] != 7 {
		t.Errorf("slack should spread evenly, got %v", rows)
	}

	for _, r := range distributeRows(panes, 8) {
		if r < minPaneRows {
			t.Errorf("an overfull column must not starve a pane, got %d", r)
		}
	}
}

func TestLocalPaneKeepsItsHeight(t *testing.T) {
	panes := []pane{{items: make([]item, 1), compact: true}, {items: make([]item, 2)}, {items: make([]item, 2)}}
	rows := distributeRows(panes, 20)
	if rows[0] != 1+borderCells || rows[0]+rows[1]+rows[2] != 20 {
		t.Errorf("the local pane should stay one row tall and the rest fill the column, got %v", rows)
	}

	alone := distributeRows([]pane{{items: make([]item, 1), compact: true}}, 20)
	if alone[0] != 20 {
		t.Errorf("a lone local pane still fills the column, got %v", alone)
	}
}

func TestPaneIsCompactWhenNothingInItIsWatched(t *testing.T) {
	watched := vm("forge")
	watched.Watch = true
	stray := vm("stray")
	stray.Group, stray.Watch = "local", true

	m := testModel([]target.Target{hostTarget(), watched}, &fakeBackend{})
	if !m.panes[0].compact || m.panes[1].compact {
		t.Errorf("only the unwatched pane should be compact, got %v and %v", m.panes[0].compact, m.panes[1].compact)
	}
	m = testModel([]target.Target{hostTarget(), stray}, &fakeBackend{})
	if m.panes[0].compact {
		t.Error("a pane mixing in watched targets is not compact")
	}
}

func TestLogsScreenToggle(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "boot line"
	m = sized(m)
	m = focus(m, "forge")

	m = keyThenApply(m, "ctrl+l")
	if !m.state.LogsOpen() {
		t.Fatalf("ctrl+l on a vm should open the log pager")
	}
	if m.state.LogTarget().Name != "forge" {
		t.Fatalf("log pager should target forge, got %q", m.state.LogTarget().Name)
	}
	if !strings.Contains(m.View(), "boot line") {
		t.Errorf("log pager should render the tail")
	}

	m = key(m, "esc")
	if m.state.LogsOpen() {
		t.Fatalf("esc should close the log pager")
	}

	backend.logs["forge"] = ""
	m = keyThenApply(m, "ctrl+l")
	if !strings.Contains(m.View(), "no log output") {
		t.Errorf("a target with nothing to tail should say so:\n%s", m.View())
	}
}

func TestTerminalTooSmall(t *testing.T) {
	m, _ := newTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m = next.(model)
	if !strings.Contains(m.View(), "too small") {
		t.Fatalf("sub-minimum terminal should show the too-small notice")
	}
}

func TestViewWithinBounds(t *testing.T) {
	sizes := []struct{ w, h int }{{minWidth, minHeight}, {40, 14}, {80, 24}, {120, 40}, {52, 16}}
	for _, sz := range sizes {
		m, backend := newTestModel()
		backend.logs["forge"] = strings.Repeat("a log line that is quite long indeed\n", 20)
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m = next.(model)
		m = focus(m, "forge")
		m = m.withSelectionRefreshed()

		lines := strings.Split(m.View(), "\n")
		if len(lines) > sz.h {
			t.Errorf("%dx%d: rendered %d lines, exceeds height", sz.w, sz.h, len(lines))
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > sz.w {
				t.Errorf("%dx%d: line %d width %d exceeds %d", sz.w, sz.h, i, w, sz.w)
			}
		}
	}
}

const termenvANSIProfile = 2

func stylingEnabled(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenvANSIProfile)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func named(name string) item {
	return item{target: target.Target{ID: configID(name), Name: name}}
}

func panesOf(items ...item) []pane {
	panes := []pane{{group: "pane", items: items}}
	assignPaneKeys(panes)
	assignItemKeys(panes)
	return panes
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

func (m model) withSelectionRefreshed() model {
	cmd := m.send(launcher.Selected{ID: m.selected().id()})
	return applyCmd(m, cmd)
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

func TestItemKeysComeFromTargetNames(t *testing.T) {
	m, _ := newTestModel()

	for name, want := range map[string]rune{"host": 'h', "forge": 'f', "tokyo": 't'} {
		if got := itemNamed(m, name).key; got != want {
			t.Errorf("%s should claim %q, got %q", name, want, got)
		}
	}
}

func TestPreviewHeaderNamesTheMode(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	header := func(m model) string { return strings.SplitN(ansi.Strip(m.previewBody(60, 30)), "\n", 2)[0] }

	m = focus(m, "forge")
	if got := header(m); !strings.Contains(got, "tmux") || !strings.Contains(got, "1/") {
		t.Fatalf("the header should name the selected mode: %q", got)
	}
	if got := header(key(m, "tab")); !strings.Contains(got, "2/") {
		t.Errorf("the header mode should follow tab: %q", got)
	}
}

func TestLogPeekBelongsToItsTarget(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	next, _ := m.Update(launcher.LogsPeeked{ID: configID("tokyo"), Text: "tokyo boot"})
	if m = next.(model); m.state.Logs(m.selected().target) != "" {
		t.Errorf("a tail for another target must be dropped, got %q", m.state.Logs(m.selected().target))
	}

	next, _ = m.Update(launcher.LogsPeeked{ID: configID("forge"), Text: "forge boot"})
	if m = next.(model); m.state.Logs(m.selected().target) != "forge boot" {
		t.Errorf("the selected target's tail should land, got %q", m.state.Logs(m.selected().target))
	}
}

func TestCursorSkipsAnEmptyGroup(t *testing.T) {
	m, _ := newTestModel()
	m = sized(withTargets(m))

	if len(m.panes) != 1 {
		t.Fatalf("a group with no targets gets no pane, got %d panes", len(m.panes))
	}
	m = key(m, "down")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("down in a one-row pane stays on its only row, got %q", got)
	}
}

func TestLiveBlockFollowsTheDeclaredMetrics(t *testing.T) {
	gpu := target.Metric{Type: target.TypeGauge, Label: "gpu", Value: "x", Max: "100"}
	cache := target.Metric{Type: target.TypeGauge, Label: "cache", Value: "x", Max: "x", Unit: target.UnitBytes}
	agents := target.Metric{Type: target.TypeText, Label: "agents", Value: "x"}
	broken := target.Metric{Type: target.TypeGauge, Label: "broken", Value: "x", Max: "1"}

	m, _ := newTestModel()
	box := vm("box")
	box.Stats = target.StatsPolicy{Metrics: []target.Metric{gpu, cache, agents, broken}}
	m = sized(withTargets(m, box))
	m = focus(m, "box")
	m = observed(m, "box", target.Environment{Readings: []target.Reading{reading(gpu, 37, 100), reading(cache, 1<<30, 4<<30), {Metric: agents, Text: "3", OK: true}, {Metric: broken}}})

	preview := ansi.Strip(m.previewBody(70, 40))
	order := []string{"gpu", "37 / 100", "cache", "1/4GiB", "agents", "3", "broken", "?"}
	last := -1
	for _, want := range order {
		at := strings.Index(preview[last+1:], want)
		if at < 0 {
			t.Fatalf("live block missing %q after position %d:\n%s", want, last, preview)
		}
		last += at + 1
	}
	if strings.Contains(preview, "load") || strings.Contains(preview, "memory") {
		t.Errorf("only declared metrics are shown:\n%s", preview)
	}
}

func TestEveryEndedSessionReturnsToTheDashboard(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = probed(focus(m, "forge"), "forge", true)
	m = withNotice(m, "stale message")

	exitErr := exec.Command("sh", "-c", "exit 255").Run()
	for _, c := range []struct {
		err  error
		want string
	}{{nil, ""}, {exitErr, "forge: session ended with exit 255"}} {
		next, cmd := m.Update(launcher.ConnectionEnded{Name: "forge", Err: c.err})
		m = next.(model)
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("a session ending must never close the dashboard")
			}
		}
		if m.state.Notice() != c.want {
			t.Errorf("status = %q, want %q", m.state.Notice(), c.want)
		}
		if !probing(m) {
			t.Errorf("the target should be checked again right away")
		}
		m = probed(m, "forge", true)
	}
}

func TestEditedConfigReloadsInPlace(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = probed(focus(m, "tokyo"), "tokyo", true)

	moved := vm("tokyo")
	moved.Group = "far"
	broken := errors.Join(errors.New(`target "x": no modes`), errors.New(`target "y": no modes`))
	m.runner.Catalog = planOf([]target.Target{hostTarget(), moved, remote("berlin")}, broken)

	if footer := m.renderFooter(); !strings.Contains(footer, "config") {
		t.Errorf("the footer should offer the config editor: %s", footer)
	}
	next, _ := reload(m, "")
	m = next.(model)

	var groups []string
	for _, p := range m.panes {
		groups = append(groups, p.group)
	}
	if got := strings.Join(groups, " "); got != "local far remotes" {
		t.Errorf("panes should follow the new config, got %q", got)
	}
	if got := m.statusOf(itemNamed(m, "tokyo").target); got != statusUp {
		t.Errorf("a surviving target keeps what is known about it, got %q", got)
	}
	if itemNamed(m, "forge").name() != "" {
		t.Errorf("a removed target should disappear")
	}
	if m.state.Notice() != "2 problems, see uplink config check" {
		t.Errorf("problems should be summarized, got %q", m.state.Notice())
	}

	next, _ = reload(m, "config edit: no such file")
	if got := next.(model).state.Notice(); got != "config edit: no such file" {
		t.Errorf("a failure to open the editor wins over the reload summary, got %q", got)
	}
}

func TestReloadDropsReadingsOfChangedStats(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = probed(probed(m, "forge", true), "tokyo", true)

	changed := vm("forge")
	changed.Stats = target.StatsPolicy{Metrics: linuxMetrics[1:2]}
	m.runner.Catalog = planOf([]target.Target{hostTarget(), changed, vm("tokyo")}, nil)

	next, _ := reload(m, "")
	m = next.(model)
	if forge := m.state.Observation(configID("forge")); forge.Stats.Value != nil || forge.Status != target.StatusUp {
		t.Errorf("changed stats drop their readings but keep the status, got %+v", forge)
	}
	if m.state.Observation(configID("tokyo")).Stats.Value == nil {
		t.Errorf("unchanged stats keep their readings")
	}
}

func TestReloadKeepsTheSelectionOnTheSameTarget(t *testing.T) {
	m, _ := newTestModel()
	m = sized(focus(m, "tokyo"))
	m.runner.Catalog = planOf([]target.Target{hostTarget(), vm("forge"), vm("berlin"), vm("tokyo")}, nil)

	next, _ := reload(m, "")
	if got := next.(model).selected().name(); got != "tokyo" {
		t.Errorf("a target added above the selection must not move it, got %q", got)
	}
}

func TestSameNameFromAnotherSourceKeepsItsOwnState(t *testing.T) {
	m, _ := newTestModel()
	theirs := vm("box")
	theirs.ID = target.ID{Source: "lima", Key: "box"}
	theirs.Group = "lima"
	theirs.Power = &target.Power{Down: "limactl stop box"}
	m = sized(withTargets(m, vm("box"), theirs))
	m = probed(m, "box", true)

	m.focus = slices.IndexFunc(m.panes, func(p pane) bool { return p.group == "lima" })
	m.state = m.state.Select(m.selected().id())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = next.(model)

	if got := m.state.Observation(theirs.ID); got.Operation != launcher.OperationStopping || got.Status != target.StatusUnknown {
		t.Errorf("the lima box should be stopping with no borrowed status, got %+v", got)
	}
	if got := m.state.Observation(configID("box")); got.Operation != launcher.OperationNone || got.Status != target.StatusUp {
		t.Errorf("the config box should be untouched, got %+v", got)
	}
}

func TestConfigEditedReportsOnlyFailuresToStart(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 1").Run()
	for err, want := range map[error]string{
		nil:                     "",
		exitErr:                 "",
		errors.New("not found"): "config edit: not found",
	} {
		if got := configEdited(err).(launcher.ReloadRequested).Notice; got != want {
			t.Errorf("configEdited(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestNoTargetsPointsAtTheConfig(t *testing.T) {
	m, backend := newTestModel()
	m = replaceTargets(m, nil)
	m = sized(m)

	for _, k := range []string{"j", "k", "tab", "enter", "ctrl+s", "ctrl+x", "ctrl+l"} {
		m = key(m, k)
	}
	if len(backend.sessions) != 0 || len(backend.hooks) != 0 || m.state.LogsOpen() {
		t.Errorf("keys with no target do nothing, ran %v and hooks %v", backend.sessions, backend.hooks)
	}

	view := ansi.Strip(m.View())
	for _, want := range []string{"no targets", "^e to edit the config", "^e config"} {
		if !strings.Contains(view, want) {
			t.Errorf("the empty dashboard should show %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "connect") {
		t.Errorf("with nothing selected, connect is not offered:\n%s", view)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) != 24 {
		t.Errorf("the empty dashboard should fill the terminal, got %d lines", len(lines))
	}
}

func TestCtrlEOpensTheEditorDirectly(t *testing.T) {
	m, backend := newTestModel()
	m = sized(m)
	if m = keyThenApply(m, "ctrl+e"); strings.Join(backend.sessions, ", ") != "vi /tmp/config.yaml" {
		t.Errorf("ctrl+e should run the editor command, ran %v", backend.sessions)
	}

	m, backend = newTestModel()
	m.edit = func() (*exec.Cmd, error) { return nil, errors.New("read-only file system") }
	m = key(m, "ctrl+e")
	if len(backend.sessions) != 0 || m.state.Notice() != "config edit: read-only file system" {
		t.Errorf("a failed edit runs nothing and says why, ran %v, notice %q", backend.sessions, m.state.Notice())
	}
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
	m.runner.Catalog = &fakeCatalog{plan: inventory.Plan{Providers: providers}}
	m.edit = func() (*exec.Cmd, error) { return exec.Command("true"), nil }
	m.exec = func(*exec.Cmd, tea.ExecCallback) tea.Cmd { return nil }
	state, effects := m.state.Apply(time.Now(), launcher.Started{})
	m.state = state
	read := effects[0].(launcher.ReadConfig)
	next, _ := m.Update(launcher.ConfigRead{Generation: read.Generation, Plan: inventory.Plan{Providers: providers}})
	return sized(next.(model))
}

func TestEmptyDashboardSaysItIsDiscovering(t *testing.T) {
	m := startedWithProviders(t, "lima")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "discovering") {
		t.Errorf("want a discovering pane, got:\n%s", got)
	}
	m = arrive(m, "lima")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "no targets") {
		t.Errorf("once providers have answered, want no targets, got:\n%s", got)
	}
}

func TestFirstArrivalIsSelected(t *testing.T) {
	m := arrive(startedWithProviders(t, "lima"), "lima", discovered("dev", target.StatusUp))
	id := target.ID{Source: "lima", Key: "dev"}
	if m.selected().id() != id || m.state.Selection() != id {
		t.Errorf("want dev selected in both places, got %v and %v", m.selected().id(), m.state.Selection())
	}
	if got := ansi.Strip(m.View()); !strings.Contains(got, "dev") || !strings.Contains(got, "up") {
		t.Errorf("status comes from the provider, got:\n%s", got)
	}
}

func TestRefreshKeepsSelectionAndMode(t *testing.T) {
	dev := discovered("dev", target.StatusUp)
	dev.Target.Modes = []target.Mode{{Name: "one", Argv: []string{"sh"}}, {Name: "two", Argv: []string{"sh"}}}
	m := arrive(startedWithProviders(t, "lima"), "lima", dev, discovered("other", target.StatusUp))
	m = key(m, "tab")
	if got := m.state.Mode().Name; got != "two" {
		t.Fatalf("tab should reach two, got %q", got)
	}
	m = arrive(m, "lima", dev, discovered("other", target.StatusDown))
	if got := m.state.Mode().Name; got != "two" {
		t.Errorf("a refresh must not reset the mode, got %q", got)
	}
	if got := m.selected().name(); got != "dev" {
		t.Errorf("a refresh must not move the selection, got %q", got)
	}
}

func TestSelectionMovesWhenItsTargetVanishes(t *testing.T) {
	m := arrive(startedWithProviders(t, "lima"), "lima", discovered("dev", target.StatusUp), discovered("other", target.StatusUp))
	m = arrive(m, "lima", discovered("other", target.StatusUp))
	want := target.ID{Source: "lima", Key: "other"}
	if m.selected().id() != want || m.state.Selection() != want {
		t.Errorf("selection and core must agree on the neighbor, got %v and %v", m.selected().id(), m.state.Selection())
	}
	m = arrive(m, "lima")
	if m.selected().id() != (target.ID{}) || m.state.Selection() != (target.ID{}) {
		t.Errorf("nothing is left to select, got %v and %v", m.selected().id(), m.state.Selection())
	}
}

func TestSelectionStaysOnTheFirstRowUntilYouMove(t *testing.T) {
	m := startedWithProviders(t, "local", "lima")
	here := discovered("here", target.StatusUp)
	here.Target.Group = "local"
	m = arrive(m, "lima", discovered("dev", target.StatusUp))
	if got := m.selected().name(); got != "dev" {
		t.Fatalf("only dev is here, got %q", got)
	}
	m = arrive(m, "local", here)
	if got := m.selected().name(); got != "here" || m.state.Selection() != m.selected().id() {
		t.Errorf("the first row stays selected as providers answer, got %q", got)
	}

	m = key(m, "down")
	m = arrive(m, "lima", discovered("aaa", target.StatusUp), discovered("dev", target.StatusUp))
	if got := m.selected().name(); got == "here" {
		t.Errorf("once you move, arrivals must not take you back, got %q", got)
	}
}

func TestPinnedTargetComesBackAfterVanishing(t *testing.T) {
	web := discovered("web", target.StatusUp)
	web.Target.Group = "containers"
	other := discovered("other", target.StatusUp)
	m := startedWithProviders(t, "lima", "docker")
	m = arrive(m, "lima", other)
	m = arrive(m, "docker", web)
	m = key(m, "down")
	if got := m.selected().name(); got != "web" {
		t.Fatalf("want web, got %q", got)
	}

	m = arrive(m, "docker")
	if got := m.selected().name(); got != "other" {
		t.Fatalf("with its group gone, selection falls back, got %q", got)
	}
	m = arrive(m, "docker", web)
	if got := m.selected().name(); got != "web" || m.state.Selection() != m.selected().id() {
		t.Errorf("a docker restart brings you back, got %q", got)
	}
}

func waitingOnStartable(t *testing.T) (model, *fakeBackend) {
	t.Helper()
	m, backend := newTestModel()
	box := startable("nakama")
	box.Check = "true"
	box.Logs = target.LogSource{Command: "tail"}
	m = focus(sized(withTargets(m, box, vm("plain"))), "nakama")
	m = key(m, "enter")
	if got := m.state.Connection().Phase; got != launcher.PhaseAwaitingStatus {
		t.Fatalf("want waiting, got %q", got)
	}
	return m, backend
}

func TestPreviewShowsConnectingWithAWayOut(t *testing.T) {
	m, _ := waitingOnStartable(t)
	if got := preview(m); !strings.Contains(got, "connecting") || !strings.Contains(got, "cancel") {
		t.Errorf("want connecting and its hint, got:\n%s", got)
	}
}

func TestEscCancelsTheWaitThenQuits(t *testing.T) {
	m, _ := waitingOnStartable(t)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("the first esc cancels the wait and must not quit")
		}
	}
	m = next.(model)
	if m.state.Connection().Phase != launcher.PhaseNone {
		t.Errorf("got %+v", m.state.Connection())
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("the second esc should quit")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("the second esc should quit")
	}
}

func TestQStillQuitsWhileWaiting(t *testing.T) {
	m, _ := waitingOnStartable(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("q should quit")
	}
}

func TestEditorStaysShutWhileConnecting(t *testing.T) {
	m, backend := waitingOnStartable(t)
	key(m, "ctrl+e")
	if len(backend.sessions) != 0 {
		t.Errorf("a session must not launch on editor exit, ran %v", backend.sessions)
	}
}

func TestWaitResolvesThroughTheRealPath(t *testing.T) {
	m, backend := newTestModel()
	box := startable("nakama")
	box.Check = "check"
	m = focus(sized(withTargets(m, box)), "nakama")
	backend.down["nakama"] = true

	next, ask := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = applyCmd(next.(model), ask)
	if got := strings.Join(backend.hooks, ", "); !strings.Contains(got, "nakama") {
		t.Errorf("a down answer starts it, ran hooks %q", got)
	}
	if got := strings.Join(backend.sessions, ", "); got != "ssh lima-nakama tmux" {
		t.Errorf("then it connects, ran %q", got)
	}
}

func TestEnterPinsTheSelection(t *testing.T) {
	first := discovered("first", target.StatusUp)
	first.Target.Group = "first"
	m := startedWithProviders(t, "a", "b")
	m = arrive(m, "b", discovered("dev", target.StatusUnknown))
	m = key(m, "enter")
	m = arrive(m, "a", first)
	if got := m.selected().name(); got != "dev" {
		t.Errorf("a provider answering must not move a target you chose, got %q", got)
	}
}
