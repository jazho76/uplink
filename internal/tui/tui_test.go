package tui

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
)

type fakeBackend struct {
	logs     map[string]string
	down     map[string]bool
	hooks    []string
	statuses int
}

func (f *fakeBackend) Stats(t target.Target) ([]probe.Reading, error) {
	if f.down[t.Name] {
		return nil, errBoom{}
	}
	readings := make([]probe.Reading, len(t.Stats))
	for i, m := range t.Stats {
		readings[i] = reading(m, 0.1, 4)
	}
	return readings, nil
}

var linuxMetrics = probe.Metrics{{Type: probe.TypeLoad}, {Type: probe.TypeMem}, {Type: probe.TypeDisk}, {Type: probe.TypeUptime}}

func reading(m probe.Metric, value, max float64) probe.Reading {
	return probe.Reading{Metric: m, Value: value, Max: max, OK: true}
}

func loadReading(load, cores float64) probe.Reading {
	return reading(linuxMetrics[0], load, cores)
}

func linuxReadings(load, cores, used, total float64, uptime time.Duration) []probe.Reading {
	return []probe.Reading{
		loadReading(load, cores),
		reading(linuxMetrics[1], used, total),
		reading(linuxMetrics[2], used, total),
		reading(linuxMetrics[3], uptime.Seconds(), 0),
	}
}

func (f *fakeBackend) Status(t target.Target) error {
	f.statuses++
	if f.down[t.Name] {
		return errBoom{}
	}
	return nil
}

func (f *fakeBackend) Up(t target.Target) error {
	f.hooks = append(f.hooks, "up "+t.Name)
	f.down[t.Name] = false
	return nil
}

func (f *fakeBackend) Down(t target.Target) error {
	f.hooks = append(f.hooks, "down "+t.Name)
	if t.Name == "stuck" {
		return errBoom{}
	}
	f.down[t.Name] = true
	return nil
}

func (f *fakeBackend) Logs(t target.Target, _ int) string { return f.logs[t.Name] }

func hostTarget() target.Target {
	return target.Target{
		Name:   "host",
		Group:  "local",
		Expand: true,
		Logs:   "journalctl",
		Stats:  linuxMetrics,
		Modes: []target.Mode{
			{Name: "tmux", Argv: []string{"tmux"}},
			{Name: "shell", Argv: []string{"/bin/sh"}},
			{Name: "top", Argv: []string{"/bin/sh", "-c", "htop"}},
		},
	}
}

