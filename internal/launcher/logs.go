package launcher

import (
	"time"

	"github.com/jazho76/uplink/internal/target"
)

const (
	LogInterval    = time.Second
	maxLogPeek     = 300
	logScreenLines = 500
)

type logPeek struct {
	id   target.ID
	text string
	at   time.Time
}

func (s State) LogsOpen() bool { return s.logOpen }

func (s State) LogTarget() target.Target { return s.logTarget }

func (s State) LogText() string { return s.logText }

func (s *State) peekSelectedIfStale(now time.Time) []Effect {
	if s.peek.id == s.selected && now.Sub(s.peek.at) < LiveInterval {
		return nil
	}
	return s.peekSelected()
}

func (s *State) peekSelected() []Effect {
	t, ok := s.selectedTarget()
	if !ok || !s.HasLogs(t) || s.logOpen || s.entries[t.ID].peeking {
		return nil
	}
	current := s.entries[t.ID]
	current.peeking = true
	s.entries[t.ID] = current
	return []Effect{PeekLogs{Target: t, Lines: maxLogPeek}}
}

func (s State) HasLogs(t target.Target) bool {
	logs := s.resolved(t).Logs
	return logs.Command != "" && (logs.Where == target.InHost || s.Observation(t.ID).Status == target.StatusUp)
}

func (s State) Logs(t target.Target) string {
	if s.peek.id != t.ID || !s.HasLogs(t) {
		return ""
	}
	return s.peek.text
}

func (s *State) openLogs() []Effect {
	t, ok := s.selectedTarget()
	if !ok || !s.HasLogs(t) || s.connection.Phase != PhaseNone {
		return nil
	}
	s.logText = s.Logs(t)
	s.logOpen, s.logTarget, s.notice = true, t, ""
	s.logRound++
	return []Effect{ViewLogs{Target: t, Round: s.logRound, Lines: logScreenLines}}
}

func (s *State) logTicked(e LogTicked) []Effect {
	if s.logOpen && e.Round == s.logRound {
		return []Effect{ViewLogs{Target: s.logTarget, Round: s.logRound, Lines: logScreenLines}}
	}
	return nil
}

func (s *State) logsPeeked(now time.Time, e LogsPeeked) {
	current := s.entries[e.ID]
	current.peeking = false
	s.entries[e.ID] = current
	if e.ID == s.selected {
		s.peek = logPeek{id: e.ID, text: e.Text, at: now}
	}
}

func (s *State) logsViewed(e LogsViewed) []Effect {
	if s.logOpen && e.Round == s.logRound {
		s.logText = e.Text
		return []Effect{ScheduleLogTick{Round: s.logRound}}
	}
	return nil
}
