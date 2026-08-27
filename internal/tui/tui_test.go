package tui

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/humanize"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/target"
)

type fakeHost struct{}

func (fakeHost) ID() string { return target.ProviderLocal }

func (f fakeHost) List() ([]target.Target, error) {
	return []target.Target{{
		Provider: target.ProviderLocal,
		Name:     "host",
		Status:   target.StatusRunning,
		Modes: []target.Mode{
			{Name: "tmux", Argv: []string{"tmux"}},
			{Name: "shell", Argv: []string{"/bin/sh"}},
			{Name: "top", Argv: []string{"/bin/sh", "-c", "htop"}, Back: true},
		},
		Detail: []target.Field{{Key: "os", Value: "Linux"}},
	}}, nil
}

func (fakeHost) Tail(string, int) string { return "journal line" }

func (fakeHost) Probe(string) (probe.Stats, error) {
	return probe.Stats{Cores: 8, Load: 0.42, MemUsed: 1 << 30, MemTotal: 8 << 30}, nil
}

type fakeVMs struct {
	targets []target.Target
	logs    string
}

func (fakeVMs) ID() string { return target.ProviderLima }

func (*fakeVMs) Section() target.Section {
	return target.Section{Name: "vms", Placeholder: []string{"no vms", "uplink vm create <template>"}}
}

func (f *fakeVMs) List() ([]target.Target, error) { return f.targets, nil }

func (f *fakeVMs) Start(string, io.Writer) error { return nil }
func (f *fakeVMs) Stop(string) error             { return nil }
func (f *fakeVMs) Delete(string) error           { return nil }

func (f *fakeVMs) Autostart(string) bool             { return false }
func (f *fakeVMs) SetAutostart(string, bool) error   { return nil }
func (f *fakeVMs) Tail(string, int) string           { return f.logs }
func (f *fakeVMs) Probe(string) (probe.Stats, error) { return probe.Stats{Load: 0.10}, nil }

func vm(name, status string) target.Target {
	return target.Target{
		Provider: target.ProviderLima,
		Section:  "vms",
		Name:     name,
		Status:   target.Status(status),
		CPUs:     6,
		Memory:   12 << 30,
		Modes: []target.Mode{
			{Name: "tmux", Argv: []string{"limactl", "shell", name}},
			{Name: "shell", Argv: []string{"limactl", "shell", name}},
			{Name: "top", Argv: []string{"limactl", "shell", name, "--", "htop"}, Back: true},
		},
		Detail: []target.Field{{Key: "template", Value: name + "_vm"}},
	}
}

func newTestModel() (model, *fakeVMs) {
	vms := &fakeVMs{targets: []target.Target{vm("forge", "stopped"), vm("tokyo", "stopped")}}
	m := model{
		self:     "/tmp/uplink",
		reg:      target.NewRegistry(fakeHost{}, vms),
		spinner:  spinner.New(),
		input:    textinput.New(),
		hostName: "testhost",
		live:     map[string]liveEntry{},
		tasks:    map[string]string{},
	}
	m.rebuild(nil)
	return m, vms
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
	next, _ := m.Update(loadedMsg{listings: m.reg.Listings(), hostStats: probe.Stats{Cores: 8, Load: 0.42}})
	return next.(model)
}

func listed(targets ...target.Target) target.Listings {
	return target.Listings{{Provider: target.ProviderLima, Targets: targets}}
}

func panesOf(items ...item) []pane {
	return groupPanes([]group{{items: items}})
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
	if local.items[0].t.Provider != target.ProviderLocal {
		t.Fatalf("the first pane should come from the local provider, got %q", local.items[0].t.Provider)
	}
	if vms.items[0].name() != "forge" || vms.items[1].name() != "tokyo" {
		t.Fatalf("unexpected vm order: %q %q", vms.items[0].name(), vms.items[1].name())
	}
}

func TestCapabilitiesFollowProvider(t *testing.T) {
	m, _ := newTestModel()
	m = load(m)

	host, forge := itemNamed(m, "host"), itemNamed(m, "forge")
	if host.caps.lifecycle || host.caps.autostart {
		t.Errorf("host should expose no lifecycle or autostart: %+v", host.caps)
	}
	if !host.caps.probe || !host.caps.tail {
		t.Errorf("host should be probeable and carry its journal: %+v", host.caps)
	}
	if !forge.caps.lifecycle || !forge.caps.autostart || !forge.caps.tail || !forge.caps.probe {
		t.Errorf("vm should expose every capability: %+v", forge.caps)
	}
}

