package launcher

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

type entry struct {
	Observation
	checking  bool
	peeking   bool
	checkedAt time.Time
	epoch     int
}

type State struct {
	targets     []target.Target
	entries     map[target.ID]entry
	inventory   inventory.Inventory
	discoveries map[string]discovery
	loading     bool
	selected    target.ID
	modeName    string
	notice      string
	problems    error
	connection  Connection
	generation  int
	peek        logPeek
	logOpen     bool
	logRound    int
	logTarget   target.Target
	logText     string
}

func New(targets []target.Target, problems error) State {
	return State{targets: targets, entries: map[target.ID]entry{}, discoveries: map[string]discovery{}, problems: problems}
}

func (s State) Targets() []target.Target { return s.targets }

func (s State) Selection() target.ID { return s.selected }

func (s State) Notice() string {
	text := cmp.Or(s.notice, problemsNotice(s.problems))
	return shell.Plain(strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", "; "))
}

func (s State) Loading() bool { return s.loading }

func (s State) Select(id target.ID) State {
	s.selected = id
	return s
}

func (s State) WithTargets(targets []target.Target) State {
	s.targets = targets
	return s
}

func (s State) Apply(now time.Time, event Event) (State, []Effect) {
	s.entries = maps.Clone(s.entries)
	s.discoveries = maps.Clone(s.discoveries)
	effects := s.apply(now, event)
	return s, effects
}

func (s *State) apply(now time.Time, event Event) []Effect {
	switch e := event.(type) {
	case Started:
		s.loading = true
		return s.requestConfig("")

	case Ticked:
		effects := s.checkSelected()
		effects = append(effects, s.peekSelected()...)
		effects = append(effects, s.checkWatched(now)...)
		return append(effects, s.dueDiscoveries(now)...)

	case Selected:
		if s.connection.Target != e.ID {
			s.cancelConnection()
		}
		s.selected, s.modeName = e.ID, ""
		return s.refreshSelected(now)

	case ModeCycled:
		s.cycleMode(e.Delta)

	case PowerRequested:
		if s.connection.Phase == PhaseAwaitingStatus {
			s.connection = Connection{}
		}
		return s.requestPower(e.Operation)

	case PowerDone:
		return s.powerDone(now, e)

	case ConnectRequested:
		return s.requestConnection(now)

	case ConnectionCancelled:
		s.cancelConnection()

	case ConnectionEnded:
		if s.connection.Phase == PhaseEntering {
			s.connection = Connection{}
		}
		s.notice = connectionNotice(e)
		s.markStale(s.selected)
		return append(s.rediscover(now, s.selected.Source), s.refreshSelected(now)...)

	case LogsOpened:
		return s.openLogs()

	case LogsClosed:
		s.logOpen = false

	case LogTicked:
		if s.logOpen && e.Round == s.logRound {
			return []Effect{ViewLogs{Target: s.logTarget, Round: s.logRound}}
		}

	case LogsPeeked:
		current := s.entries[e.ID]
		current.peeking = false
		s.entries[e.ID] = current
		if e.ID == s.selected {
			s.peek = logPeek{id: e.ID, text: e.Text, at: now}
		}

	case LogsViewed:
		if s.logOpen && e.Round == s.logRound {
			s.logText = e.Text
			return []Effect{ScheduleLogTick{Round: s.logRound}}
		}

	case Checked:
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

	case Inspected:
		s.inspected(now, e)
		if e.ID == s.selected {
			return s.peekSelectedIfStale(now)
		}

	case ReloadRequested:
		return s.requestConfig(e.Notice)

	case ConfigRead:
		return s.configRead(now, e)

	case DiscoveryDone:
		return s.discoveryDone(now, e)

	case NoticeSet:
		s.notice = e.Text
	}
	return nil
}

func (s State) targetByID(id target.ID) (target.Target, bool) {
	i := slices.IndexFunc(s.targets, func(t target.Target) bool { return t.ID == id })
	if i < 0 {
		return target.Target{}, false
	}
	return s.targets[i], true
}

func (s State) selectedTarget() (target.Target, bool) {
	return s.targetByID(s.selected)
}

func problemsNotice(problems error) string {
	switch count := countProblems(problems); count {
	case 0:
		return ""
	case 1:
		return problems.Error()
	default:
		return fmt.Sprintf("%d problems, see uplink config check", count)
	}
}

func countProblems(err error) int {
	if err == nil {
		return 0
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return 1
	}
	count := 0
	for _, inner := range joined.Unwrap() {
		count += countProblems(inner)
	}
	return count
}
