package launcher

import (
	"cmp"
	"errors"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const (
	LiveInterval   = 5 * time.Second
	watchInterval  = 30 * time.Second
	maxWatchChecks = 4
)

var (
	errDown  = errors.New("target is not up")
	errStale = errors.New("stats changed")

	errIncomplete = errors.New("inspection did not finish")
)

type Observation struct {
	Status    target.Status
	Reason    string
	Operation Operation
	Stats     target.Fact[[]target.Reading]
	Tools     target.Fact[[]string]
	Sessions  target.Fact[[]target.Session]
}

func (o Observation) Label() string {
	return cmp.Or(string(o.Operation), string(cmp.Or(o.Status, target.StatusUnknown)))
}

func (s State) Observation(id target.ID) Observation {
	observation := s.entries[id].Observation
	observation.Status = cmp.Or(observation.Status, target.StatusUnknown)
	return observation
}

func (s *State) refreshSelected(now time.Time) []Effect {
	return append(s.checkSelectedIfStale(now), s.peekSelectedIfStale(now)...)
}

func (s *State) check(t target.Target, full bool) Effect {
	current := s.entries[t.ID]
	current.checking = true
	s.entries[t.ID] = current
	return Check{Target: t, Full: full, Epoch: current.epoch}
}

func (s *State) checkSelectedIfStale(now time.Time) []Effect {
	if now.Sub(s.entries[s.selected].Stats.At) < LiveInterval {
		return nil
	}
	return s.checkSelected()
}

func (s *State) checkSelected() []Effect {
	t, ok := s.selectedTarget()
	if !ok || s.entries[t.ID].checking {
		return nil
	}
	if !t.Discovered() {
		return []Effect{s.check(t, true)}
	}
	if s.Observation(t.ID).Status != target.StatusUp {
		return nil
	}
	return s.inspect(t)
}

func (s *State) inspect(t target.Target) []Effect {
	current := s.entries[t.ID]
	current.checking = true
	s.entries[t.ID] = current
	return []Effect{Inspect{Target: t, Epoch: current.epoch}}
}

func (s *State) checkWatched(now time.Time) []Effect {
	inFlight := 0
	for _, current := range s.entries {
		if current.checking {
			inFlight++
		}
	}

	var effects []Effect
	for _, t := range s.targets {
		if inFlight >= maxWatchChecks {
			break
		}
		current := s.entries[t.ID]
		if !t.Watch || t.Discovered() || t.ID == s.selected || current.checking || now.Sub(current.checkedAt) < watchInterval {
			continue
		}
		effects = append(effects, s.check(t, false))
		inFlight++
	}
	return effects
}

func (s *State) checked(now time.Time, e Checked) []Effect {
	accepted := s.settle(now, e)
	if e.ID != s.selected {
		return nil
	}
	var effects []Effect
	if t, ok := s.targetByID(e.ID); ok && accepted && e.Full && e.Status == target.StatusUp {
		effects = s.inspect(t)
	}
	if accepted {
		effects = append(effects, s.resolveConnection(e.ID)...)
	}
	return append(effects, s.peekSelectedIfStale(now)...)
}

func (s *State) settle(now time.Time, e Checked) bool {
	current := s.entries[e.ID]
	current.checking = false
	if e.Epoch != current.epoch {
		s.entries[e.ID] = current
		return false
	}
	current.checkedAt = now
	current.Status, current.Reason = e.Status, reasonFor(e.Status, e.Reason)
	switch {
	case e.Status == target.StatusUp:
	case e.Full:
		current.failInspection(errDown, now)
	default:
		current.failInspection(errDown, current.Stats.At)
	}
	s.entries[e.ID] = current
	return true
}

func reasonFor(status target.Status, reason string) string {
	if status == target.StatusUnknown {
		return shell.Plain(reason)
	}
	return ""
}

func (e *entry) failInspection(err error, at time.Time) {
	e.Stats.Fail(err, at)
	e.Tools.Fail(err, at)
	e.Sessions.Fail(err, at)
}

func (s *State) markStale(id target.ID) {
	current, ok := s.entries[id]
	if !ok {
		return
	}
	current.Stats.At, current.Tools.At, current.Sessions.At = time.Time{}, time.Time{}, time.Time{}
	s.entries[id] = current
}

func (s *State) observeReported(now time.Time, d inventory.Discovered) {
	current := s.entries[d.Target.ID]
	wasUp := current.Status == target.StatusUp
	current.Status, current.Reason = d.Status, reasonFor(d.Status, d.Reason)
	switch {
	case d.Status != target.StatusUp:
		current.failInspection(errDown, now)
	case !wasUp:
		current.Stats.At, current.Tools.At, current.Sessions.At = time.Time{}, time.Time{}, time.Time{}
	}
	s.entries[d.Target.ID] = current
}

func (s *State) inspectionDone(now time.Time, e Inspected) []Effect {
	s.inspected(now, e)
	if e.ID == s.selected {
		return s.peekSelectedIfStale(now)
	}
	return nil
}

func (s *State) inspected(now time.Time, e Inspected) {
	current := s.entries[e.ID]
	current.checking = false
	if e.Epoch == current.epoch && current.Status == target.StatusUp {
		if e.Err != nil {
			current.failInspection(e.Err, now)
		} else {
			record(&current.Stats, e.Env.Readings, e.Env.Missing.Stats, now)
			record(&current.Tools, e.Env.Tools, e.Env.Missing.Tools, now)
			record(&current.Sessions, e.Env.Sessions, e.Env.Missing.Sessions, now)
		}
	}
	s.entries[e.ID] = current
}

func record[T any](fact *target.Fact[T], value T, missing bool, now time.Time) {
	if missing {
		fact.Fail(errIncomplete, now)
		return
	}
	fact.Observe(value, now)
}
