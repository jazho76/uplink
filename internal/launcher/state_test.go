package launcher

import (
	"errors"
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
