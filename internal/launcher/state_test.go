package launcher

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

var clock = time.Unix(1000, 0)

func box(name string) target.Target {
	return target.Target{ID: target.ID{Source: target.ConfigSource, Key: name}, Name: name, Modes: []target.Mode{{Name: "shell", Argv: []string{"sh"}}}}
}

func selected(t target.Target, others ...target.Target) State {
	return New(append([]target.Target{t}, others...), nil).Select(t.ID)
}

func TestApplyDoesNotChangeTheOriginalState(t *testing.T) {
	vm := box("vm")
	original := selected(vm)
	next, effects := original.Apply(clock, Ticked{})
	if len(effects) == 0 || !next.entries[vm.ID].checking {
		t.Fatalf("a tick should start a check, got %+v", effects)
	}
	if original.entries[vm.ID].checking {
		t.Error("Apply must leave its receiver alone")
	}
}

func TestFactsFollowEachCheck(t *testing.T) {
	vm := box("vm")
	boom := errors.New("boom")
	env := target.Environment{Tools: []string{"tmux"}}
	later := clock.Add(time.Minute)
	observed, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Up: true, Full: true, Env: env})

	for name, c := range map[string]struct {
		event     Checked
		current   bool
		at        time.Time
		wantState target.Status
	}{
		"up full success": {Checked{ID: vm.ID, Up: true, Full: true, Env: env}, true, later, target.StatusUp},
		"up full failure": {Checked{ID: vm.ID, Up: true, Full: true, InspectErr: boom}, false, later, target.StatusUp},
		"down full":       {Checked{ID: vm.ID, Full: true}, false, later, target.StatusDown},
		"down not full":   {Checked{ID: vm.ID}, false, clock, target.StatusDown},
		"up not full":     {Checked{ID: vm.ID, Up: true}, true, clock, target.StatusUp},
	} {
		next, _ := observed.Apply(later, c.event)
		got := next.Observation(vm.ID)
		if got.Status != c.wantState || got.Stats.Current() != c.current || got.Tools.Current() != c.current || got.Sessions.Current() != c.current {
			t.Errorf("%s: got %+v", name, got)
		}
		if got.Stats.At != c.at {
			t.Errorf("%s: At = %v, want %v", name, got.Stats.At, c.at)
		}
		if !got.Tools.Known {
			t.Errorf("%s: what inspection learned stays known", name)
		}
	}
}

func TestOldCheckIsDroppedButStillPeeksLogs(t *testing.T) {
	vm := box("vm")
	vm.Logs.Command = "tail"
	state := selected(vm)
	current := state.entries[vm.ID]
	current.epoch = 1
	state.entries[vm.ID] = current

	next, effects := state.Apply(clock, Checked{ID: vm.ID, Epoch: 0, Up: true, Full: true})
	if next.Observation(vm.ID).Status != target.StatusUnknown {
		t.Errorf("a check from before a power operation must not settle, got %+v", next.Observation(vm.ID))
	}
	if len(effects) != 1 {
		t.Fatalf("want the log peek, got %+v", effects)
	}
	if _, ok := effects[0].(PeekLogs); !ok {
		t.Errorf("want the log peek, got %+v", effects[0])
	}
}

func TestTickCountsTheSelectedCheckAgainstTheWatchCap(t *testing.T) {
	var watched []target.Target
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		w := box(name)
		w.Watch = true
		watched = append(watched, w)
	}
	_, effects := selected(box("vm"), watched...).Apply(clock, Ticked{})
	if len(effects) != maxWatchChecks {
		t.Errorf("want %d checks including the selected one, got %d", maxWatchChecks, len(effects))
	}
	if first := effects[0].(Check); !first.Full || first.Target.Name != "vm" {
		t.Errorf("the selected target goes first and in full, got %+v", first)
	}
}

func TestOlderReloadIsIgnored(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, ReloadRequested{})
	state, _ = state.Apply(clock, ReloadRequested{})
	next, effects := state.Apply(clock, Reloaded{Generation: 1, Targets: nil})
	if len(next.Targets()) != 1 || effects != nil {
		t.Errorf("an older reload must not win, got %d targets and %+v", len(next.Targets()), effects)
	}
}

