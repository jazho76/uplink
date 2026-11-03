package launcher

import (
	"context"
	"sync"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/shell"
)

const stopWait = 2 * time.Second

type Catalog interface {
	ReadPlan() inventory.Plan
	Discover(ctx context.Context, name string) ([]inventory.Discovered, error)
}

type Runner struct {
	Backend Backend
	Catalog Catalog

	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	stopped  bool
	inFlight sync.WaitGroup
}

func NewRunner(backend Backend, catalog Catalog) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{Backend: backend, Catalog: catalog, ctx: ctx, cancel: cancel}
}

func (r *Runner) Run(effect Effect) Event {
	if power, ok := effect.(Power); ok {
		return r.operate(context.WithoutCancel(r.ctx), power)
	}
	if !r.begin() {
		return nil
	}
	defer r.inFlight.Done()
	return r.probe(r.ctx, effect)
}

func (r *Runner) begin() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return false
	}
	r.inFlight.Add(1)
	return true
}

func (r *Runner) Stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.cancel()

	done := make(chan struct{})
	go func() {
		r.inFlight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(stopWait):
	}
}

func (r *Runner) operate(ctx context.Context, e Power) Event {
	operation := r.Backend.Down
	if e.Operation == OperationStarting {
		operation = r.Backend.Up
	}
	return PowerDone{ID: e.Target.ID, Operation: e.Operation, Err: operation(ctx, e.Target)}
}

func (r *Runner) probe(ctx context.Context, effect Effect) Event {
	switch e := effect.(type) {
	case Check:
		status, err := r.Backend.Status(ctx, e.Target)
		return Checked{ID: e.Target.ID, Epoch: e.Epoch, Full: e.Full, Status: status, Reason: shell.Describe(err)}

	case PeekLogs:
		return LogsPeeked{ID: e.Target.ID, Text: r.Backend.Logs(ctx, e.Target, maxLogPeek)}

	case ViewLogs:
		return LogsViewed{Round: e.Round, Text: r.Backend.Logs(ctx, e.Target, logScreenLines)}

	case Inspect:
		env, err := r.Backend.Inspect(ctx, e.Target)
		return Inspected{ID: e.Target.ID, Epoch: e.Epoch, Env: env, Err: err}

	case ReadConfig:
		return ConfigRead{Generation: e.Generation, Plan: r.Catalog.ReadPlan(), Notice: e.Notice}

	case Discover:
		found, err := r.Catalog.Discover(ctx, e.Provider)
		return DiscoveryDone{Provider: e.Provider, Epoch: e.Epoch, Found: found, Err: err}
	}
	return nil
}
