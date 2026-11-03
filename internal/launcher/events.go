package launcher

import "github.com/jazho76/uplink/internal/target"

type Event interface{ isEvent() }

type (
	Started struct{}
	Ticked  struct{}

	Selected   struct{ ID target.ID }
	ModeCycled struct{ Delta int }

	PowerRequested struct{ Operation Operation }
	PowerDone      struct {
		ID        target.ID
		Operation Operation
		Err       error
	}

	ConnectRequested struct{}
	ConnectionEnded  struct {
		Name string
		Err  error
	}

	LogsOpened struct{}
	LogsClosed struct{}
	LogTicked  struct{ Round int }
	LogsPeeked struct {
		ID   target.ID
		Text string
	}
	LogsViewed struct {
		Round int
		Text  string
	}

	Checked struct {
		ID         target.ID
		Epoch      int
		Up         bool
		Full       bool
		Env        target.Environment
		InspectErr error
	}

	ReloadRequested struct{ Notice string }
	Reloaded        struct {
		Generation int
		Targets    []target.Target
		Problems   error
		Notice     string
		Selected   target.ID
	}

	NoticeSet struct{ Text string }
)

func (Started) isEvent()          {}
func (Ticked) isEvent()           {}
func (Selected) isEvent()         {}
func (ModeCycled) isEvent()       {}
func (PowerRequested) isEvent()   {}
func (PowerDone) isEvent()        {}
func (ConnectRequested) isEvent() {}
func (ConnectionEnded) isEvent()  {}
func (LogsOpened) isEvent()       {}
func (LogsClosed) isEvent()       {}
func (LogTicked) isEvent()        {}
func (LogsPeeked) isEvent()       {}
func (LogsViewed) isEvent()       {}
func (Checked) isEvent()          {}
func (ReloadRequested) isEvent()  {}
func (Reloaded) isEvent()         {}
func (NoticeSet) isEvent()        {}

type Effect interface{ isEffect() }

type (
	Check struct {
		Target target.Target
		Full   bool
		Epoch  int
	}
	PeekLogs struct{ Target target.Target }
	ViewLogs struct {
		Target target.Target
		Round  int
	}
	Power struct {
		Target    target.Target
		Operation Operation
	}
	Reload struct {
		Generation int
		Notice     string
	}

	Connect struct {
		Name string
		Argv []string
	}
	ScheduleLogTick struct{ Round int }
)

func (Check) isEffect()           {}
func (PeekLogs) isEffect()        {}
func (ViewLogs) isEffect()        {}
func (Power) isEffect()           {}
func (Reload) isEffect()          {}
func (Connect) isEffect()         {}
func (ScheduleLogTick) isEffect() {}
