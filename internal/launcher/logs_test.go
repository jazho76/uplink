package launcher

import (
	"errors"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

func withLogs(t target.Target) target.Target {
	t.Logs.Command = "tail"
	return t
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

func detectedVM(name string) target.Target {
	vm := box(name)
	vm.Logs = target.DetectedLogs()
	return vm
}

func journalReported() target.Environment {
	return target.Environment{Profile: target.Profile{LogSystem: target.LogSystemJournal}}
}

func TestDetectedLogsWaitForTheProfile(t *testing.T) {
	vm := detectedVM("vm")
	state, effects := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	if countEffects[PeekLogs](effects) != 0 || state.HasLogs(vm) {
		t.Fatal("nothing is known about its logs before inspection")
	}

	state, effects = state.Apply(clock, Inspected{ID: vm.ID, Env: journalReported()})
	if !state.HasLogs(vm) {
		t.Fatal("the reported log system should offer logs")
	}
	for _, effect := range effects {
		if peek, ok := effect.(PeekLogs); ok {
			if peek.Target.Logs.Command == "" {
				t.Errorf("the effect must carry the concrete command, got %+v", peek.Target.Logs)
			}
			return
		}
	}
	t.Error("the report should peek straight away")
}

func TestNoLogSystemMeansNoLogs(t *testing.T) {
	vm := detectedVM("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, effects := state.Apply(clock, Inspected{ID: vm.ID})
	if countEffects[PeekLogs](effects) != 0 || state.HasLogs(vm) {
		t.Error("an empty profile has nothing to read")
	}
}

func TestDetectedLogsFollowStatusWithoutRedetecting(t *testing.T) {
	vm := detectedVM("vm")
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp, Full: true})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: journalReported()})

	for _, status := range []target.Status{target.StatusDown, target.StatusUnknown} {
		away, _ := state.Apply(clock, Checked{ID: vm.ID, Status: status})
		if away.HasLogs(vm) {
			t.Errorf("%s: logs inside the target need it up", status)
		}
		back, _ := away.Apply(clock, Checked{ID: vm.ID, Status: target.StatusUp})
		if !back.HasLogs(vm) {
			t.Errorf("back from %s: the profile should still be known", status)
		}
	}
}

func TestHostLogsDoNotNeedTheTargetUp(t *testing.T) {
	vm := withLogs(box("vm"))
	state, _ := selected(vm).Apply(clock, Checked{ID: vm.ID, Status: target.StatusDown})
	if !state.HasLogs(vm) {
		t.Error("host logs are readable while the target is down")
	}
}

func TestProfileSurvivesRefreshesAndFailures(t *testing.T) {
	vm := limaVM("vm")
	vm.Logs = target.DetectedLogs()
	state := startedWith(t, "lima").Select(vm.ID)
	found := discoveredOf(vm, target.StatusUp)
	found[0].Target.Logs = vm.Logs
	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: found})
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: journalReported()})

	state, _ = state.Apply(clock, DiscoveryDone{Provider: "lima", Found: found})
	if !state.HasLogs(vm) {
		t.Error("a discovery refresh replaces the declaration, not the facts")
	}
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Err: errors.New("boom")})
	if !state.HasLogs(vm) {
		t.Error("a failed inspection keeps what was known")
	}
	state, _ = state.Apply(clock, Inspected{ID: vm.ID, Env: target.Environment{Missing: target.MissingSections{Profile: true}}})
	if !state.HasLogs(vm) {
		t.Error("an unfinished section keeps what was known")
	}
}

func TestLogsStayShutWhileConnecting(t *testing.T) {
	_, state := startableVM()
	state, _ = state.Apply(clock, ConnectRequested{})
	if next, effects := state.Apply(clock, LogsOpened{}); next.LogsOpen() || len(effects) != 0 {
		t.Error("a session must not launch from the log screen")
	}
}
