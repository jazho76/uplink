package launcher

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"time"

	"github.com/jazho76/uplink/internal/target"
)

const (
	LiveInterval = 5 * time.Second
	LogInterval  = time.Second

	watchInterval  = 30 * time.Second
	maxWatchChecks = 4
	maxLogPeek     = 300
	logScreenLines = 500
)

var (
	errDown  = errors.New("target is down")
	errStale = errors.New("stats changed")
)

type Operation string

const (
	OperationNone     Operation = ""
	OperationStarting Operation = "starting"
	OperationStopping Operation = "stopping"
)

type Observation struct {
	Status    target.Status
	Operation Operation
	Stats     target.Fact[[]target.Reading]
	Tools     target.Fact[[]string]
	Sessions  target.Fact[[]target.Session]
}

func (o Observation) Label() string {
	return cmp.Or(string(o.Operation), string(cmp.Or(o.Status, target.StatusUnknown)))
}

type entry struct {
	Observation
	checking  bool
	peeking   bool
	checkedAt time.Time
	epoch     int
}

type logPeek struct {
	id   target.ID
	text string
	at   time.Time
}

type State struct {
	targets           []target.Target
	entries           map[target.ID]entry
	selected          target.ID
	modeName          string
	notice            string
	connectAfterStart target.ID
	reloadGeneration  int
	peek              logPeek
	logOpen           bool
	logRound          int
	logTarget         target.Target
	logText           string
}

func New(targets []target.Target, problems error) State {
	return State{targets: targets, entries: map[target.ID]entry{}, notice: problemsNotice(problems)}
}

func (s State) Targets() []target.Target { return s.targets }

func (s State) Selection() target.ID { return s.selected }

func (s State) Notice() string { return s.notice }

func (s State) Observation(id target.ID) Observation {
	observation := s.entries[id].Observation
	observation.Status = cmp.Or(observation.Status, target.StatusUnknown)
	return observation
}

func (s State) LogsOpen() bool { return s.logOpen }

func (s State) LogTarget() target.Target { return s.logTarget }

func (s State) LogText() string { return s.logText }

func (s State) ReloadIsCurrent(generation int) bool { return generation == s.reloadGeneration }

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
	effects := s.apply(now, event)
	return s, effects
}

func (s *State) apply(now time.Time, event Event) []Effect {
	switch e := event.(type) {
	case Started:
		return append(s.refreshSelected(now), s.checkWatched(now)...)

	case Ticked:
		effects := s.checkSelected()
		effects = append(effects, s.peekSelected()...)
		return append(effects, s.checkWatched(now)...)

	case Selected:
		s.selected, s.modeName = e.ID, ""
		return s.refreshSelected(now)

	case ModeCycled:
		s.cycleMode(e.Delta)

	case PowerRequested:
		return s.requestPower(e.Operation)

	case PowerDone:
		return s.powerDone(now, e)

	case ConnectRequested:
		return s.requestConnection()

	case ConnectionEnded:
		s.notice = connectionNotice(e)
		s.markStale(s.selected)
		return s.refreshSelected(now)

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
		s.settle(now, e)
		if e.ID == s.selected {
			return s.peekSelectedIfStale(now)
		}

	case ReloadRequested:
		s.reloadGeneration++
		return []Effect{Reload{Generation: s.reloadGeneration, Notice: e.Notice}}

	case Reloaded:
		return s.reloaded(now, e)

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

func (s *State) check(t target.Target, full bool) Effect {
	current := s.entries[t.ID]
	current.checking = true
	s.entries[t.ID] = current
	return Check{Target: t, Full: full, Epoch: current.epoch}
}

func (s *State) refreshSelected(now time.Time) []Effect {
	return append(s.checkSelectedIfStale(now), s.peekSelectedIfStale(now)...)
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
	return []Effect{s.check(t, true)}
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
		if !t.Watch || t.ID == s.selected || current.checking || now.Sub(current.checkedAt) < watchInterval {
			continue
		}
		effects = append(effects, s.check(t, false))
		inFlight++
	}
	return effects
}

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
	return []Effect{PeekLogs{Target: t}}
}

func (s State) HasLogs(t target.Target) bool {
	if t.Logs.Command != "" {
		return true
	}
	observation := s.Observation(t.ID)
	return t.Logs.Journal && observation.Tools.Known && observation.Status != target.StatusDown &&
		slices.Contains(observation.Tools.Value, target.ToolJournal)
}

func (s State) Logs(t target.Target) string {
	if s.peek.id != t.ID || !s.HasLogs(t) {
		return ""
	}
	return s.peek.text
}

func (s *State) openLogs() []Effect {
	t, ok := s.selectedTarget()
	if !ok || !s.HasLogs(t) {
		return nil
	}
	s.logText = s.Logs(t)
	s.logOpen, s.logTarget, s.notice = true, t, ""
	s.logRound++
	return []Effect{ViewLogs{Target: t, Round: s.logRound}}
}