func vm(name string) target.Target {
	via := []string{"ssh", "lima-" + name}
	return target.Target{
		Name:  name,
		Group: "vms",
		Via:   via,
		Logs:  "tail serial.log",
		Stats: linuxMetrics,
		Modes: []target.Mode{
			{Name: "tmux", Argv: append(via, "tmux")},
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
	m := newModel([]target.Target{hostTarget(), vm("forge"), vm("tokyo")}, backend)
	m.self, m.hostName = "/tmp/uplink", "testhost"
	return m, backend
}

func withTargets(m model, targets ...target.Target) model {
	m.targets = append([]target.Target{hostTarget()}, targets...)
	m.rebuild()
	return m
}

func probed(m model, name string, up bool) model {
	next, _ := m.Update(liveMsg{name: name, up: up, full: true, readings: linuxReadings(0, 4, 0, 0, 0), statsOK: up})
	return next.(model)
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

func load(m model) model {
	next, _ := m.Update(hostStatsMsg{stats: probe.Stats{Cores: 8, Load: 0.42}})
	return next.(model)
}

func sized(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	return next.(model)
}

func TestRebuildOrdersHostFirst(t *testing.T) {
	m, _ := newTestModel()
	m = load(m)
	if len(m.panes) != 2 {
		t.Fatalf("want a local and a vms pane, got %d", len(m.panes))
	}
	local, vms := m.panes[0], m.panes[1]
	if local.section != "local" || local.items[0].name() != "host" {
		t.Fatalf("the first pane should hold the host, got %q", local.section)
	}
	if vms.items[0].name() != "forge" || vms.items[1].name() != "tokyo" {
		t.Fatalf("unexpected vm order: %q %q", vms.items[0].name(), vms.items[1].name())
	}
}

func TestCapabilitiesFollowTheConfig(t *testing.T) {
	m, _ := newTestModel()
	quiet := remote("dojo")
	quiet.Logs, quiet.Stats = "", nil
	m = load(withTargets(m, vm("forge"), quiet))

	m = sized(focus(m, "forge"))
	if !strings.Contains(m.renderFooter(), "logs") || !strings.Contains(ansi.Strip(m.previewBody(60, 30)), "live") {
		t.Errorf("a target with logs and stats offers both")
	}
	m = focus(m, "dojo")
	if strings.Contains(m.renderFooter(), "logs") || strings.Contains(ansi.Strip(m.previewBody(60, 30)), "live") {
		t.Errorf("a target without logs or stats offers neither")
	}
	if m.liveFetch() == nil {
		t.Errorf("a target without stats is still checked for reachability")
	}
}

func TestFooterTracksCapabilities(t *testing.T) {
	m, _ := newTestModel()
	quiet := remote("dojo")
	quiet.Logs = ""
	m = sized(load(withTargets(m, vm("forge"), quiet)))

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
	m = sized(load(m))

	if got := m.statusOf(itemNamed(m, "forge").t); got != statusUnknown {
		t.Fatalf("an unprobed target is unknown, got %q", got)
	}
	m = probed(m, "forge", true)
	m = probed(m, "tokyo", false)
	if got := m.statusOf(itemNamed(m, "forge").t); got != statusUp {
		t.Fatalf("a successful probe marks forge up, got %q", got)
	}
	if got := m.statusOf(itemNamed(m, "tokyo").t); got != statusDown {
		t.Fatalf("a failed probe marks tokyo down, got %q", got)
	}

	m = focus(m, "forge")
	view := m.View()
	for _, want := range []string{"forge", "tokyo", "host", "up", "lima-forge", "vms"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}

	m = focus(m, "host")
	if !strings.Contains(m.View(), "testhost") {
		t.Errorf("host bar missing hostname")
	}
}

func TestReachabilityStandsInForStats(t *testing.T) {
	m, backend := newTestModel()
	quiet := remote("dojo")
	quiet.Stats = nil
	m = sized(load(withTargets(m, quiet)))
	m = focus(m, "dojo")

	m = m.withSelectionRefreshed()
	if got := m.statusOf(itemNamed(m, "dojo").t); got != statusUp {
		t.Fatalf("a reachable target without stats is up, got %q", got)
	}
	if strings.Contains(ansi.Strip(m.previewBody(60, 30)), "live") {
		t.Errorf("a target without stats has no live block")
	}

	backend.down["dojo"] = true
	m.live = map[string]liveEntry{}
	m = m.withSelectionRefreshed()
	if got := m.statusOf(itemNamed(m, "dojo").t); got != statusDown {
		t.Errorf("an unreachable target is down, got %q", got)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }

func TestModeCycling(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	if got := m.mode().Name; got != "tmux" {
		t.Fatalf("a fresh row starts on its default mode, got %q", got)
	}

	m = key(m, "tab")
	if got := m.mode().Name; got != "shell" {
		t.Errorf("tab should advance to shell, got %q", got)
	}
	m = key(m, "tab")
	if got := m.mode().Name; got != "top" {
		t.Errorf("tab should advance to top, got %q", got)
	}
	m = key(m, "tab")
	if got := m.mode().Name; got != "tmux" {
		t.Errorf("tab should wrap around to tmux, got %q", got)
	}

	m = key(m, "shift+tab")
	if got := m.mode().Name; got != "top" {
		t.Errorf("shift+tab should wrap backwards to top, got %q", got)
	}
}

func TestModeResetsWhenCursorMoves(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	m = key(m, "tab")
	if m.modeIdx == 0 {
		t.Fatal("tab should leave the row off-default")
	}

	m = key(m, "down")
	if m.modeIdx != 0 {
		t.Errorf("moving the cursor must reset the mode, got index %d", m.modeIdx)
	}

	m = key(m, "tab")
	m = key(m, "up")
	if m.modeIdx != 0 {
		t.Errorf("moving back must also reset, got index %d", m.modeIdx)
	}
}

func TestModeSurfacedInListAndPreview(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
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
	for _, want := range []string{"mode", "shell", "2 of 3"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview missing %q: %s", want, preview)
		}
	}
}

func TestGlyphDistinguishesState(t *testing.T) {
	shape := func(expand bool, s status) string {
		mark, _ := glyphOf(target.Target{Expand: expand}, s)
		return mark
	}

	host := shape(true, statusUp)
	up := shape(false, statusUp)
	down := shape(false, statusDown)
	unknown := shape(false, statusUnknown)
	starting := shape(false, statusStarting)

	distinct := map[string]string{
		"host vs up":          host + up,
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

	for name, g := range map[string]string{"host": host, "up": up, "down": down, "unknown": unknown, "starting": starting} {
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

	var sections []string
	for _, p := range m.panes {
		sections = append(sections, p.section)
	}
	if got := strings.Join(sections, " "); got != "local remotes vms ungrouped" {
		t.Errorf("panes follow first appearance, got %q", got)
	}
	if remotes := m.panes[1]; len(remotes.items) != 2 || remotes.items[1].name() != "dojo" {
		t.Errorf("a later member joins its group's pane, got %+v", remotes.items)
	}
}

func TestWatchedTargetsAreCheckedInTheBackground(t *testing.T) {
	m, backend := newTestModel()
	watched := vm("nakama")
	watched.Watch = true
	m = withTargets(m, watched, remote("dojo"))
	backend.down["nakama"] = true

	m = applyCmd(m, m.watchFetch())
	if got := m.statusOf(itemNamed(m, "nakama").t); got != statusDown {
		t.Errorf("a watched target resolves without being selected, got %q", got)
	}
	if got := m.statusOf(itemNamed(m, "dojo").t); got != statusUnknown {
		t.Errorf("an unwatched target waits for selection, got %q", got)
	}
	if m.watchFetch() != nil {
		t.Errorf("a fresh check should not repeat before the watch interval")
	}
}

func TestWatchChecksAreCapped(t *testing.T) {
	m, _ := newTestModel()
	var many []target.Target
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		t := vm(name)
		t.Watch = true
		many = append(many, t)
	}
	m = withTargets(m, many...)

	m.watchFetch()
	pending := 0
	for _, e := range m.live {
		if e.pending {
			pending++
		}
	}
	if pending != maxWatchChecks {
		t.Errorf("at most %d checks may be in flight, got %d", maxWatchChecks, pending)
	}
}

func TestStartAndStopRunTheHooks(t *testing.T) {
	m, backend := newTestModel()
	box := vm("nakama")
	box.Up, box.Down = "limactl start nakama", "limactl stop nakama"
	m = sized(load(withTargets(m, box, vm("plain"))))
	m = focus(m, "nakama").withSelectionRefreshed()

	if footer := m.renderFooter(); strings.Contains(footer, "start") || !strings.Contains(footer, "stop") {
		t.Errorf("an up target offers stop only: %s", footer)
	}
	if m = keyThenApply(m, "ctrl+s"); len(backend.hooks) != 0 {
		t.Errorf("starting an up target does nothing, ran %v", backend.hooks)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = next.(model)
	if got := m.statusOf(itemNamed(m, "nakama").t); got != statusStopping {
		t.Errorf("a running hook shows its transition, got %q", got)
	}
	if again, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX}); again.(model).live["nakama"].action != statusStopping {
		t.Errorf("a second press must not stack hooks")
	}
	if footer := m.renderFooter(); strings.Contains(footer, "start") || strings.Contains(footer, "stop") {
		t.Errorf("a target mid-transition offers neither: %s", footer)
	}
	m = finishHook(m, cmd)
	if got := m.statusOf(itemNamed(m, "nakama").t); got != statusDown {
		t.Errorf("after stopping, the target is checked again, got %q", got)
	}

	if footer := m.renderFooter(); !strings.Contains(footer, "start") || strings.Contains(footer, "stop") {
		t.Errorf("a down target offers start only: %s", footer)
	}

	m = pressHook(m, tea.KeyCtrlS)
	if got := m.statusOf(itemNamed(m, "nakama").t); got != statusUp {
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

func TestStatsStandInForTheDefaultStatus(t *testing.T) {
	m, backend := newTestModel()
	hooked := remote("hooked")
	hooked.Status = "check hooked"
	m = sized(load(withTargets(m, remote("dojo"), hooked)))

	m = focus(m, "dojo").withSelectionRefreshed()
	if backend.statuses != 0 || m.statusOf(itemNamed(m, "dojo").t) != statusUp {
		t.Errorf("a successful probe proves the target up without a second round trip, ran %d checks", backend.statuses)
	}

	m = focus(m, "hooked").withSelectionRefreshed()
	if backend.statuses != 1 {
		t.Errorf("a status hook is always consulted, ran %d checks", backend.statuses)
	}
}

func TestResultsFromBeforeAHookAreDropped(t *testing.T) {
	m, backend := newTestModel()
	box := vm("nakama")
	box.Down = "limactl stop nakama"
	m = sized(load(withTargets(m, box)))
	m = focus(m, "nakama").withSelectionRefreshed()

	stale := m.check(box, false)
	m = pressHook(m, tea.KeyCtrlX)
	backend.down["nakama"] = false
	next, _ := m.Update(stale())
	m = next.(model)
	if got := m.statusOf(itemNamed(m, "nakama").t); got != statusDown {
		t.Errorf("a check issued before the hook must not overwrite its outcome, got %q", got)
	}
}

func TestFailedHookReportsAndRechecks(t *testing.T) {
	m, _ := newTestModel()
	stuck := vm("stuck")
	stuck.Down = "false"
	m = sized(load(withTargets(m, stuck)))
	m = focus(m, "stuck").withSelectionRefreshed()

	m = pressHook(m, tea.KeyCtrlX)
	if !strings.Contains(m.status, "stop stuck failed") {
		t.Errorf("a failed hook should be reported, got %q", m.status)
	}
	if got := m.statusOf(itemNamed(m, "stuck").t); got != statusUp {
		t.Errorf("a failed stop leaves the target as it is, got %q", got)
	}
}

func TestEveryStatusIsProbed(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = probed(m, "tokyo", false)

	m = focus(m, "forge")
	if m.liveFetch() == nil {
		t.Error("an unknown target must be probed, or its status can never resolve")
	}

	m = focus(m, "tokyo")
	m.live = map[string]liveEntry{}
	if m.liveFetch() == nil {
		t.Error("a down target must be probed again, or it can never come back up")
	}
}

func TestSlowProbesNeverStack(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	if m.liveFetch() == nil {
		t.Fatal("the first probe should go out")
	}
	if m.liveFetch() != nil {
		t.Error("a probe still running must not be joined by another")
	}

	m = probed(m, "forge", true)
	landed := m.live["forge"]
	landed.at = time.Now().Add(-liveInterval)
	m.live["forge"] = landed
	if m.liveFetch() == nil {
		t.Error("once the result lands and goes stale, the next probe may go out")
	}
}

func TestPendingProbeShowsNoReadings(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")
	m.liveFetch()

	preview := ansi.Strip(m.previewBody(60, 30))
	if strings.Contains(preview, "0.00") || !strings.Contains(preview, "…") {
		t.Errorf("a probe in flight has no readings yet:\n%s", preview)
	}
	if got := m.statusOf(itemNamed(m, "forge").t); got != statusUnknown {
		t.Errorf("a probe in flight leaves the status unknown, got %q", got)
	}
}

func TestReopenedLogsDropTheEarlierRound(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "fresh"
	m = sized(load(m))
	m = focus(m, "forge")

	m = key(m, "ctrl+l")
	stale := m.logRound
	m = key(key(m, "esc"), "ctrl+l")

	next, tick := m.Update(logViewMsg{round: stale, text: "stale"})
	m = next.(model)
	if m.logView == "stale" || tick != nil {
		t.Errorf("a reply from a closed log view must neither land nor reschedule")
	}
	if _, tick = m.Update(logTickMsg{round: stale}); tick != nil {
		t.Errorf("a tick from a closed log view must not fetch again")
	}
}

func TestMultiLineStatusStaysOnOneRow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m.status = errors.Join(errBoom{}, errBoom{}).Error()

	if lines := strings.Count(m.renderFooter(), "\n"); lines != 1 {
		t.Errorf("footer must stay two rows regardless of error count, got %d newlines", lines)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) > 24 {
		t.Errorf("a joined error overflowed the height budget: %d lines", len(lines))
	}
}

func TestCursorWrapsThroughPanes(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	for _, want := range []string{"shell", "top", "forge", "tokyo"} {
		m = key(m, "down")
		if got := m.selected().label(); got != want {
			t.Fatalf("down should reach %q, got %q", want, got)
		}
	}
	if got := m.focusedSection(); got != "vms" {
		t.Errorf("spilling should carry the focus with it, got %q", got)
	}

	m = key(m, "down")
	if got := m.selected().label(); got != "tmux" {
		t.Fatalf("down from the last item wraps to the first, got %q", got)
	}
	if got := m.focusedSection(); got != "local" {
		t.Errorf("wrapping should carry the focus too, got %q", got)
	}

	m = key(m, "up")
	if got := m.selected().label(); got != "tokyo" {
		t.Fatalf("up from the first item lands on the last, got %q", got)
	}
}

func TestSpillResetsTheMode(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	m = key(m, "tab")
	if m.modeIdx == 0 {
		t.Fatal("tab should leave the row off-default")
	}

	m = key(m, "up")
	if got := m.selected().name(); got != "host" {
		t.Fatalf("up should spill to host, got %q", got)
	}
	if m.modeIdx != 0 {
		t.Errorf("landing on another pane must reset the mode, got index %d", m.modeIdx)
	}
}

func TestPaneKeysDerivedFromSection(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	if len(m.panes) != 2 {
		t.Fatalf("want one pane per section, got %d", len(m.panes))
	}
	if m.panes[0].section != "local" || m.panes[1].section != "vms" {
		t.Fatalf("unexpected sections: %q %q", m.panes[0].section, m.panes[1].section)
	}
	if m.panes[0].key != 'l' || m.panes[1].key != 'v' {
		t.Fatalf("unexpected pane keys: %q %q", m.panes[0].key, m.panes[1].key)
	}
	if !strings.Contains(m.View(), "local") {
		t.Errorf("the local target should land in a pane named for its kind")
	}
}

func TestPaneFocusRemembersItsCursor(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

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
		{section: "remotes", items: []item{{}}},
		{section: "rigs", items: []item{{}}},
		{section: "queue", items: []item{{}}},
	}
	assignPaneKeys(panes)

	if panes[0].key != 'r' || panes[1].key != 'i' || panes[2].key != 'u' {
		t.Errorf("keys should fall through collisions and reserved letters, got %q %q %q",
			panes[0].key, panes[1].key, panes[2].key)
	}
}

func TestItemKeysAvoidOtherPaneKeys(t *testing.T) {
	panes := []pane{
		{section: "local", items: []item{named("host")}},
		{section: "vms", items: []item{named("vault")}},
		{section: "remotes", items: []item{named("riga"), named("lisbon")}},
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
	m = sized(load(m))
	m = focus(probed(m, "forge", true), "forge")
	m.live["forge"] = liveEntry{at: time.Now(), state: statusUp, statsOK: true, readings: linuxReadings(1, 4, 1<<30, 4<<30, 0)}

	preview := ansi.Strip(m.previewBody(60, 30))
	live, spec := strings.Index(preview, "live"), strings.Index(preview, "spec")
	if live < 0 || spec < 0 {
		t.Fatalf("preview should carry both sections:\n%s", preview)
	}
	if live > spec {
		t.Errorf("changing data should lead the static detail:\n%s", preview)
	}
	for _, want := range []string{"load", "memory", "disk"} {
		if !strings.Contains(preview, want) {
			t.Errorf("live block missing %q:\n%s", want, preview)
		}
	}
}

func TestDownTargetShowsNoStaleUptime(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(probed(m, "forge", true), "forge")
	m.live["forge"] = liveEntry{at: time.Now(), state: statusUp, statsOK: true, readings: linuxReadings(0, 4, 0, 0, 76*time.Hour)}

	if got := m.uptimeOf(m.selected()); got == "" {
		t.Fatalf("a reachable target should report uptime")
	}

	m = focus(probed(m, "forge", false), "forge")
	if got := m.uptimeOf(m.selected()); got != "" {
		t.Errorf("a down target must not show the uptime from its last probe, got %q", got)
	}
	if preview := ansi.Strip(m.previewBody(60, 20)); !strings.Contains(preview, "unavailable") || strings.Contains(preview, "load") {
		t.Errorf("a failed probe should say its readings are unavailable, naming no metric:\n%s", preview)
	}
}

func TestPaneSummaryWaitsForKnownStatuses(t *testing.T) {
	m, _ := newTestModel()
	up, off, unprobed := item{t: remote("up")}, item{t: remote("off")}, item{t: remote("new")}
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
}

func TestClipMeasuresDisplayWidth(t *testing.T) {
	if got := clip("日本語テスト", 4); lipgloss.Width(got) != 4 {
		t.Errorf("clip should count display cells, got %q spanning %d", got, lipgloss.Width(got))
	}
	if got := clip("a\tb\x00c", 10); got != "a bc" {
		t.Errorf("clip should flatten tabs and drop control bytes, got %q", got)
	}
}

func TestPaneLetterThenItemLetterSelects(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

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
	if m.status != "" {
		t.Errorf("a miss must stay silent, got status %q", m.status)
	}
}

func TestFocusedPaneLetterFallsThroughToItsItems(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(withTargets(m, vm("forge"), vm("vault"))))

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
	m = sized(load(m))

	forge := m.panes[1].items[0]
	if forge.key == 0 {
		t.Fatalf("forge should have claimed a letter")
	}
	plain := m.paneRow(item{t: forge.t}, false, true, 30)

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

func TestLogsScreenToggle(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "boot line"
	m = sized(load(m))
	m = focus(m, "forge")

	m = keyThenApply(m, "ctrl+l")
	if m.screen != screenLogs {
		t.Fatalf("ctrl+l on a vm should open the log pager")
	}
	if m.logTarget.Name != "forge" {
		t.Fatalf("log pager should target forge, got %q", m.logTarget.Name)
	}
	if !strings.Contains(m.View(), "boot line") {
		t.Errorf("log pager should render the tail")
	}

	m = key(m, "esc")
	if m.screen != screenList {
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
		m = load(next.(model))
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
	return item{t: target.Target{Name: name}}
}

func panesOf(items ...item) []pane {
	panes := []pane{{section: "pane", items: items}}
	assignPaneKeys(panes)
	assignItemKeys(panes)
	return panes
}

func focus(m model, name string) model {
	for i, p := range m.panes {
		for j, it := range p.items {
			if it.name() == name || it.label() == name {
				m.focus, m.panes[i].cursor = i, j
				return m
			}
		}
	}
	return m
}

func (m model) withSelectionRefreshed() model {
	return applyCmd(m, m.onSelectionChange())
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
		next, _ := m.Update(msg)
		m = next.(model)
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
	"ctrl+l":    tea.KeyCtrlL,
	"ctrl+s":    tea.KeyCtrlS,
	"ctrl+x":    tea.KeyCtrlX,
}

func TestLocalExpandsToOneRowPerMode(t *testing.T) {
	m, _ := newTestModel()
	m = load(m)

	local := m.panes[0]
	if len(local.items) != 3 {
		t.Fatalf("each local mode should get a row, got %d", len(local.items))
	}
	for i, want := range []string{"tmux", "shell", "top"} {
		row := local.items[i]
		if row.label() != want {
			t.Errorf("row %d should be labelled %q, got %q", i, want, row.label())
		}
		if row.name() != "host" {
			t.Errorf("every row is the same machine, got %q", row.name())
		}
		if len(row.t.Modes) != 1 || row.t.Modes[0].Name != want {
			t.Errorf("row %d should carry only its own mode, got %v", i, row.t.Modes)
		}
	}
}

func TestLaunchRowFixesItsMode(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "shell")

	if got := m.mode().Name; got != "shell" {
		t.Fatalf("a launch row selects its own mode, got %q", got)
	}
	m = key(m, "tab")
	if got := m.mode().Name; got != "shell" {
		t.Errorf("tab has nothing to cycle on a launch row, got %q", got)
	}
	if footer := m.renderFooter(); strings.Contains(footer, "tab") {
		t.Errorf("a launch row should not advertise tab: %s", footer)
	}
	if badge := ansi.Strip(m.renderPanes(30, 20)); strings.Contains(badge, "[shell]") {
		t.Errorf("the row is the mode, so it needs no badge:\n%s", badge)
	}

	if m = focus(m, "top"); m.mode().Name != "top" {
		t.Errorf("each launch row selects its own mode, got %q", m.mode().Name)
	}
}

func TestLaunchRowsWithoutModesSurvive(t *testing.T) {
	bare := item{t: target.Target{Expand: true, Name: "host"}}
	if rows := launcherRows(bare); len(rows) != 1 || rows[0].launcher {
		t.Errorf("a modeless host stays one plain row, got %+v", rows)
	}
}

func TestItemKeysComeFromRowLabels(t *testing.T) {
	m, _ := newTestModel()
	m = load(m)

	for i, want := range []rune{'t', 's', 'o'} {
		if got := m.panes[0].items[i].key; got != want {
			t.Errorf("row %d should claim %q, got %q", i, want, got)
		}
	}
}

func TestLauncherPaneKeepsOnlyWhatItNeeds(t *testing.T) {
	launcher := pane{section: "local", items: []item{{launcher: true}, {launcher: true}, {launcher: true}}}
	vms := pane{section: "vms", items: []item{{}, {}}}

	rows := distributeRows([]pane{launcher, vms}, 20)
	if rows[0] != len(launcher.items)+borderCells {
		t.Errorf("a launcher pane cannot use extra rows, got %d", rows[0])
	}
	if rows[0]+rows[1] != 20 {
		t.Errorf("the slack should still be spent, got %d of 20", rows[0]+rows[1])
	}
}

func TestPaneSummarySkipsLauncherPanes(t *testing.T) {
	m, _ := newTestModel()
	up := item{t: hostTarget(), launcher: true}
	if got := m.paneSummary(pane{items: []item{up, up, up}}); got != "" {
		t.Errorf("launch rows are one machine, not three, got %q", got)
	}
}

func TestPreviewShowsWhatEnterWillRun(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	tmux := ansi.Strip(m.previewBody(60, 30))
	if !strings.Contains(tmux, "run") || !strings.Contains(tmux, "tmux") {
		t.Fatalf("the preview should state the command:\n%s", tmux)
	}

	top := ansi.Strip(focus(m, "top").previewBody(60, 30))
	if !strings.Contains(top, "htop") {
		t.Errorf("the run line should follow the cursor:\n%s", top)
	}
	if tmux == top {
		t.Errorf("moving between launch rows must change something:\n%s", top)
	}
}

func TestLogPeekBelongsToItsTarget(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	next, _ := m.Update(logPeekMsg{name: "tokyo", text: "tokyo boot"})
	if m = next.(model); m.logPeek.text != "" {
		t.Errorf("a tail for another target must be dropped, got %q", m.logPeek.text)
	}

	next, _ = m.Update(logPeekMsg{name: "forge", text: "forge boot"})
	if m = next.(model); m.logPeek.text != "forge boot" {
		t.Errorf("the selected target's tail should land, got %q", m.logPeek.text)
	}
}

func TestLogPeekIsThrottled(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	if cmd := m.peekLogs(); cmd == nil {
		t.Fatalf("the first peek should fetch")
	}
	m.logPeek = peek{name: m.selected().name(), text: "journal line", at: time.Now()}
	if cmd := m.peekLogs(); cmd != nil {
		t.Errorf("a fresh peek should not refetch on every poll")
	}
}

func TestCursorSkipsAnEmptyGroup(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(withTargets(m)))

	if len(m.panes) != 1 {
		t.Fatalf("a kind with no targets gets no pane, got %d panes", len(m.panes))
	}
	for _, want := range []string{"shell", "top", "tmux"} {
		m = key(m, "down")
		if got := m.selected().label(); got != want {
			t.Fatalf("down should reach %q, got %q", want, got)
		}
	}
}

func TestLiveBlockFollowsTheDeclaredMetrics(t *testing.T) {
	gpu := probe.Metric{Type: probe.TypeGauge, Label: "gpu", Value: "x", Max: "100"}
	cache := probe.Metric{Type: probe.TypeGauge, Label: "cache", Value: "x", Max: "x", Unit: probe.UnitBytes}
	agents := probe.Metric{Type: probe.TypeText, Label: "agents", Value: "x"}
	broken := probe.Metric{Type: probe.TypeGauge, Label: "broken", Value: "x", Max: "1"}

	m, _ := newTestModel()
	box := vm("box")
	box.Stats = probe.Metrics{gpu, cache, agents, broken}
	m = sized(load(withTargets(m, box)))
	m = focus(m, "box")
	m.live["box"] = liveEntry{at: time.Now(), state: statusUp, statsOK: true,
		readings: []probe.Reading{reading(gpu, 37, 100), reading(cache, 1<<30, 4<<30), {Metric: agents, Text: "3", OK: true}, {Metric: broken}}}

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

func TestTickRefreshesLogsInPlace(t *testing.T) {
	m, backend := newTestModel()
	m = sized(load(m))
	m = applyCmd(m, m.peekLogs())
	if got := m.logsFor(m.selected()); got != "journal line" {
		t.Fatalf("the first peek should land, got %q", got)
	}

	backend.logs["host"] = "journal line\nnew line"
	if m.peekLogs() != nil {
		t.Fatalf("a selection change right after a peek should not refetch")
	}
	m = applyCmd(m, m.peekSelected())
	if got := m.logsFor(m.selected()); !strings.HasSuffix(got, "new line") {
		t.Errorf("the tick should refresh the tail while staying on a target, got %q", got)
	}

	m.screen = screenLogs
	if m.peekSelected() != nil {
		t.Errorf("the full log screen polls on its own, the preview should not")
	}
}

func TestTickProbesOnCadenceButNeverStacks(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = probed(focus(m, "forge"), "forge", true)

	if m.liveFetch() != nil {
		t.Fatalf("a selection change right after a probe should not refetch")
	}
	if m.probeSelected() == nil {
		t.Errorf("the tick should probe even when the last result just landed")
	}
	if m.probeSelected() != nil {
		t.Errorf("a probe still in flight must not be joined by another")
	}
}

func TestSessionEndReportsWithoutBlame(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	for err, want := range map[error]string{
		nil:                     "",
		exitErr:                 "forge: session ended with exit 3",
		errors.New("not found"): "forge: not found",
	} {
		if got := sessionEnded("forge", err).status; got != want {
			t.Errorf("sessionEnded(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestEveryEndedSessionReturnsToTheDashboard(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = probed(focus(m, "forge"), "forge", true)
	m.status = "stale message"

	for _, ended := range []execDoneMsg{{}, {status: "forge: session ended with exit 255"}} {
		next, cmd := m.Update(ended)
		m = next.(model)
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("a session ending must never close the dashboard")
			}
		}
		if m.status != ended.status {
			t.Errorf("status = %q, want %q", m.status, ended.status)
		}
		if !m.live["forge"].pending {
			t.Errorf("the target should be checked again right away")
		}
		m = probed(m, "forge", true)
	}
}

func TestConfigStatusSummarizesProblems(t *testing.T) {
	one := errors.New(`target "box": no modes`)
	nested := errors.Join(errors.Join(one, errors.New("b")), errors.New("c"))
	for err, want := range map[error]string{
		nil:    "",
		one:    `config: target "box": no modes`,
		nested: "config: 3 problems, see uplink config check",
	} {
		if got := configStatus(err); got != want {
			t.Errorf("configStatus(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestEditedConfigReloadsInPlace(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = probed(focus(m, "tokyo"), "tokyo", true)

	moved := vm("tokyo")
	moved.Group = "far"
	broken := errors.Join(errors.New(`target "x": no modes`), errors.New(`target "y": no modes`))
	m.load = func() ([]target.Target, error) {
		return []target.Target{hostTarget(), moved, remote("berlin")}, broken
	}

	if footer := m.renderFooter(); !strings.Contains(footer, "config") {
		t.Errorf("the footer should offer the config editor: %s", footer)
	}
	next, _ := m.Update(configEditedMsg{})
	m = next.(model)

	var sections []string
	for _, p := range m.panes {
		sections = append(sections, p.section)
	}
	if got := strings.Join(sections, " "); got != "local far remotes" {
		t.Errorf("panes should follow the new config, got %q", got)
	}
	if got := m.statusOf(itemNamed(m, "tokyo").t); got != statusUp {
		t.Errorf("a surviving target keeps what is known about it, got %q", got)
	}
	if itemNamed(m, "forge").name() != "" {
		t.Errorf("a removed target should disappear")
	}
	if m.status != "config: 2 problems, see uplink config check" {
		t.Errorf("problems should be summarized, got %q", m.status)
	}

	next, _ = m.Update(configEditedMsg{status: "config edit: no such file"})
	if got := next.(model).status; got != "config edit: no such file" {
		t.Errorf("a failure to open the editor wins over the reload summary, got %q", got)
	}
}

func TestReloadProbesTheSelectionAgainAtOnce(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = probed(focus(m, "forge"), "forge", true)
	m.load = func() ([]target.Target, error) { return m.targets, nil }

	next, _ := m.Update(configEditedMsg{})
	m = next.(model)
	if !m.live["forge"].pending {
		t.Errorf("a reload must not leave the selection on readings from the old config")
	}
}

func TestReloadDropsReadingsOfChangedStats(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = probed(probed(m, "forge", true), "tokyo", true)

	changed := vm("forge")
	changed.Stats = probe.Metrics{{Type: probe.TypeMem}}
	m.load = func() ([]target.Target, error) { return []target.Target{hostTarget(), changed, vm("tokyo")}, nil }

	next, _ := m.Update(configEditedMsg{})
	m = next.(model)
	if forge := m.live["forge"]; forge.readings != nil || forge.state != statusUp {
		t.Errorf("changed stats drop their readings but keep the status, got %+v", forge)
	}
	if m.live["tokyo"].readings == nil {
		t.Errorf("unchanged stats keep their readings")
	}
}

func TestConfigEditedReportsOnlyFailuresToStart(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 1").Run()
	for err, want := range map[error]string{
		nil:                     "",
		exitErr:                 "",
		errors.New("not found"): "config edit: not found",
	} {
		if got := configEdited(err).(configEditedMsg).status; got != want {
			t.Errorf("configEdited(%v) = %q, want %q", err, got, want)
		}
	}
}
