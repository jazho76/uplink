package launcher

import (
	"fmt"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

type Operation string

const (
	OperationNone     Operation = ""
	OperationStarting Operation = "starting"
	OperationStopping Operation = "stopping"
)

func powerCommand(t target.Target, operation Operation) (command string, settled target.Status) {
	if operation == OperationStarting {
		return t.StartCommand(), target.StatusUp
	}
	return t.StopCommand(), target.StatusDown
}

func (s State) CanPower(t target.Target, operation Operation) bool {
	command, settled := powerCommand(t, operation)
	observation := s.Observation(t.ID)
	return command != "" && observation.Operation == OperationNone && observation.Status != settled
}

func (s *State) requestPower(operation Operation) []Effect {
	t, ok := s.selectedTarget()
	if !ok || !s.CanPower(t, operation) {
		return nil
	}
	current := s.entries[t.ID]
	current.Operation = operation
	s.entries[t.ID] = current
	s.notice = ""
	return []Effect{Power{Target: t, Operation: operation}}
}

func (s *State) powerDone(now time.Time, e PowerDone) []Effect {
	started := s.connection == Connection{Target: e.ID, Phase: PhaseStarting}
	connect := started && e.Operation == OperationStarting && e.Err == nil
	if started {
		s.connection = Connection{}
	}
	current := s.entries[e.ID]
	current.Operation = OperationNone
	current.epoch++
	s.entries[e.ID] = current
	s.markStale(e.ID)
	if e.Err != nil {
		s.notice = fmt.Sprintf("%s %s failed: %v", verb(e.Operation), e.ID, e.Err)
	}
	effects := s.recheck(now, e.ID)
	if connect {
		effects = append(effects, s.enter(e.ID)...)
	}
	return effects
}

func (s *State) recheck(now time.Time, id target.ID) []Effect {
	if id.Source != target.ConfigSource {
		return s.rediscover(now, id.Source)
	}
	t, ok := s.targetByID(id)
	if !ok {
		return nil
	}
	return []Effect{s.check(t, t.ID == s.selected)}
}

func verb(operation Operation) string {
	if operation == OperationStarting {
		return "start"
	}
	return "stop"
}
