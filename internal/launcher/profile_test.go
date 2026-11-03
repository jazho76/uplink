package launcher

import (
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

func detectedStats(name string) target.Target {
	vm := box(name)
	vm.Metrics = target.DetectedMetrics()
	return vm
}

func linuxReported(readings ...target.Reading) target.Environment {
	return target.Environment{Profile: target.Profile{OS: "Linux"}, Readings: readings}
}

func upAndSelected(t *testing.T, vm target.Target) State {
	t.Helper()
	state, effects := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	if countEffects[Inspect](effects) != 1 {
		t.Fatalf("up inspects once, got %+v", effects)
	}
	return state
}

func TestFirstPassLearnsTheProfileThenRunsTheMetrics(t *testing.T) {
	vm := detectedStats("vm")
	state := upAndSelected(t, vm)

	state, effects := state.Apply(clock, Inspected{ID: vm.ID, Env: linuxReported(target.Reading{})})
	got := state.Observation(vm.ID)
	if !got.Profile.Known || got.Stats.Known || !got.Stats.At.IsZero() {
		t.Errorf("the first pass ran no metrics, got %+v", got)
	}
	if countEffects[Inspect](effects) != 1 {
		t.Fatalf("want the second pass at once, got %+v", effects)
	}
	for _, effect := range effects {
		if inspect, ok := effect.(Inspect); ok && len(inspect.Target.Metrics) == 0 {
			t.Errorf("the second pass must carry the metrics, got %+v", inspect.Target.Metrics)
		}
	}

	state, effects = state.Apply(clock, Inspected{ID: vm.ID, Env: linuxReported(target.Reading{})})
	if !state.Observation(vm.ID).Stats.Known || countEffects[Inspect](effects) != 0 {
		t.Errorf("the second pass settles it, got %+v and %+v", state.Observation(vm.ID), effects)
	}
}

func TestDeclaredMetricsNeedNoSecondPass(t *testing.T) {
	vm := box("vm")
	vm.Metrics = []target.Metric{{Type: target.TypeText, Label: "mine", Value: "x"}}
	state, effects := upAndSelected(t, vm).Apply(clock, Inspected{ID: vm.ID, Env: linuxReported(target.Reading{})})
	if !state.Observation(vm.ID).Stats.Known || countEffects[Inspect](effects) != 0 {
		t.Errorf("declared metrics report on the first pass, got %+v and %+v", state.Observation(vm.ID), effects)
	}
}

func TestUnknownKernelSettlesWithoutLooping(t *testing.T) {
	vm := detectedStats("vm")
	plan9 := target.Environment{Profile: target.Profile{OS: "Plan9"}}
	state, effects := upAndSelected(t, vm).Apply(clock, Inspected{ID: vm.ID, Env: plan9})
	if countEffects[Inspect](effects) != 1 {
		t.Fatalf("want one more pass, got %+v", effects)
	}
	state, effects = state.Apply(clock, Inspected{ID: vm.ID, Env: plan9})
	got := state.Observation(vm.ID).Stats
	if !got.Known || len(got.Value) != 0 || countEffects[Inspect](effects) != 0 {
		t.Errorf("no metrics is an answer, got %+v and %+v", got, effects)
	}
}

func TestMissingProfileWaitsForTheNextTick(t *testing.T) {
	vm := detectedStats("vm")
	missing := target.Environment{Missing: target.MissingSections{Profile: true, Stats: true}}
	state, effects := upAndSelected(t, vm).Apply(clock, Inspected{ID: vm.ID, Env: missing})
	if countEffects[Inspect](effects) != 0 || state.Observation(vm.ID).Stats.Err != nil {
		t.Errorf("nothing was learned, got %+v and %+v", state.Observation(vm.ID), effects)
	}
	if _, effects := state.Apply(clock.Add(LiveInterval), Ticked{}); countEffects[Check](effects) != 1 {
		t.Errorf("the tick tries again, got %+v", effects)
	}
}

func TestKeptProfileNeedsNoSecondPassAfterARestart(t *testing.T) {
	vm := detectedStats("vm")
	state, _ := upAndSelected(t, vm).Apply(clock, Inspected{ID: vm.ID, Env: linuxReported()})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: linuxReported()})

	state, _ = state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown, Full: true})
	state, effects := state.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	if countEffects[Inspect](effects) != 1 {
		t.Fatalf("back up inspects, got %+v", effects)
	}
	state, effects = state.Apply(clock, Inspected{ID: vm.ID, Env: linuxReported(target.Reading{})})
	if len(state.Observation(vm.ID).Stats.Value) != 1 || countEffects[Inspect](effects) != 0 {
		t.Errorf("the profile is still known, got %+v and %+v", state.Observation(vm.ID), effects)
	}
}

func reloadedWith(t *testing.T, state State, changed target.Target) State {
	t.Helper()
	state, _ = state.Apply(clock, ReloadRequested{})
	next, _ := state.Apply(clock.Add(time.Minute), ConfigRead{Generation: 1, Plan: inventory.Plan{Declared: []target.Target{changed}}})
	return next
}

func TestReloadKeepsReadingsOfUnchangedDetectedStats(t *testing.T) {
	vm := detectedStats("vm")
	state, _ := upAndSelected(t, vm).Apply(clock, Inspected{ID: vm.ID, Env: linuxReported()})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: linuxReported(target.Reading{})})

	got := reloadedWith(t, state, vm).Observation(vm.ID).Stats
	if len(got.Value) != 1 || got.Err != nil {
		t.Errorf("the declaration did not change, got %+v", got)
	}
}

func TestReloadTurningDetectedStatsOffInvalidates(t *testing.T) {
	vm := detectedStats("vm")
	state, _ := upAndSelected(t, vm).Apply(clock, Inspected{ID: vm.ID, Env: linuxReported()})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: linuxReported(target.Reading{})})

	off := vm
	off.Metrics = []target.Metric{}
	if got := reloadedWith(t, state, off).Observation(vm.ID).Stats; got.Value != nil || got.Err == nil {
		t.Errorf("stats: [] is a change even though nil and empty compare equal, got %+v", got)
	}
}
