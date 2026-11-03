package launcher

import (
	"fmt"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

type ConnectionPhase string

const (
	PhaseNone           ConnectionPhase = ""
	PhaseAwaitingStatus ConnectionPhase = "awaiting status"
	PhaseStarting       ConnectionPhase = "starting"
	PhaseEntering       ConnectionPhase = "entering"
)

type Connection struct {
	Target target.ID
	Phase  ConnectionPhase
}

func (c Connection) Cancellable() bool {
	return c.Phase == PhaseAwaitingStatus || c.Phase == PhaseStarting
}

func (s State) Connection() Connection { return s.connection }

func (s *State) requestConnection(now time.Time) []Effect {
	t, ok := s.selectedTarget()
	if !ok || s.connection.Phase != PhaseNone {
		return nil
	}
	observation := s.Observation(t.ID)
	startable := t.StartCommand() != ""
	switch {
	case observation.Operation != OperationNone:
		return nil
	case observation.Status == target.StatusDown && startable:
		return s.start(t.ID)
	case observation.Status == target.StatusUnknown && startable:
		s.connection = Connection{Target: t.ID, Phase: PhaseAwaitingStatus}
		return s.askStatus(now, t)
	}
	return s.enter(t.ID)
}

func (s *State) start(id target.ID) []Effect {
	s.connection = Connection{Target: id, Phase: PhaseStarting}
	return s.requestPower(OperationStarting)
}

func (s *State) askStatus(now time.Time, t target.Target) []Effect {
	if t.Discovered() {
		if !s.inventory.Plans(t.ID.Source) || s.discoveries[t.ID.Source].inFlight {
			return nil
		}
		return []Effect{s.discover(now, t.ID.Source)}
	}
	if s.entries[t.ID].checking {
		return nil
	}
	return []Effect{s.check(t, true)}
}

func (s *State) resolveConnection(id target.ID) []Effect {
	if s.connection != (Connection{Target: id, Phase: PhaseAwaitingStatus}) {
		return nil
	}
	t, ok := s.targetByID(id)
	if ok && s.Observation(id).Status == target.StatusDown && t.StartCommand() != "" {
		return s.start(id)
	}
	return s.enter(id)
}

func (s *State) enter(id target.ID) []Effect {
	mode := s.Mode()
	t, ok := s.targetByID(id)
	if !ok || len(mode.Argv) == 0 {
		s.connection = Connection{}
		return nil
	}
	s.connection = Connection{Target: id, Phase: PhaseEntering}
	return []Effect{Connect{Name: t.Name, Argv: mode.Argv}}
}

func (s *State) cancelConnection() {
	if s.connection.Cancellable() {
		s.connection = Connection{}
	}
}

func (s *State) connectionEnded(now time.Time, e ConnectionEnded) []Effect {
	if s.connection.Phase == PhaseEntering {
		s.connection = Connection{}
	}
	s.notice = connectionNotice(e)
	s.markStale(s.selected)
	return append(s.rediscover(now, s.selected.Source), s.refreshSelected(now)...)
}

func connectionNotice(e ConnectionEnded) string {
	switch {
	case e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Name, e.Err)
	case e.ExitCode != 0:
		return fmt.Sprintf("%s: session ended with exit %d", e.Name, e.ExitCode)
	default:
		return ""
	}
}
