package launcher

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

func startableVM() (target.Target, State) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	return vm, selected(vm, box("other"))
}

func TestStartThenConnectIssuesRecheckAndConnection(t *testing.T) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown})

	state, effects := state.Apply(clock, ConnectRequested{})
	if _, ok := effects[0].(Power); !ok || state.Observation(vm.ID).Operation != OperationStarting {
		t.Fatalf("a stopped target starts first, got %+v", effects)
	}
	_, effects = state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting})
	if len(effects) != 2 {
		t.Fatalf("want recheck and connection, got %+v", effects)
	}
	if _, ok := effects[1].(Connect); !ok {
		t.Errorf("then it connects, got %+v", effects[1])
	}
}

func TestFailedStartDoesNotConnect(t *testing.T) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown})
	state, _ = state.Apply(clock, ConnectRequested{})
	next, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting, Err: errors.New("no")})
	for _, effect := range effects {
		if _, ok := effect.(Connect); ok {
			t.Error("a failed start must not connect")
		}
	}
	if next.Notice() == "" {
		t.Error("the failure is reported")
	}
}

func TestConnectionWalksItsPhases(t *testing.T) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown})
	state, _ = state.Apply(clock, ConnectRequested{})
	state, _ = state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting})
	if got := state.Connection().Phase; got != PhaseEntering {
		t.Errorf("a finished start enters, got %q", got)
	}
	state, _ = state.Apply(clock, ConnectionEnded{})
	if got := state.Connection(); got != (Connection{}) {
		t.Errorf("returning clears the slot, got %+v", got)
	}
	_, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting})
	for _, effect := range effects {
		if _, ok := effect.(Connect); ok {
			t.Error("a later start must not connect again")
		}
	}
}

func TestConnectionEndReportsWithoutBlame(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	for err, want := range map[error]string{
		nil:                     "",
		exitErr:                 "forge: session ended with exit 3",
		errors.New("not found"): "forge: not found",
	} {
		if got := connectionNotice(ConnectionEnded{Name: "forge", Err: err}); got != want {
			t.Errorf("connectionNotice(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestConnectionEndRediscovers(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima").Select(vm.ID)
	_, effects := state.Apply(clock, ConnectionEnded{})
	if countEffects[Discover](effects) != 1 {
		t.Errorf("results from before the session must not hide what changed, got %+v", effects)
	}
}

func TestEnteringIgnoresCancelsAndASecondEnter(t *testing.T) {
	vm, other := box("vm"), box("other")
	state, effects := selected(vm, other).Apply(clock, ConnectRequested{})
	if countEffects[Connect](effects) != 1 || state.Connection().Phase != PhaseEntering {
		t.Fatalf("got %+v and %+v", effects, state.Connection())
	}
	if _, effects := state.Apply(clock, ConnectRequested{}); countEffects[Connect](effects) != 0 {
		t.Errorf("a queued second enter must not start a second session, got %+v", effects)
	}
	moved, _ := state.Apply(clock, Selected{ID: other.ID})
	if got := moved.Connection().Phase; got != PhaseEntering {
		t.Errorf("a session already handed over cannot be cancelled, got %q", got)
	}
}

func TestMovingAwayCancelsAStart(t *testing.T) {
	vm, other := box("vm"), box("other")
	vm.Power = &target.Power{Up: "start"}
	state, _ := selected(vm, other).Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown})
	state, _ = state.Apply(clock, ConnectRequested{})
	state, _ = state.Apply(clock, Selected{ID: other.ID})
	if got := state.Connection(); got != (Connection{}) {
		t.Fatalf("got %+v", got)
	}
	_, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting})
	if countEffects[Connect](effects) != 0 {
		t.Errorf("a start finishing elsewhere connects nothing, got %+v", effects)
	}
}

func TestNothingToRunClearsTheSlot(t *testing.T) {
	vm := box("vm")
	vm.Modes = nil
	state, effects := selected(vm).Apply(clock, ConnectRequested{})
	if len(effects) != 0 || state.Connection() != (Connection{}) {
		t.Errorf("got %+v and %+v", effects, state.Connection())
	}
}

func TestEnterOnUnknownStartableWaitsThenDecides(t *testing.T) {
	vm, state := startableVM()
	state, effects := state.Apply(clock, ConnectRequested{})
	if got := state.Connection().Phase; got != PhaseAwaitingStatus {
		t.Fatalf("got %q", got)
	}
	if countEffects[Check](effects) != 1 || countEffects[Connect](effects)+countEffects[Power](effects) != 0 {
		t.Fatalf("it asks and does nothing else yet, got %+v", effects)
	}

	for name, c := range map[string]struct {
		status  target.Status
		connect int
		power   int
		phase   ConnectionPhase
	}{
		"up enters":                 {target.StatusUp, 1, 0, PhaseEntering},
		"down starts":               {target.StatusDown, 0, 1, PhaseStarting},
		"unknown enters without up": {target.StatusUnknown, 1, 0, PhaseEntering},
	} {
		next, effects := state.Apply(clock, Checked{ID: vm.ID, Status: c.status, Full: true})
		if countEffects[Connect](effects) != c.connect || countEffects[Power](effects) != c.power || next.Connection().Phase != c.phase {
			t.Errorf("%s: got %+v in %q", name, effects, next.Connection().Phase)
		}
	}
}

