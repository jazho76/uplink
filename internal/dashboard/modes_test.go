package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

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
