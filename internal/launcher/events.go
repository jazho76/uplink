package launcher

import (
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/target"
)

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
		ID     target.ID
		Epoch  int
		Status target.Status
		Reason string
		Full   bool
	}

	Inspected struct {
		ID    target.ID
		Epoch int
		Env   target.Environment
		Err   error
	}

	ReloadRequested struct{ Notice string }
	ConfigRead      struct {
		Generation int
		Plan       inventory.Plan
		Notice     string
	}
	DiscoveryDone struct {
		Provider string
		Epoch    int
		Found    []inventory.Discovered
		Err      error
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
func (Inspected) isEvent()        {}
func (ConfigRead) isEvent()       {}
func (DiscoveryDone) isEvent()    {}
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
	Inspect struct {
		Target target.Target
		Epoch  int
	}
	ReadConfig struct {
		Generation int
		Notice     string
	}
	Discover struct {
		Provider string
		Epoch    int
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
func (Inspect) isEffect()         {}
func (ReadConfig) isEffect()      {}
func (Discover) isEffect()        {}
func (Connect) isEffect()         {}
func (ScheduleLogTick) isEffect() {}