func (s *State) settle(now time.Time, e Checked) {
	current := s.entries[e.ID]
	if e.Epoch != current.epoch {
		return
	}
	current.checking, current.checkedAt = false, now
	current.Status = target.StatusDown
	if e.Up {
		current.Status = target.StatusUp
	}
	switch {
	case e.Full && e.Up && e.InspectErr == nil:
		current.Stats.Observe(e.Env.Readings, now)
		current.Tools.Observe(e.Env.Tools, now)
		current.Sessions.Observe(e.Env.Sessions, now)
	case e.Full && e.Up:
		current.failInspection(e.InspectErr, now)
	case e.Full:
		current.failInspection(errDown, now)
	case !e.Up:
		current.failInspection(errDown, current.Stats.At)
	}
	s.entries[e.ID] = current
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
	connect := e.Operation == OperationStarting && e.Err == nil &&
		s.connectAfterStart == e.ID && s.selected == e.ID
	if s.connectAfterStart == e.ID {
		s.connectAfterStart = target.ID{}
	}
	current := s.entries[e.ID]
	current.Operation = OperationNone
	current.epoch++
	s.entries[e.ID] = current
	s.markStale(e.ID)
	if e.Err != nil {
		s.notice = fmt.Sprintf("%s %s failed: %v", verb(e.Operation), e.ID, e.Err)
	}
	t, ok := s.targetByID(e.ID)
	if !ok {
		return nil
	}
	effects := []Effect{s.check(t, t.ID == s.selected)}
	if connect {
		effects = append(effects, s.connection()...)
	}
	return effects
}

func verb(operation Operation) string {
	if operation == OperationStarting {
		return "start"
	}
	return "stop"
}

func (s *State) requestConnection() []Effect {
	t, ok := s.selectedTarget()
	if !ok {
		return nil
	}
	observation := s.Observation(t.ID)
	switch {
	case observation.Operation != OperationNone:
		return nil
	case observation.Status == target.StatusDown && t.StartCommand() != "":
		s.connectAfterStart = t.ID
		return s.requestPower(OperationStarting)
	}
	return s.connection()
}

func (s State) connection() []Effect {
	mode := s.Mode()
	t, ok := s.selectedTarget()
	if !ok || len(mode.Argv) == 0 {
		return nil
	}
	return []Effect{Connect{Name: t.Name, Argv: mode.Argv}}
}

func connectionNotice(e ConnectionEnded) string {
	var exitErr *exec.ExitError
	switch {
	case e.Err == nil:
		return ""
	case errors.As(e.Err, &exitErr):
		return fmt.Sprintf("%s: session ended with exit %d", e.Name, exitErr.ExitCode())
	default:
		return fmt.Sprintf("%s: %v", e.Name, e.Err)
	}
}

func (s *State) reloaded(now time.Time, e Reloaded) []Effect {
	if !s.ReloadIsCurrent(e.Generation) {
		return nil
	}
	for _, t := range e.Targets {
		previous, ok := s.targetByID(t.ID)
		if ok && (!slices.Equal(previous.Stats.Metrics, t.Stats.Metrics) || previous.Stats.Builtin != t.Stats.Builtin) {
			current := s.entries[t.ID]
			current.Stats.Value, current.Stats.Err = nil, errStale
			current.Sessions.Err = errStale
			s.entries[t.ID] = current
		}
	}
	s.targets, s.selected, s.modeName = e.Targets, e.Selected, ""
	s.markStale(s.selected)
	s.notice = cmp.Or(e.Notice, problemsNotice(e.Problems))
	return append(s.refreshSelected(now), s.checkWatched(now)...)
}

func (s State) Modes(t target.Target) []target.Mode {
	return target.AvailableModes(t, s.Observation(t.ID).Tools)
}

func (s State) ModeIndex() int {
	t, _ := s.selectedTarget()
	selected := slices.IndexFunc(s.Modes(t), func(mode target.Mode) bool { return mode.Name == s.modeName })
	return max(selected, 0)
}

func (s State) Mode() target.Mode {
	t, _ := s.selectedTarget()
	modes := s.Modes(t)
	if len(modes) == 0 {
		return target.Mode{}
	}
	return modes[s.ModeIndex()]
}

func (s *State) cycleMode(delta int) {
	t, _ := s.selectedTarget()
	modes := s.Modes(t)
	if len(modes) < 2 {
		return
	}
	s.modeName = modes[wrap(s.ModeIndex()+delta, len(modes))].Name
}

func wrap(i, n int) int {
	return ((i % n) + n) % n
}

func problemsNotice(problems error) string {
	switch count := countProblems(problems); count {
	case 0:
		return ""
	case 1:
		return "config: " + problems.Error()
	default:
		return fmt.Sprintf("config: %d problems, see uplink config check", count)
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
