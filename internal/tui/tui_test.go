package tui

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		Modes:    []target.Mode{{Name: "tmux", Argv: []string{"tmux"}}},
		Detail:   []target.Field{{Key: "os", Value: "Linux"}},
	}}, nil
}

func (fakeHost) Probe(string) (probe.Stats, error) {
	return probe.Stats{Cores: 8, Load: "0.42", MemUsed: 1 << 30, MemTotal: 8 << 30}, nil
}

type fakeVMs struct {
	targets []target.Target
	logs    string
}

func (fakeVMs) ID() string { return target.ProviderLima }

func (f *fakeVMs) List() ([]target.Target, error) { return f.targets, nil }

func (f *fakeVMs) Start(string, io.Writer) error { return nil }
func (f *fakeVMs) Stop(string) error             { return nil }
func (f *fakeVMs) Delete(string) error           { return nil }

func (f *fakeVMs) Autostart(string) bool             { return false }
func (f *fakeVMs) SetAutostart(string, bool) error   { return nil }
func (f *fakeVMs) Tail(string, int) string           { return f.logs }
func (f *fakeVMs) Probe(string) (probe.Stats, error) { return probe.Stats{Load: "0.10"}, nil }

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

func load(m model) model {
	targets, _ := m.reg.All()
	next, _ := m.Update(loadedMsg{targets: targets, hostStats: probe.Stats{Cores: 8, Load: "0.42"}})
	return next.(model)
}

func sized(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	return next.(model)
}

func TestRebuildOrdersHostFirst(t *testing.T) {
	m, _ := newTestModel()
	m = load(m)
	if len(m.items) != 3 {
		t.Fatalf("want 3 items (host + 2 vms), got %d", len(m.items))
	}
	if m.items[0].t.Provider != target.ProviderLocal {
		t.Fatalf("first item should come from the local provider, got %q", m.items[0].t.Provider)
	}
	if m.items[1].name() != "forge" || m.items[2].name() != "tokyo" {
		t.Fatalf("unexpected vm order: %q %q", m.items[1].name(), m.items[2].name())
	}
}

func TestCapabilitiesFollowProvider(t *testing.T) {
	m, _ := newTestModel()
	m = load(m)

	host, forge := m.items[0], m.items[1]
	if host.caps.lifecycle || host.caps.autostart || host.caps.tail {
		t.Errorf("host should expose no lifecycle, autostart, or logs: %+v", host.caps)
	}
	if !host.caps.probe {
		t.Errorf("host should be probeable")
	}
	if !forge.caps.lifecycle || !forge.caps.autostart || !forge.caps.tail || !forge.caps.probe {
		t.Errorf("vm should expose every capability: %+v", forge.caps)
	}
}

func TestFooterTracksCapabilities(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	m.cursor = 0
	host := m.renderFooter()
	for _, absent := range []string{"stop", "restart", "del", "logs"} {
		if strings.Contains(host, absent) {
			t.Errorf("host footer should not offer %q: %s", absent, host)
		}
	}

	m.cursor = 1
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

	if !m.items[1].running() {
		t.Fatalf("forge should be running, got status %q", m.items[1].t.Status)
	}
	if m.items[2].t.Status != target.StatusStopped {
		t.Fatalf("tokyo should be stopped, got %q", m.items[2].t.Status)
	}

	m.cursor = 1
	view := m.View()
	for _, want := range []string{"forge", "tokyo", "host", "running", "template", "forge_vm", "vms"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}

	m.cursor = 0
	if !strings.Contains(m.View(), "testhost") {
		t.Errorf("host bar missing hostname")
	}
}

func TestProviderErrorKeepsTargets(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	next, _ := m.Update(loadedMsg{targets: []target.Target{vm("forge", "running")}, err: errBoom{}})
	m = next.(model)
	if len(m.items) != 1 {
		t.Fatalf("targets from healthy providers should survive, got %d items", len(m.items))
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
	m.cursor = 1

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
	m.cursor = 1

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

func TestSingleModeTargetIgnoresTab(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m.cursor = 0

	m = key(m, "tab")
	if m.modeIdx != 0 {
		t.Errorf("a one-mode target has nothing to cycle, got index %d", m.modeIdx)
	}
	if strings.Contains(m.renderFooter(), "tab") {
		t.Errorf("footer should not advertise tab for a one-mode target")
	}
}

func TestModeSurfacedInListAndPreview(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m.cursor = 1

	if strings.Contains(m.renderList(), "[tmux]") {
		t.Errorf("the default mode should stay out of the list row")
	}
	if !strings.Contains(m.renderFooter(), "tab") {
		t.Errorf("footer should advertise tab for a multi-mode target")
	}

	m = key(m, "tab")
	if !strings.Contains(m.renderList(), "[shell]") {
		t.Errorf("an off-default mode should be marked on the row: %s", m.renderList())
	}
	preview := m.renderPreview(50, 20)
	for _, want := range []string{"mode", "shell", "2 of 3"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview missing %q: %s", want, preview)
		}
	}
}

func TestGlyphDistinguishesKindAndState(t *testing.T) {
	shape := func(provider string, status target.Status) string {
		return glyph(item{t: target.Target{Provider: provider, Status: status}})
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

	m.cursor = 1
	if m.liveFetch() == nil {
		t.Error("an unknown target must be probed, or its status can never resolve")
	}

	m.cursor = 2
	if m.liveFetch() != nil {
		t.Error("a stopped target has nothing to probe")
	}
}

func TestMultiLineStatusStaysOnOneRow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	next, _ := m.Update(loadedMsg{
		targets: []target.Target{vm("forge", "running")},
		err:     errors.Join(errBoom{}, errBoom{}),
	})
	m = next.(model)

	if lines := strings.Count(m.renderFooter(), "\n"); lines != 1 {
		t.Errorf("footer must stay two rows regardless of error count, got %d newlines", lines)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) > 24 {
		t.Errorf("a joined error overflowed the height budget: %d lines", len(lines))
	}
}

func TestCursorBounds(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))

	m = key(m, "up")
	if m.cursor != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", m.cursor)
	}

	for i := 0; i < 10; i++ {
		m = key(m, "down")
	}
	if m.cursor != len(m.items)-1 {
		t.Fatalf("cursor should clamp at %d, got %d", len(m.items)-1, m.cursor)
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m.cursor = 1

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
	m.cursor = 0

	m = key(m, "ctrl+x")
	if m.screen != screenList {
		t.Fatalf("ctrl+x on the host should do nothing, got screen %v", m.screen)
	}
}

func TestLogsScreenToggle(t *testing.T) {
	m, vms := newTestModel()
	vms.logs = "boot line"
	m = sized(load(m))
	m.cursor = 1

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
	sizes := []struct{ w, h int }{{40, 14}, {80, 24}, {120, 40}, {52, 16}}
	for _, sz := range sizes {
		m, vms := newTestModel()
		vms.logs = strings.Repeat("a log line that is quite long indeed\n", 20)
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m = load(next.(model))
		m.cursor = 1
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

func TestConcurrentTaskGuards(t *testing.T) {
	m, _ := newTestModel()
	m = sized(load(m))
	m.cursor = 1

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

func (m model) withSelectionRefreshed() model {
	m.onSelectionChange()
	return m
}

func key(m model, s string) model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	if special, ok := specialKeys[s]; ok {
		next, _ = m.Update(tea.KeyMsg{Type: special})
	}
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
