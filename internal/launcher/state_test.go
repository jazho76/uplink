package launcher

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
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

func TestFactsFollowEachResult(t *testing.T) {
	vm := box("vm")
	boom := errors.New("boom")
	env := target.Environment{Tools: []string{"tmux"}}
	later := clock.Add(time.Minute)
	observed, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	observed, _ = observed.Apply(clock, Inspected{ID: vm.ID, Env: env})

	for name, c := range map[string]struct {
		event     Event
		current   bool
		at        time.Time
		wantState target.Status
	}{
		"inspected":         {Inspected{ID: vm.ID, Env: env}, true, later, target.StatusUp},
		"inspection failed": {Inspected{ID: vm.ID, Err: boom}, false, later, target.StatusUp},
		"down full":         {Checked{ID: vm.ID, Full: true, Status: target.StatusDown}, false, later, target.StatusDown},
		"down not full":     {Checked{ID: vm.ID, Status: target.StatusDown}, false, clock, target.StatusDown},
		"up":                {Checked{ID: vm.ID, Status: target.StatusUp}, true, clock, target.StatusUp},
		"up full":           {Checked{ID: vm.ID, Status: target.StatusUp, Full: true}, true, clock, target.StatusUp},
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

func TestStatusArrivesBeforeInspection(t *testing.T) {
	vm := box("vm")
	state, effects := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	if state.Observation(vm.ID).Status != target.StatusUp {
		t.Error("status is known at once")
	}
	if countEffects[Inspect](effects) != 1 {
		t.Errorf("an up target is inspected next, got %+v", effects)
	}
	if _, effects := selected(vm).Apply(clock, Checked{ID: vm.ID, Full: true, Status: target.StatusDown}); countEffects[Inspect](effects) != 0 {
		t.Errorf("a down target is not inspected, got %+v", effects)
	}
	other := box("other")
	moved := selected(other, vm)
	if _, effects := moved.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true}); countEffects[Inspect](effects) != 0 {
		t.Errorf("only the selected target is inspected, got %+v", effects)
	}
}

func TestOldCheckIsDroppedButStillPeeksLogs(t *testing.T) {
	vm := box("vm")
	vm.Logs.Command = "tail"
	state := selected(vm)
	current := state.entries[vm.ID]
	current.epoch = 1
	state.entries[vm.ID] = current

	next, effects := state.Apply(clock, Checked{ID: vm.ID, Epoch: 0, Status: target.StatusUp, Full: true})
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

func TestOlderConfigReadIsIgnored(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, ReloadRequested{})
	state, _ = state.Apply(clock, ReloadRequested{})
	next, effects := state.Apply(clock, ConfigRead{Generation: 1})
	if len(next.Targets()) != 1 || effects != nil {
		t.Errorf("an older read must not win, got %d targets and %+v", len(next.Targets()), effects)
	}
}

func TestReloadWithChangedStatsInvalidatesThem(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: target.Environment{Readings: []target.Reading{{}}, Sessions: []target.Session{{Name: "s"}}}})
	state, _ = state.Apply(clock, ReloadRequested{})

	changed := vm
	changed.Stats.Builtin = true
	next, _ := state.Apply(clock.Add(time.Minute), ConfigRead{Generation: 1, Plan: inventory.Plan{Declared: []target.Target{changed}}})
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

func TestProblemsNoticeSummarizes(t *testing.T) {
	one := errors.New(`target "box": no modes`)
	nested := errors.Join(errors.Join(one, errors.New("b")), errors.New("c"))
	for err, want := range map[error]string{
		nil:    "",
		one:    `target "box": no modes`,
		nested: "3 problems, see uplink config check",
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

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Full: true, Status: target.StatusDown})
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

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID})
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
	state, effects := selected(vm, watched, unwatched).Apply(clock, Ticked{})

	var background []target.ID
	for _, effect := range effects {
		if check := effect.(Check); !check.Full {
			background = append(background, check.Target.ID)
		}
	}
	if len(background) != 1 || background[0] != watched.ID {
		t.Fatalf("only the watched target is checked in the background, got %v", background)
	}

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID})
	state, _ = state.Apply(clock, Checked{ID: watched.ID, Status: target.StatusDown})
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
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID})
	state, _ = state.Apply(clock, ReloadRequested{})
	_, effects := state.Apply(clock, ConfigRead{Generation: 1, Plan: inventory.Plan{Declared: []target.Target{vm}}})
	if countEffects[Check](effects) != 1 {
		t.Errorf("a reload must not leave the selection on readings from the old config, got %+v", effects)
	}
}

