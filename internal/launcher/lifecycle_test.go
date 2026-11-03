package launcher

import (
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

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
