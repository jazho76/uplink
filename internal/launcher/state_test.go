package launcher

import (
	"errors"
	"strings"
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

func countEffects[E Effect](effects []Effect) int {
	count := 0
	for _, effect := range effects {
		if _, ok := effect.(E); ok {
			count++
		}
	}
	return count
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
