package launcher

import "github.com/jazho76/uplink/internal/target"

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
