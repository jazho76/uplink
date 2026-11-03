package launcher

import (
	"context"

	"github.com/jazho76/uplink/internal/target"
)

type Runner struct {
	Backend Backend
	Load    func() ([]target.Target, error)
}

func (r *Runner) Run(effect Effect) Event {
	ctx := context.Background()
	switch e := effect.(type) {
	case Check:
		checked := Checked{ID: e.Target.ID, Epoch: e.Epoch, Full: e.Full}
		checked.Up = r.Backend.Status(ctx, e.Target) == nil
		if checked.Up && e.Full {
			checked.Env, checked.InspectErr = r.Backend.Inspect(ctx, e.Target)
		}
		return checked

	case PeekLogs:
		return LogsPeeked{ID: e.Target.ID, Text: r.Backend.Logs(ctx, e.Target, maxLogPeek)}

	case ViewLogs:
		return LogsViewed{Round: e.Round, Text: r.Backend.Logs(ctx, e.Target, logScreenLines)}

	case Power:
		operate := r.Backend.Down
		if e.Operation == OperationStarting {
			operate = r.Backend.Up
		}
		return PowerDone{ID: e.Target.ID, Operation: e.Operation, Err: operate(ctx, e.Target)}

	case Reload:
		targets, problems := r.Load()
		return Reloaded{Generation: e.Generation, Targets: targets, Problems: problems, Notice: e.Notice}
	}
	return nil
}
