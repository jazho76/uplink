package dashboard

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

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