func TestProblemsSurviveNoticesComingAndGoing(t *testing.T) {
	vm := box("vm")
	state := New([]target.Target{vm}, errors.New("target \"x\": broken")).Select(vm.ID)
	if got := state.Notice(); got != `target "x": broken` {
		t.Fatalf("a problem shows when nothing else does, got %q", got)
	}
	state, _ = state.Apply(clock, NoticeSet{Text: "hello"})
	if got := state.Notice(); got != "hello" {
		t.Errorf("a notice goes first, got %q", got)
	}
	state, _ = state.Apply(clock, ConnectionEnded{})
	if got := state.Notice(); got != `target "x": broken` {
		t.Errorf("clearing a notice must not lose the problem, got %q", got)
	}
}

func limaVM(name string) target.Target {
	t := box(name)
	t.ID.Source = "lima"
	return t
}

func discoveredOf(t target.Target, status target.Status) []inventory.Discovered {
	return []inventory.Discovered{{Target: target.Target{ID: target.ID{Key: t.ID.Key}, Name: t.Name, Modes: t.Modes, Power: t.Power}, Status: status}}
}

func startedWith(t *testing.T, providers ...string) State {
	t.Helper()
	state, effects := New(nil, nil).Apply(clock, Started{})
	if !state.Loading() {
		t.Error("nothing has answered yet")
	}
	read, ok := effects[0].(ReadConfig)
	if !ok {
		t.Fatalf("startup reads the config, got %+v", effects)
	}
	state, effects = state.Apply(clock, ConfigRead{Generation: read.Generation, Plan: inventory.Plan{Providers: providers}})
	if got := countEffects[Discover](effects); got != len(providers) {
		t.Errorf("want a discovery per provider, got %+v", effects)
	}
	return state
}

func TestStartupReadsTheConfigThenDiscovers(t *testing.T) {
	state := startedWith(t, "lima", "docker")
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(limaVM("vm"), target.StatusUp)})
	if !state.Loading() || len(state.Targets()) != 1 {
		t.Errorf("docker has not answered, and lima's targets are already here, got %v", state.Targets())
	}
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "docker", Err: inventory.ErrUnavailable})
	if state.Loading() {
		t.Error("everyone has answered")
	}
	if got := state.Observation(limaVM("vm").ID).Status; got != target.StatusUp {
		t.Errorf("status comes from the provider, got %q", got)
	}
}

func TestDiscoveredTargetIsInspectedOnlyWhileUp(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima").Select(vm.ID)

	state, effects := state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusDown)})
	if countEffects[Inspect](effects)+countEffects[Check](effects) != 0 {
		t.Errorf("a stopped target is not inspected and has no status call, got %+v", effects)
	}
	if got := state.Observation(vm.ID).Stats; got.At.IsZero() || got.Err == nil {
		t.Errorf("the preview must not wait forever, got %+v", got)
	}

	later := clock.Add(time.Second)
	state, effects = state.Apply(later, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	if countEffects[Inspect](effects) != 1 {
		t.Errorf("coming up inspects at once, got %+v", effects)
	}
	if !state.Observation(vm.ID).Stats.At.IsZero() {
		t.Error("the preview shows it is waiting")
	}
}

func TestFailedInspectionLeavesAReportedTargetUp(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima").Select(vm.ID)
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Err: errors.New("permission denied")})
	got := state.Observation(vm.ID)
	if got.Status != target.StatusUp || got.Stats.Err == nil {
		t.Errorf("up means the owner says so, got %+v", got)
	}
}

func TestLateInspectionCannotOverwriteAStop(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima").Select(vm.ID)
	state, effects := state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	if countEffects[Inspect](effects) != 1 {
		t.Fatal("it should be inspected")
	}
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusDown)})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: target.Environment{Tools: []string{"tmux"}}})
	if got := state.Observation(vm.ID); got.Tools.Current() {
		t.Errorf("a late result must not land, got %+v", got)
	}
	if _, effects := state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)}); countEffects[Inspect](effects) != 1 {
		t.Errorf("the flag must not stick, got %+v", effects)
	}
}

func TestSelectedProviderIsDiscoveredOftenAndOthersRarely(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima", "docker").Select(vm.ID)
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusDown)})
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "docker"})

	_, effects := state.Apply(clock.Add(LiveInterval), Ticked{})
	if len(effects) != 1 || effects[0].(Discover).Provider != "lima" {
		t.Errorf("only the selected target's provider is due, got %+v", effects)
	}
	_, effects = state.Apply(clock.Add(watchInterval), Ticked{})
	if countEffects[Discover](effects) != 2 {
		t.Errorf("both are due, got %+v", effects)
	}
	busy, _ := state.Apply(clock.Add(LiveInterval), Ticked{})
	if _, effects := busy.Apply(clock.Add(2*LiveInterval), Ticked{}); countEffects[Discover](effects) != 0 {
		t.Errorf("one in flight is enough, got %+v", effects)
	}
}

