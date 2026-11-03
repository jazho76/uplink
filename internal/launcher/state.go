package launcher

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

const (
	LiveInterval = 5 * time.Second
	LogInterval  = time.Second

	watchInterval  = 30 * time.Second
	intervalGrace  = time.Second
	maxWatchChecks = 4
	maxLogPeek     = 300
	logScreenLines = 500
)

var (
	errDown  = errors.New("target is not up")
	errStale = errors.New("stats changed")
)

type Operation string

const (
	OperationNone     Operation = ""
	OperationStarting Operation = "starting"
	OperationStopping Operation = "stopping"
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

type discovery struct {
	inFlight bool
	issuedAt time.Time
	epoch    int
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

func (s State) Connection() Connection { return s.connection }

func (s State) Notice() string { return cmp.Or(s.notice, problemsNotice(s.problems)) }

func (s State) Observation(id target.ID) Observation {
	observation := s.entries[id].Observation
	observation.Status = cmp.Or(observation.Status, target.StatusUnknown)
	return observation
}

func (s State) LogsOpen() bool { return s.logOpen }

func (s State) LogTarget() target.Target { return s.logTarget }

func (s State) LogText() string { return s.logText }

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
	if !ok || !s.HasLogs(t) || s.connection.Phase != PhaseNone {
		return nil
	}
	s.logText = s.Logs(t)
	s.logOpen, s.logTarget, s.notice = true, t, ""
	s.logRound++
	return []Effect{ViewLogs{Target: t, Round: s.logRound}}
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
		return reason
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

func verb(operation Operation) string {
	if operation == OperationStarting {
		return "start"
	}
	return "stop"
}

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

func (s *State) inspected(now time.Time, e Inspected) {
	current := s.entries[e.ID]
	current.checking = false
	if e.Epoch == current.epoch && current.Status == target.StatusUp {
		if e.Err != nil {
			current.failInspection(e.Err, now)
		} else {
			current.Stats.Observe(e.Env.Readings, now)
			current.Tools.Observe(e.Env.Tools, now)
			current.Sessions.Observe(e.Env.Sessions, now)
		}
	}
	s.entries[e.ID] = current
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