func TestReloadWithChangedStatsInvalidatesThem(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Up: true, Full: true, Env: target.Environment{Readings: []target.Reading{{}}, Sessions: []target.Session{{Name: "s"}}}})
	state, _ = state.Apply(clock, ReloadRequested{})

	changed := vm
	changed.Stats.Builtin = true
	next, _ := state.Apply(clock.Add(time.Minute), Reloaded{Generation: 1, Targets: []target.Target{changed}, Selected: vm.ID})
	got := next.Observation(vm.ID)
	if got.Stats.Value != nil || got.Stats.Current() || got.Sessions.Current() {
		t.Errorf("readings of the old stats are gone, got %+v", got)
	}
	if !got.Stats.Known {
		t.Error("clearing readings does not forget that inspection happened")
	}
}

func TestStartThenConnectIssuesRecheckAndConnection(t *testing.T) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID})

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
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID})
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

func TestFinishedStartForgetsThePendingConnection(t *testing.T) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID})
	state, _ = state.Apply(clock, ConnectRequested{})
	state, _ = state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting})
	if state.connectAfterStart != (target.ID{}) {
		t.Errorf("got %q", state.connectAfterStart)
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

func TestProblemsNoticeSummarizes(t *testing.T) {
	one := errors.New(`target "box": no modes`)
	nested := errors.Join(errors.Join(one, errors.New("b")), errors.New("c"))
	for err, want := range map[error]string{
		nil:    "",
		one:    `config: target "box": no modes`,
		nested: "config: 3 problems, see uplink config check",
	} {
		if got := problemsNotice(err); got != want {
			t.Errorf("problemsNotice(%v) = %q, want %q", err, got, want)
		}
	}
}

func countEffects[E Effect](effects []Effect) int {
	count := 0
	for _, effect := range effects {
		if _, ok := effect.(E); ok {
			count++
		}
	}
	return count
}

func withLogs(t target.Target) target.Target {
	t.Logs.Command = "tail"
	return t
}

func TestEveryStatusIsCheckedAgain(t *testing.T) {
	vm, other := box("vm"), box("other")
	state := selected(vm, other)

	_, effects := state.Apply(clock, Selected{ID: other.ID})
	if countEffects[Check](effects) != 1 {
		t.Error("an unknown target must be checked, or its status can never resolve")
	}

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Full: true})
	_, effects = state.Apply(clock.Add(LiveInterval), Selected{ID: vm.ID})
	if countEffects[Check](effects) != 1 {
		t.Error("a down target must be checked again, or it can never come back up")
	}
}

func TestChecksNeverStack(t *testing.T) {
	vm := box("vm")
	state, effects := selected(vm).Apply(clock, Selected{ID: vm.ID})
	if countEffects[Check](effects) != 1 {
		t.Fatal("the first check should go out")
	}
	if _, effects := state.Apply(clock, Selected{ID: vm.ID}); countEffects[Check](effects) != 0 {
		t.Error("a selection change must not join a running check")
	}
	state, effects = state.Apply(clock, Ticked{})
	if countEffects[Check](effects) != 0 {
		t.Error("a tick must not join a running check")
	}

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Up: true, Full: true})
	if _, effects := state.Apply(clock.Add(time.Second), Selected{ID: vm.ID}); countEffects[Check](effects) != 0 {
		t.Error("a selection change right after a check should not repeat it")
	}
	if _, effects := state.Apply(clock.Add(LiveInterval), Selected{ID: vm.ID}); countEffects[Check](effects) != 1 {
		t.Error("once the result goes stale, the next check may go out")
	}
	if _, effects := state.Apply(clock.Add(time.Second), Ticked{}); countEffects[Check](effects) != 1 {
		t.Error("the tick checks on its own cadence, even right after a result")
	}
}