func TestEnterOnUnknownWithNothingToDecideEntersAtOnce(t *testing.T) {
	vm := box("vm")
	state, effects := selected(vm).Apply(clock, ConnectRequested{})
	if countEffects[Connect](effects) != 1 || state.Connection().Phase != PhaseEntering {
		t.Errorf("no answer changes the outcome, got %+v", effects)
	}
}

func TestWaitingReusesACheckAlreadyComing(t *testing.T) {
	vm, state := startableVM()
	state, effects := state.Apply(clock, Ticked{})
	if countEffects[Check](effects) != 1 {
		t.Fatalf("got %+v", effects)
	}
	state, effects = state.Apply(clock, ConnectRequested{})
	if len(effects) != 0 || state.Connection().Phase != PhaseAwaitingStatus {
		t.Errorf("one check is enough, got %+v", effects)
	}
	if _, effects := state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp}); countEffects[Connect](effects) != 1 {
		t.Errorf("a background result answers it too, got %+v", effects)
	}
}

func TestStaleCheckDoesNotResolveNorWedge(t *testing.T) {
	vm, state := startableVM()
	state, _ = state.Apply(clock, ConnectRequested{})
	current := state.entries[vm.ID]
	current.epoch = 1
	state.entries[vm.ID] = current

	state, effects := state.Apply(clock, Checked{ID: vm.ID, Epoch: 0, Status: target.StatusUp, Full: true})
	if countEffects[Connect](effects) != 0 || state.Connection().Phase != PhaseAwaitingStatus {
		t.Errorf("a stale result decides nothing, got %+v", effects)
	}
	if _, effects := state.Apply(clock, Ticked{}); countEffects[Check](effects) != 1 {
		t.Errorf("the tick asks again, got %+v", effects)
	}
}

func TestWaitCanBeCancelled(t *testing.T) {
	vm, state := startableVM()
	waiting, _ := state.Apply(clock, ConnectRequested{})
	other := box("other")

	cancelled, _ := waiting.Apply(clock, ConnectionCancelled{})
	moved, _ := waiting.Apply(clock, Selected{ID: other.ID})
	powered, effects := waiting.Apply(clock, PowerRequested{Operation: OperationStarting})
	if countEffects[Power](effects) != 1 {
		t.Errorf("the key still does its job, got %+v", effects)
	}
	for name, state := range map[string]State{"esc": cancelled, "moving": moved, "power key": powered} {
		if got := state.Connection(); got != (Connection{}) {
			t.Errorf("%s: got %+v", name, got)
		}
		if _, effects := state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp}); countEffects[Connect](effects) != 0 {
			t.Errorf("%s: nothing connects afterwards, got %+v", name, effects)
		}
	}
}

func TestCancellingAStartLetsItFinishWithoutConnecting(t *testing.T) {
	vm, state := startableVM()
	state, _ = state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown})
	state, _ = state.Apply(clock, ConnectRequested{})
	state, _ = state.Apply(clock, ConnectionCancelled{})
	if got := state.Observation(vm.ID).Operation; got != OperationStarting {
		t.Errorf("the start carries on, got %q", got)
	}
	if _, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting}); countEffects[Connect](effects) != 0 {
		t.Errorf("without connecting, got %+v", effects)
	}
}

func TestDiscoveredTargetWaitsOnItsProvider(t *testing.T) {
	vm := limaVM("vm")
	vm.Power = &target.Power{Up: "start"}
	state := startedWith(t, "lima", "docker").Select(vm.ID)
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUnknown)})

	state, effects := state.Apply(clock.Add(time.Second), ConnectRequested{})
	if countEffects[Discover](effects) != 1 || state.Connection().Phase != PhaseAwaitingStatus {
		t.Fatalf("it asks its provider, got %+v", effects)
	}
	still, effects := state.Apply(clock, DiscoveryDone{Provider: "docker"})
	if countEffects[Connect](effects) != 0 || still.Connection().Phase != PhaseAwaitingStatus {
		t.Errorf("another provider answers nothing, got %+v", effects)
	}
	next, effects := still.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusDown)})
	if countEffects[Power](effects) != 1 || next.Connection().Phase != PhaseStarting {
		t.Errorf("down starts, got %+v", effects)
	}
	gone, _ := still.Apply(clock, DiscoveryDone{Provider: "lima"})
	if gone.Connection() != (Connection{}) || !strings.Contains(gone.Notice(), "is gone") {
		t.Errorf("a vanished target cancels and says so, got %+v and %q", gone.Connection(), gone.Notice())
	}
}
