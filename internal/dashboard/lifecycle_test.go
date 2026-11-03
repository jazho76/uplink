package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

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