func TestWatchedTargetsAreCheckedInTheBackground(t *testing.T) {
	vm, watched, unwatched := box("vm"), box("watched"), box("unwatched")
	watched.Watch = true
	state, effects := selected(vm, watched, unwatched).Apply(clock, Started{})

	var background []target.ID
	for _, effect := range effects {
		if check := effect.(Check); !check.Full {
			background = append(background, check.Target.ID)
		}
	}
	if len(background) != 1 || background[0] != watched.ID {
		t.Fatalf("only the watched target is checked in the background, got %v", background)
	}

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Up: true, Full: true})
	state, _ = state.Apply(clock, Checked{ID: watched.ID})
	if got := state.Observation(watched.ID).Status; got != target.StatusDown {
		t.Errorf("a watched target resolves without being selected, got %q", got)
	}
	if _, effects := state.Apply(clock.Add(time.Second), Ticked{}); countEffects[Check](effects) != 1 {
		t.Errorf("a fresh watched check should not repeat before its interval, got %+v", effects)
	}
	if _, effects := state.Apply(clock.Add(watchInterval), Ticked{}); countEffects[Check](effects) != 2 {
		t.Errorf("it repeats after its interval, got %+v", effects)
	}
}

func TestLogPeeksNeverStackAndAreThrottled(t *testing.T) {
	vm, other := withLogs(box("vm")), withLogs(box("other"))
	state, effects := selected(vm, other).Apply(clock, Ticked{})
	if countEffects[PeekLogs](effects) != 1 {
		t.Fatal("the first peek should go out")
	}
	if _, effects := state.Apply(clock, Ticked{}); countEffects[PeekLogs](effects) != 0 {
		t.Error("a slow host must not stack up peeks")
	}

	state, _ = state.Apply(clock, LogsPeeked{ID: vm.ID, Text: "line"})
	if _, effects := state.Apply(clock.Add(time.Second), Selected{ID: vm.ID}); countEffects[PeekLogs](effects) != 0 {
		t.Error("a selection change right after a peek should not refetch")
	}
	if _, effects := state.Apply(clock.Add(time.Second), Ticked{}); countEffects[PeekLogs](effects) != 1 {
		t.Error("the tick refreshes the tail while staying on a target")
	}

	moved, effects := state.Apply(clock, Selected{ID: other.ID})
	if countEffects[PeekLogs](effects) != 1 {
		t.Fatal("moving to another target peeks it")
	}
	moved, _ = moved.Apply(clock, Ticked{})
	dropped, _ := moved.Apply(clock, LogsPeeked{ID: vm.ID, Text: "late"})
	if got := dropped.Logs(vm); got != "line" {
		t.Errorf("a tail for another target must not replace the slot, got %q", got)
	}
}

func TestOpenLogScreenStopsPreviewPeeks(t *testing.T) {
	vm := withLogs(box("vm"))
	state, _ := selected(vm).Apply(clock, LogsOpened{})
	if _, effects := state.Apply(clock, Ticked{}); countEffects[PeekLogs](effects) != 0 {
		t.Error("the full log screen polls on its own, the preview should not")
	}
}

func TestReloadChecksTheSelectionAgainAtOnce(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Up: true, Full: true})
	state, _ = state.Apply(clock, ReloadRequested{})
	_, effects := state.Apply(clock, Reloaded{Generation: 1, Targets: []target.Target{vm}, Selected: vm.ID})
	if countEffects[Check](effects) != 1 {
		t.Errorf("a reload must not leave the selection on readings from the old config, got %+v", effects)
	}
}

func TestProblemsSurviveNoticesComingAndGoing(t *testing.T) {
	vm := box("vm")
	state := New([]target.Target{vm}, errors.New("target \"x\": broken")).Select(vm.ID)
	if got := state.Notice(); got != `config: target "x": broken` {
		t.Fatalf("a problem shows when nothing else does, got %q", got)
	}
	state, _ = state.Apply(clock, NoticeSet{Text: "hello"})
	if got := state.Notice(); got != "hello" {
		t.Errorf("a notice goes first, got %q", got)
	}
	state, _ = state.Apply(clock, ConnectionEnded{})
	if got := state.Notice(); got != `config: target "x": broken` {
		t.Errorf("clearing a notice must not lose the problem, got %q", got)
	}
}
