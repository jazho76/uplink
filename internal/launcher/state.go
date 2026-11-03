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
		return s.started()
	case Ticked:
		return s.ticked(now)
	case Selected:
		return s.selectTarget(now, e.ID)
	case ModeCycled:
		s.cycleMode(e.Delta)
	case PowerRequested:
		return s.powerRequested(e.Operation)
	case PowerDone:
		return s.powerDone(now, e)
	case ConnectRequested:
		return s.requestConnection(now)
	case ConnectionCancelled:
		s.cancelConnection()
	case ConnectionEnded:
		return s.connectionEnded(now, e)
	case LogsOpened:
		return s.openLogs()
	case LogsClosed:
		s.logOpen = false
	case LogTicked:
		return s.logTicked(e)
	case LogsPeeked:
		s.logsPeeked(now, e)
	case LogsViewed:
		return s.logsViewed(e)
	case Checked:
		return s.checked(now, e)
	case Inspected:
		return s.inspectionDone(now, e)
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

func (s *State) ticked(now time.Time) []Effect {
	effects := s.checkSelected()
	effects = append(effects, s.peekSelected()...)
	effects = append(effects, s.checkWatched(now)...)
	return append(effects, s.dueDiscoveries(now)...)
}

func (s *State) selectTarget(now time.Time, id target.ID) []Effect {
	if s.connection.Target != id {
		s.cancelConnection()
	}
	s.selected, s.modeName = id, ""
	return s.refreshSelected(now)
}

func (s State) targetByID(id target.ID) (target.Target, bool) {
	i := slices.IndexFunc(s.targets, func(t target.Target) bool { return t.ID == id })
	if i < 0 {
		return target.Target{}, false
	}
	return s.resolved(s.targets[i]), true
}

func (s State) resolved(t target.Target) target.Target {
	return t.Resolve(s.entries[t.ID].Profile.Value)
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
