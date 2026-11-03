package dashboard

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/system"
	"github.com/jazho76/uplink/internal/target"
)

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

func TestHidingTheSelectedTargetMovesToANeighbor(t *testing.T) {
	m := startedWithProviders(t, "lima")
	m = arrive(m, "lima", discovered("dev", target.StatusUp), discovered("other", target.StatusUp))
	m = key(m, "down")
	if got := m.selected().name(); got != "other" {
		t.Fatalf("want other, got %q", got)
	}

	hide := true
	plan := inventory.Plan{Providers: []string{"lima"}, Overlays: []inventory.Overlay{{Match: "lima/other", Hide: &hide}}}
	m.runner.(*system.Runner).Catalog = &fakeCatalog{plan: plan}
	next, _ := reload(m, "")
	m = next.(model)
	want := target.ID{Source: "lima", Key: "dev"}
	if m.selected().id() != want || m.state.Selection() != want {
		t.Errorf("selection and core must agree on the neighbor, got %v and %v", m.selected().id(), m.state.Selection())
	}
	if got := ansi.Strip(m.View()); strings.Contains(got, "other") {
		t.Errorf("a hidden target is not listed, got:\n%s", got)
	}
}

func TestEditedConfigReloadsInPlace(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = probed(focus(m, "tokyo"), "tokyo", true)

	moved := vm("tokyo")
	moved.Group = "far"
	broken := errors.Join(errors.New(`target "x": no modes`), errors.New(`target "y": no modes`))
	m.runner.(*system.Runner).Catalog = planOf([]target.Target{hostTarget(), moved, remote("berlin")}, broken)

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

func TestReloadKeepsTheSelectionOnTheSameTarget(t *testing.T) {
	m, _ := newTestModel()
	m = sized(focus(m, "tokyo"))
	m.runner.(*system.Runner).Catalog = planOf([]target.Target{hostTarget(), vm("forge"), vm("berlin"), vm("tokyo")}, nil)

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
