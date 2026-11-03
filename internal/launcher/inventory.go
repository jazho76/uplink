package launcher

import (
	"fmt"
	"slices"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
)

const intervalGrace = time.Second

type discovery struct {
	inFlight bool
	issuedAt time.Time
	epoch    int
}

func (s *State) started() []Effect {
	s.loading = true
	return s.requestConfig("")
}

func (s *State) requestConfig(notice string) []Effect {
	s.generation++
	return []Effect{ReadConfig{Generation: s.generation, Notice: notice}}
}

func (s *State) configRead(now time.Time, e ConfigRead) []Effect {
	if e.Generation != s.generation {
		return nil
	}
	s.inventory = s.inventory.WithPlan(e.Plan)
	s.invalidateChangedStats()
	s.adopt(now)
	s.modeName, s.notice = "", e.Notice
	s.markStale(s.selected)

	var effects []Effect
	for _, name := range s.inventory.Providers() {
		if !s.discoveries[name].inFlight {
			effects = append(effects, s.discover(now, name))
		}
	}
	effects = append(effects, s.refreshSelected(now)...)
	return append(effects, s.checkWatched(now)...)
}

func (s *State) invalidateChangedStats() {
	merged, _ := s.inventory.Merged()
	for _, d := range merged {
		t := d.Target
		previous, ok := s.targetByID(t.ID)
		if ok && (!slices.Equal(previous.Stats.Metrics, t.Stats.Metrics) || previous.Stats.Builtin != t.Stats.Builtin) {
			current := s.entries[t.ID]
			current.Stats.Value, current.Stats.Err = nil, errStale
			current.Sessions.Err = errStale
			s.entries[t.ID] = current
		}
	}
}

func (s *State) discoveryDone(now time.Time, e DiscoveryDone) []Effect {
	current := s.discoveries[e.Provider]
	if e.Epoch != current.epoch {
		return nil
	}
	current.inFlight = false
	s.discoveries[e.Provider] = current
	if !s.inventory.Plans(e.Provider) {
		return nil
	}
	s.inventory = s.inventory.WithResult(e.Provider, e.Found, e.Err)
	s.adopt(now)
	effects := s.refreshSelected(now)
	if s.connection.Target.Source == e.Provider {
		effects = append(effects, s.resolveConnection(s.connection.Target)...)
	}
	return effects
}

func (s *State) adopt(now time.Time) {
	merged, problems := s.inventory.Merged()
	s.targets, s.problems = inventory.Targets(merged), problems
	for _, d := range merged {
		if d.Target.Discovered() {
			s.observeReported(now, d)
		}
	}
	s.loading = s.loading && !s.inventory.Answered()
	if _, ok := s.targetByID(s.logTarget.ID); s.logOpen && !ok {
		s.logOpen = false
	}
	if _, ok := s.targetByID(s.connection.Target); s.connection.Cancellable() && !ok {
		s.notice = fmt.Sprintf("%s is gone", s.connection.Target)
		s.connection = Connection{}
	}
}

func (s *State) discover(now time.Time, name string) Effect {
	current := s.discoveries[name]
	current.inFlight, current.issuedAt = true, now
	s.discoveries[name] = current
	return Discover{Provider: name, Epoch: current.epoch}
}

func (s *State) rediscover(now time.Time, name string) []Effect {
	if !s.inventory.Plans(name) {
		return nil
	}
	current := s.discoveries[name]
	current.epoch++
	s.discoveries[name] = current
	return []Effect{s.discover(now, name)}
}

func (s *State) dueDiscoveries(now time.Time) []Effect {
	var effects []Effect
	for _, name := range s.inventory.Providers() {
		interval := watchInterval
		if s.selected.Source == name {
			interval = LiveInterval
		}
		current := s.discoveries[name]
		if !current.inFlight && now.Sub(current.issuedAt) >= interval-intervalGrace {
			effects = append(effects, s.discover(now, name))
		}
	}
	return effects
}
