package launcher

import (
	"errors"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

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

func TestMissingSectionsKeepWhatWasKnown(t *testing.T) {
	vm := box("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: target.Environment{
		Readings: []target.Reading{{}},
		Tools:    []string{"tmux"},
		Sessions: []target.Session{{Name: "s"}},
	}})
	later := clock.Add(time.Minute)
	state, _ = state.Apply(later, Inspected{ID: vm.ID, Env: target.Environment{
		Tools:    []string{"zellij"},
		Sessions: []target.Session{{Name: "new"}},
		Missing:  target.MissingSections{Stats: true},
	}})
	got := state.Observation(vm.ID)
	if got.Stats.Current() || !got.Stats.Known || len(got.Stats.Value) != 1 || got.Stats.At != later {
		t.Errorf("stats keep their last value, marked not current, got %+v", got.Stats)
	}
	if !got.Tools.Current() || got.Tools.Value[0] != "zellij" || got.Sessions.Value[0].Name != "new" {
		t.Errorf("finished sections land, got %+v", got)
	}
}

func TestToolsThatNeverFinishedStayUnknown(t *testing.T) {
	vm := box("vm")
	vm.Modes = []target.Mode{{Name: "tmux", Argv: []string{"sh"}, Requires: "tmux"}, {Name: "shell", Argv: []string{"sh"}}}
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: target.Environment{Missing: target.MissingSections{Tools: true}}})
	if got := state.Mode().Name; got != "tmux" {
		t.Errorf("not finishing is not the same as tmux being missing, got %q", got)
	}
}