func TestFooterTracksCapabilities(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	m = focus(m, "host")
	host := m.renderFooter()
	for _, absent := range []string{"stop", "restart", "del"} {
		if strings.Contains(host, absent) {
			t.Errorf("host footer should not offer %q: %s", absent, host)
		}
	}
	if !strings.Contains(host, "logs") {
		t.Errorf("the host carries its journal, so its footer offers logs: %s", host)
	}

	m = focus(m, "forge")
	vmFooter := m.renderFooter()
	for _, want := range []string{"connect", "logs", "stop", "restart", "auto", "del", "quit"} {
		if !strings.Contains(vmFooter, want) {
			t.Errorf("vm footer missing %q: %s", want, vmFooter)
		}
	}
}

func TestLoadedMsgSetsStatus(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running"), vm("tokyo", "stopped")}
	m = sized(load(m))

	if forge := itemNamed(m, "forge"); !forge.running() {
		t.Fatalf("forge should be running, got status %q", forge.t.Status)
	}
	if tokyo := itemNamed(m, "tokyo"); tokyo.t.Status != target.StatusStopped {
		t.Fatalf("tokyo should be stopped, got %q", tokyo.t.Status)
	}

	m = focus(m, "forge")
	view := m.View()
	for _, want := range []string{"forge", "tokyo", "host", "running", "template", "forge_vm", "vms"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}

	m = focus(m, "host")
	if !strings.Contains(m.View(), "testhost") {
		t.Errorf("host bar missing hostname")
	}
}

func TestProviderErrorKeepsTargets(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	next, _ := m.Update(loadedMsg{listings: listed(vm("forge", "running")), err: errBoom{}})
	m = next.(model)
	if len(m.panes) != 1 || len(m.panes[0].items) != 1 {
		t.Fatalf("targets from healthy providers should survive, got %d panes", len(m.panes))
	}
	if !strings.Contains(m.status, "boom") {
		t.Errorf("status should surface the provider error, got %q", m.status)
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
	if !m.mode().Back {
		t.Errorf("top carries back: the dashboard should survive it")
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

func TestGlyphDistinguishesKindAndState(t *testing.T) {
	shape := func(provider string, status target.Status) string {
		return glyph(item{t: target.Target{Provider: provider, Status: status}}, plainStyle)
	}

	host := shape(target.ProviderLocal, target.StatusRunning)
	vmUp := shape(target.ProviderLima, target.StatusRunning)
	vmOff := shape(target.ProviderLima, target.StatusStopped)
	remoteUp := shape(target.ProviderRemote, target.StatusRunning)
	remoteDown := shape(target.ProviderRemote, target.StatusUnreachable)
	remoteUnprobed := shape(target.ProviderRemote, target.StatusUnknown)

	distinct := map[string]string{
		"host vs vm":                  host + vmUp,
		"remote vs vm, both up":       remoteUp + vmUp,
		"remote vs vm, both down":     remoteDown + vmOff,
		"unreachable vs never probed": remoteDown + remoteUnprobed,
		"reachable vs unreachable":    remoteUp + remoteDown,
	}
	for name, pair := range distinct {
		half := len(pair) / 2
		if pair[:half] == pair[half:] {
			t.Errorf("%s should be visually distinct, both render %q", name, pair[:half])
		}
	}

	widths := map[string]string{
		"host": host, "vm up": vmUp, "vm off": vmOff,
		"remote up": remoteUp, "remote down": remoteDown, "remote unprobed": remoteUnprobed,
	}
	for name, g := range widths {
		if w := lipgloss.Width(g); w != 1 {
			t.Errorf("%s glyph occupies %d columns, want 1", name, w)
		}
	}
}

func TestUnknownStatusIsStillProbed(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = []target.Target{
		{Provider: target.ProviderLima, Name: "fresh", Status: target.StatusUnknown},
		{Provider: target.ProviderLima, Name: "off", Status: target.StatusStopped},
	}
	m = sized(load(m))

	m = focus(m, "fresh")
	if m.liveFetch() == nil {
		t.Error("an unknown target must be probed, or its status can never resolve")
	}

	m = focus(m, "off")
	if m.liveFetch() != nil {
		t.Error("a stopped target has nothing to probe")
	}
}

func TestMultiLineStatusStaysOnOneRow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	next, _ := m.Update(loadedMsg{
		listings: listed(vm("forge", "running")),
		err:      errors.Join(errBoom{}, errBoom{}),
	})
	m = next.(model)

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
		t.Errorf("a sectionless target should land in a pane named for its provider")
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
	panes := panesOf(
		named(target.ProviderLocal, "local", "host"),
		named(target.ProviderLima, "vms", "vault"),
		named(target.ProviderRemote, "remotes", "riga"),
		named(target.ProviderRemote, "remotes", "lisbon"),
	)

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
	panes := panesOf(
		named(target.ProviderRemote, "remotes", "dojo"),
		named(target.ProviderRemote, "remotes", "dublin"),
		named(target.ProviderRemote, "remotes", "denver"),
	)

	want := []rune{'d', 'u', 'e'}
	for i, it := range panes[0].items {
		if it.key != want[i] {
			t.Errorf("%s took %q, want %q", it.name(), it.key, want[i])
		}
	}
}

func TestItemKeyExhaustionLeavesItemUnaddressable(t *testing.T) {
	panes := panesOf(
		named(target.ProviderLima, "vms", "ab"),
		named(target.ProviderLima, "vms", "ba"),
		named(target.ProviderLima, "vms", "aab"),
		named(target.ProviderLima, "vms", "jkq"),
	)

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
		{"load", 0.34, "4.10 / 12"},
		{"memory", 0.26, "8/31GiB"},
		{"disk", 0.75, "112/150GiB"},
	}, 12), 60))

	var columns []int
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		columns = append(columns, strings.Index(line, "%"))
	}
	for i, at := range columns {
		if at < 0 || at != columns[0] {
			t.Errorf("row %d puts its percent at column %d, want %d:\n%s", i, at, columns[0], block)
		}
	}
}