func TestPowerRediscoversAndDropsOlderResults(t *testing.T) {
	vm := limaVM("vm")
	vm.Power = &target.Power{Up: "start"}
	state := startedWith(t, "lima").Select(vm.ID)
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusDown)})

	older, _ := state.Apply(clock.Add(LiveInterval), Ticked{})
	state, _ = older.Apply(clock, PowerRequested{Operation: OperationStarting})
	state, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStarting})
	if countEffects[Discover](effects) != 1 || countEffects[Check](effects) != 0 {
		t.Fatalf("the provider is asked again, got %+v", effects)
	}
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Epoch: 0, Found: discoveredOf(vm, target.StatusDown)})
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Epoch: 1, Found: discoveredOf(vm, target.StatusUp)})
	if got := state.Observation(vm.ID).Status; got != target.StatusUp {
		t.Errorf("a result from before the start must not win, got %q", got)
	}
}

func TestPowerOnAVanishedTargetStillRediscovers(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima")
	_, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStopping})
	if countEffects[Discover](effects) != 1 {
		t.Errorf("got %+v", effects)
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

func TestProviderDroppedFromThePlanStaysGone(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima")
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	state, _ = state.Apply(clock, ReloadRequested{})
	state, _ = state.Apply(clock, ConfigRead{Generation: state.generation})
	if len(state.Targets()) != 0 {
		t.Fatalf("got %v", state.Targets())
	}
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	if len(state.Targets()) != 0 {
		t.Errorf("a late result must not bring it back, got %v", state.Targets())
	}
	if _, effects := state.Apply(clock, PowerDone{ID: vm.ID, Operation: OperationStopping}); countEffects[Discover](effects) != 0 {
		t.Errorf("nor may a late power result, got %+v", effects)
	}
}

func TestLogScreenClosesWhenItsTargetVanishes(t *testing.T) {
	vm := limaVM("vm")
	vm.Logs.Command = "tail"
	state := startedWith(t, "lima").Select(vm.ID)
	found := discoveredOf(vm, target.StatusUp)
	found[0].Target.Logs = vm.Logs
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: found})
	state, _ = state.Apply(clock, LogsOpened{})
	if !state.LogsOpen() {
		t.Fatal("it should open")
	}
	state, _ = state.Apply(clock.Add(LiveInterval), Ticked{})
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima"})
	if state.LogsOpen() {
		t.Error("there is nothing left to show")
	}
}

func TestProviderFailureDoesNotTakeOverTheNotice(t *testing.T) {
	state := startedWith(t, "lima")
	state, _ = state.Apply(clock, NoticeSet{Text: "session ended"})
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Err: errors.New("timed out")})
	if got := state.Notice(); got != "session ended" {
		t.Errorf("got %q", got)
	}
	state, _ = state.Apply(clock, ConnectionEnded{})
	if got := state.Notice(); !strings.Contains(got, "timed out") {
		t.Errorf("the problem shows once the notice clears, got %q", got)
	}
}

func TestUnknownCarriesItsReasonUntilResolved(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUnknown, Reason: "Permission denied"})
	if got := state.Observation(vm.ID); got.Status != target.StatusUnknown || got.Reason != "Permission denied" {
		t.Errorf("got %+v", got)
	}
	if _, effects := state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUnknown, Full: true}); countEffects[Inspect](effects) != 0 {
		t.Errorf("only an up target is inspected, got %+v", effects)
	}
	state, _ = state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown, Reason: "exit 1"})
	if got := state.Observation(vm.ID).Reason; got != "" {
		t.Errorf("only unknown has a reason, got %q", got)
	}
}

func TestFailedDiscoveryExplainsTheUnknown(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima").Select(vm.ID)
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Err: errors.New("limactl broke")})
	if got := state.Observation(vm.ID); got.Status != target.StatusUnknown || got.Reason != "limactl broke" {
		t.Errorf("got %+v", got)
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

func startableVM() (target.Target, State) {
	vm := box("vm")
	vm.Power = &target.Power{Up: "start"}
	return vm, selected(vm, box("other"))
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

func TestLogsStayShutWhileConnecting(t *testing.T) {
	_, state := startableVM()
	state, _ = state.Apply(clock, ConnectRequested{})
	if next, effects := state.Apply(clock, LogsOpened{}); next.LogsOpen() || len(effects) != 0 {
		t.Error("a session must not launch from the log screen")
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

func TestNoticeIsOneCleanLine(t *testing.T) {
	state, _ := selected(box("vm")).Apply(clock, NoticeSet{Text: "first\n\x1b[31msecond\x1b[0m\n"})
	if got := state.Notice(); got != "first; second" {
		t.Errorf("got %q", got)
	}
	errs := New(nil, errors.New("line\x1b]0;x\x07")).Notice()
	if errs != "line" {
		t.Errorf("problems are cleaned too, got %q", errs)
	}
}
