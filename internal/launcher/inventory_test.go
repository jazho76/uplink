package launcher

import (
	"errors"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

func limaVM(name string) target.Target {
	t := box(name)
	t.ID.Source = "lima"
	return t
}

func discoveredOf(t target.Target, status target.Status) []inventory.Discovered {
	return []inventory.Discovered{{Target: target.Target{ID: target.ID{Key: t.ID.Key}, Name: t.Name, Metrics: t.Metrics, Modes: t.Modes, Power: t.Power}, Status: status}}
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
	changed.Metrics = target.DetectedMetrics()
	next, _ := state.Apply(clock.Add(time.Minute), ConfigRead{Generation: 1, Plan: inventory.Plan{Declared: []target.Target{changed}}})
	got := next.Observation(vm.ID)
	if got.Stats.Value != nil || got.Stats.Current() || got.Sessions.Current() {
		t.Errorf("readings of the old stats are gone, got %+v", got)
	}
	if !got.Stats.Known {
		t.Error("clearing readings does not forget that inspection happened")
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

func TestFailedDiscoveryExplainsTheUnknown(t *testing.T) {
	vm := limaVM("vm")
	state := startedWith(t, "lima").Select(vm.ID)
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: discoveredOf(vm, target.StatusUp)})
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Err: errors.New("limactl broke")})
	if got := state.Observation(vm.ID); got.Status != target.StatusUnknown || got.Reason != "limactl broke" {
		t.Errorf("got %+v", got)
	}
}