func TestLoadChartAppearsBelowTheGauges(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running")}
	m = sized(load(m))
	m = focus(m, "forge")

	var history []float64
	for i := 0; i < 20; i++ {
		history = appendSample(history, float64(i%7)/7)
	}
	m.live["forge"] = liveEntry{history: history,
		stats: probe.Stats{Cores: 4, Load: 1, MemUsed: 1 << 30, MemTotal: 4 << 30}}

	preview := ansi.Strip(m.previewBody(70, 40))
	gauges, chart := strings.Index(preview, "disk"), strings.Index(preview, "load · last")
	if chart < 0 {
		t.Fatalf("a roomy preview should carry the chart:\n%s", preview)
	}
	if gauges > chart {
		t.Errorf("the chart belongs under the gauges:\n%s", preview)
	}
	if !strings.Contains(preview, "peak ") {
		t.Errorf("the chart must state the ceiling it scaled to:\n%s", preview)
	}

	cramped := ansi.Strip(m.previewBody(20, 40))
	if strings.Contains(cramped, "load · last") {
		t.Errorf("a pane too narrow to plot drops the chart:\n%s", cramped)
	}
}

func TestLoadChartSpansTheAvailableWidth(t *testing.T) {
	e := liveEntry{stats: probe.Stats{Cores: 4, Load: 1}}
	for i := 0; i < 20; i++ {
		e.history = appendSample(e.history, float64(i%7)/7)
	}

	for _, width := range []int{28, 40, 96} {
		block := ansi.Strip(renderLoadChart(e, 0.25, width, 40, 0))
		for i, line := range strings.Split(strings.Trim(block, "\n"), "\n") {
			if got := lipgloss.Width(line); got != width {
				t.Errorf("%d cols: chart line %d spans %d", width, i, got)
			}
		}
	}
}

func TestLoadChartPeakDescribesWhatIsPlotted(t *testing.T) {
	e := liveEntry{stats: probe.Stats{Cores: 10}}
	e.history = appendSample(e.history, 0.9)
	for i := 0; i < 60; i++ {
		e.history = appendSample(e.history, 0.2)
	}

	narrow := ansi.Strip(renderLoadChart(e, 0.2, 30, 40, 0))
	if strings.Contains(narrow, "peak 9.00") {
		t.Errorf("a spike scrolled off the chart must not be claimed as its peak:\n%s", narrow)
	}
	if !strings.Contains(narrow, "peak 2.00") {
		t.Errorf("the peak should be the tallest column actually drawn:\n%s", narrow)
	}

	wide := ansi.Strip(renderLoadChart(e, 0.2, 100, 40, 0))
	if !strings.Contains(wide, "peak 9.00") {
		t.Errorf("a chart wide enough to include the spike should report it:\n%s", wide)
	}
}

func TestLoadChartWindowFollowsWhatIsVisible(t *testing.T) {
	e := liveEntry{stats: probe.Stats{Cores: 4}}
	for i := 0; i < maxLiveSamples; i++ {
		e.history = appendSample(e.history, 0.3)
	}

	const narrowWidth = 30
	wide := ansi.Strip(renderLoadChart(e, 0.3, 120, 40, 0))
	narrow := ansi.Strip(renderLoadChart(e, 0.3, narrowWidth, 40, 0))
	if wide == narrow {
		t.Fatalf("a wider chart shows a longer window")
	}
	if !strings.Contains(narrow, humanize.Duration(narrowWidth*liveInterval)) {
		t.Errorf("the window must describe the visible samples, not the whole buffer:\n%s", narrow)
	}
}

func TestPreviewLeadsWithLiveData(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running")}
	m = sized(load(m))
	m = focus(m, "forge")
	m.live["forge"] = liveEntry{stats: probe.Stats{Cores: 4, Load: 1, MemUsed: 1 << 30, MemTotal: 4 << 30}}

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

func TestStoppedTargetShowsNoStaleUptime(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running")}
	m = sized(load(m))
	m = focus(m, "forge")
	m.live["forge"] = liveEntry{stats: probe.Stats{Cores: 4, Uptime: 76 * time.Hour}}

	if got := m.uptimeOf(m.selected()); got == "" {
		t.Fatalf("a running target should report uptime")
	}

	vms.targets = []target.Target{vm("forge", "stopped")}
	m = load(m)
	m = focus(m, "forge")
	if got := m.uptimeOf(m.selected()); got != "" {
		t.Errorf("a stopped target must not show the uptime from its last run, got %q", got)
	}
	if strings.Contains(ansi.Strip(m.previewBody(60, 20)), "live") {
		t.Errorf("a stopped target has no live block, so its header has nothing to report")
	}
}

func TestPaneSummaryWaitsForKnownStatuses(t *testing.T) {
	up := item{t: target.Target{Status: target.StatusRunning}}
	off := item{t: target.Target{Status: target.StatusStopped}}
	unprobed := item{t: target.Target{Status: target.StatusUnknown}}

	if got := paneSummary(pane{items: []item{up, off}}); got != "1 up" {
		t.Errorf("want %q, got %q", "1 up", got)
	}
	if got := paneSummary(pane{items: []item{up, off, unprobed}}); got != "" {
		t.Errorf("one unprobed member must suppress the count, got %q", got)
	}
	if got := paneSummary(pane{items: []item{up}}); got != "" {
		t.Errorf("a lone member is its own summary, got %q", got)
	}
	if got := paneSummary(pane{}); got != "" {
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
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running"), vm("tokyo", "running")}
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
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running"), vm("vault", "running")}
	m = sized(load(m))

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

	rows := distributeRows(panes, 20, 20)
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

	for _, r := range distributeRows(panes, 20, 8) {
		if r < minPaneRows {
			t.Errorf("an overfull column must not starve a pane, got %d", r)
		}
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	m = key(m, "ctrl+x")
	if m.screen != screenConfirm {
		t.Fatalf("ctrl+x on a vm should enter the confirm screen")
	}

	m.input.SetValue("nope")
	m = key(m, "enter")
	if m.screen != screenList || m.status != "aborted" {
		t.Fatalf("mismatched name should abort, got screen=%v status=%q", m.screen, m.status)
	}
}

func TestDeleteRejectedWithoutLifecycle(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "host")

	m = key(m, "ctrl+x")
	if m.screen != screenList {
		t.Fatalf("ctrl+x on the host should do nothing, got screen %v", m.screen)
	}
}

func TestLogsScreenToggle(t *testing.T) {
	m, vms := newTestModel()
	vms.logs = "boot line"
	m = sized(load(m))
	m = focus(m, "forge")

	m = key(m, "ctrl+l")
	if m.screen != screenLogs {
		t.Fatalf("ctrl+l on a vm should open the log pager")
	}
	if m.logName != "forge" {
		t.Fatalf("log pager should target forge, got %q", m.logName)
	}
	if !strings.Contains(m.View(), "boot line") {
		t.Errorf("log pager should render the tail")
	}

	m = key(m, "esc")
	if m.screen != screenList {
		t.Fatalf("esc should close the log pager")
	}

	vms.logs = ""
	m = key(m, "ctrl+l")
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
		m, vms := newTestModel()
		vms.logs = strings.Repeat("a log line that is quite long indeed\n", 20)
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

func TestSpinnerAnimatesOnlyWhileTasksRun(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	if _, idle := m.Update(m.spinner.Tick()); idle != nil {
		t.Errorf("an idle list should not keep rebuilding frames ten times a second")
	}

	next, started := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(model)
	if m.tasks["forge"] != verbStop {
		t.Fatalf("ctrl+s should mark forge stopping, got %q", m.tasks["forge"])
	}
	if !kicksTheSpinner(started) {
		t.Errorf("starting a task must set the spinner back in motion")
	}
	if _, running := m.Update(m.spinner.Tick()); running == nil {
		t.Errorf("a running task keeps the spinner ticking")
	}
}

func kicksTheSpinner(cmd tea.Cmd) bool {
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		return false
	}
	for _, queued := range batch {
		if _, tick := queued().(spinner.TickMsg); tick {
			return true
		}
	}
	return false
}

func TestConcurrentTaskGuards(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	m = key(m, "ctrl+r")
	if m.tasks["forge"] != verbRestart {
		t.Fatalf("ctrl+r should mark forge restarting, got %q", m.tasks["forge"])
	}
	m = key(m, "ctrl+s")
	if m.tasks["forge"] != verbRestart {
		t.Fatalf("a second action on a busy VM must not change its task")
	}

	view := m.View()
	if !strings.Contains(view, verbRestart) {
		t.Errorf("list should show the inline %q verb", verbRestart)
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 90 {
			t.Errorf("inline task overran width: %d", w)
		}
	}

	next, _ := m.Update(actionMsg{name: "forge", status: "restarted forge"})
	m = next.(model)
	if m.hasTask("forge") {
		t.Fatalf("actionMsg should clear the task")
	}
}

const termenvANSIProfile = 2

func stylingEnabled(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenvANSIProfile)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func named(provider, section, name string) item {
	return item{t: target.Target{Provider: provider, Section: section, Name: name}}
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
	"ctrl+x":    tea.KeyCtrlX,
	"ctrl+l":    tea.KeyCtrlL,
	"ctrl+r":    tea.KeyCtrlR,
	"ctrl+s":    tea.KeyCtrlS,
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

	if m = focus(m, "top"); !m.mode().Back {
		t.Errorf("a back mode should still return to the dashboard")
	}
}

func TestLaunchRowsWithoutModesSurvive(t *testing.T) {
	bare := item{t: target.Target{Provider: target.ProviderLocal, Name: "host"}}
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

	rows := distributeRows([]pane{launcher, vms}, 20, 20)
	if rows[0] != len(launcher.items)+borderCells {
		t.Errorf("a launcher pane cannot use extra rows, got %d", rows[0])
	}
	if rows[0]+rows[1] != 20 {
		t.Errorf("the slack should still be spent, got %d of 20", rows[0]+rows[1])
	}
}

func TestPaneSummarySkipsLauncherPanes(t *testing.T) {
	up := item{t: target.Target{Status: target.StatusRunning}, launcher: true}
	if got := paneSummary(pane{items: []item{up, up, up}}); got != "" {
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
	m, vms := newTestModel()
	vms.targets = []target.Target{vm("forge", "running"), vm("tokyo", "running")}
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

func TestDeclaredSectionKeepsItsPaneWhenEmpty(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = nil
	m = sized(load(m))

	if len(m.panes) != 2 {
		t.Fatalf("the vms pane should outlive its last vm, got %d panes", len(m.panes))
	}
	empty := m.panes[1]
	if empty.section != "vms" || len(empty.items) != 0 {
		t.Fatalf("want an empty vms pane, got %q with %d items", empty.section, len(empty.items))
	}
	if empty.key != 0 {
		t.Errorf("an inert pane takes no jump key, got %q", empty.key)
	}
	if !strings.Contains(m.View(), "no vms") {
		t.Error("the empty pane should say why it is empty")
	}
}

func TestUndeclaredSectionVanishesWhenEmpty(t *testing.T) {
	panes := groupPanes([]group{
		{items: []item{named(target.ProviderLocal, "local", "host")}},
		{section: "vms", placeholder: []string{"no vms"}},
		{},
	})

	if len(panes) != 2 {
		t.Fatalf("only a declared section survives with no targets, got %d panes", len(panes))
	}
	if panes[0].section != "local" || panes[1].section != "vms" {
		t.Fatalf("want the local and vms panes, got %q and %q", panes[0].section, panes[1].section)
	}
	if len(panes[1].placeholder) != 1 {
		t.Errorf("the declared section should carry its placeholder, got %+v", panes[1])
	}
}

func TestCursorSkipsAnEmptyPane(t *testing.T) {
	m, vms := newTestModel()
	vms.targets = nil
	m = sized(load(m))

	for _, want := range []string{"shell", "top", "tmux"} {
		m = key(m, "down")
		if got := m.selected().label(); got != want {
			t.Fatalf("down should reach %q, got %q", want, got)
		}
	}
	if got := m.focusedSection(); got != "local" {
		t.Errorf("an empty pane must never take the focus, got %q", got)
	}
}

func TestFocusLeavesTheSectionItEmptied(t *testing.T) {
	m, vms := newTestModel()
	m = sized(load(m))
	m = focus(m, "forge")

	vms.targets = nil
	m = load(m)

	if got := m.focusedSection(); got != "local" {
		t.Errorf("deleting the last vm should hand the focus back, got %q", got)
	}
	if m.selected().name() == "" {
		t.Error("the focus should land on a real target")
	}
}

func TestEmptyPaneGetsNoSlack(t *testing.T) {
	launcher := pane{section: "local", items: []item{{launcher: true}, {launcher: true}}}
	empty := pane{section: "vms", placeholder: []string{"no vms"}}
	remotes := pane{section: "remotes", items: []item{{}, {}}}

	rows := distributeRows([]pane{launcher, empty, remotes}, 20, 20)
	if rows[1] != minPaneRows {
		t.Errorf("an empty pane should not absorb slack, got %d", rows[1])
	}
	if sum := rows[0] + rows[1] + rows[2]; sum != 20 {
		t.Errorf("the slack should still be spent, got %d of 20", sum)
	}

	fresh := distributeRows([]pane{launcher, empty}, 20, 20)
	if fresh[1] != minPaneRows || fresh[0]+fresh[1] != 20 {
		t.Errorf("with nothing else to grow the launcher takes the slack, got %v", fresh)
	}
}

func TestPlaceholderKeepsOnlyTheLinesThatFit(t *testing.T) {
	m, _ := newTestModel()
	p := pane{section: "vms", placeholder: []string{"no vms", "uplink vm create <template>"}}

	wide := m.renderPane(p, false, 40, 4)
	if !strings.Contains(wide, "no vms") || !strings.Contains(wide, "uplink vm create <template>") {
		t.Errorf("a wide pane shows the whole placeholder, got:\n%s", wide)
	}

	narrow := m.renderPane(p, false, minPaneColumnW, 4)
	if !strings.Contains(narrow, "no vms") {
		t.Errorf("the first placeholder line must survive at %d cols, got:\n%s", minPaneColumnW, narrow)
	}
	if strings.Contains(narrow, "uplink") {
		t.Errorf("a line that does not fit is dropped, not truncated, got:\n%s", narrow)
	}
	for _, line := range strings.Split(narrow, "\n") {
		if got := lipgloss.Width(line); got != minPaneColumnW {
			t.Errorf("pane line spans %d, want %d", got, minPaneColumnW)
		}
	}
}

func TestPlaceholderStaysHiddenWhileTheSectionHasTargets(t *testing.T) {
	panes := groupPanes([]group{{
		section:     "vms",
		placeholder: []string{"no vms"},
		items:       []item{named(target.ProviderLima, "vms", "forge")},
	}})

	m, _ := newTestModel()
	if got := m.renderPane(panes[0], false, 40, 6); strings.Contains(got, "no vms") {
		t.Errorf("a section with targets never shows its placeholder, got:\n%s", got)
	}
	if got := paneBodyRows(panes[0], paneTextWidth(40)); got != 1 {
		t.Errorf("a populated pane is sized by its items, got %d rows", got)
	}
}
