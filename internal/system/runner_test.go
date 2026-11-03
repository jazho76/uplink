package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

type blockingBackend struct {
	Commands
	statusStarted chan struct{}
	statusErr     chan error
	upCtxErr      chan error
	release       chan struct{}
}

func (b blockingBackend) Status(ctx context.Context, _ target.Target) (target.Status, error) {
	close(b.statusStarted)
	<-ctx.Done()
	b.statusErr <- ctx.Err()
	return target.StatusUnknown, ctx.Err()
}

func (b blockingBackend) Up(ctx context.Context, _ target.Target) error {
	<-b.release
	b.upCtxErr <- ctx.Err()
	return nil
}

func TestStopCancelsProbesAndWaitsForThem(t *testing.T) {
	backend := blockingBackend{statusStarted: make(chan struct{}), statusErr: make(chan error, 1)}
	runner := NewRunner(backend, nil)
	finished := make(chan launcher.Event, 1)
	go func() { finished <- runner.Run(launcher.Check{Target: box("vm")}) }()
	<-backend.statusStarted

	runner.Stop()
	if err := <-backend.statusErr; !errors.Is(err, context.Canceled) {
		t.Errorf("a probe should see the cancellation, got %v", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Error("Stop should wait for the probe to finish")
	}
}

func TestPowerOutlivesStop(t *testing.T) {
	backend := blockingBackend{upCtxErr: make(chan error, 1), release: make(chan struct{})}
	runner := NewRunner(backend, nil)
	finished := make(chan launcher.Event, 1)
	go func() {
		finished <- runner.Run(launcher.Power{Target: box("vm"), Operation: launcher.OperationStarting})
	}()

	start := time.Now()
	runner.Stop()
	if time.Since(start) > stopWait/2 {
		t.Errorf("Stop must not wait for a start to finish, took %s", time.Since(start))
	}
	close(backend.release)
	if err := <-backend.upCtxErr; err != nil {
		t.Errorf("a start keeps its own lifetime, got %v", err)
	}
	if done, ok := (<-finished).(launcher.PowerDone); !ok || done.Err != nil {
		t.Errorf("got %+v", done)
	}
}

func TestNothingStartsAfterStop(t *testing.T) {
	runner := NewRunner(Commands{}, nil)
	runner.Stop()
	if got := runner.Run(launcher.Check{Target: box("vm")}); got != nil {
		t.Errorf("a stopped runner starts nothing, got %+v", got)
	}
}

func TestStopKillsTheProbeProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	vm := box("vm")
	vm.Check = "sleep 30 & echo $! > " + pidFile + "; wait"
	runner := NewRunner(Commands{}, nil)
	finished := make(chan launcher.Event, 1)
	go func() { finished <- runner.Run(launcher.Check{Target: vm}) }()

	var pid int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && pid == 0; time.Sleep(10 * time.Millisecond) {
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if pid == 0 {
		t.Fatal("the probe never started")
	}

	runner.Stop()
	if checked, ok := (<-finished).(launcher.Checked); !ok || checked.Status != target.StatusUnknown {
		t.Errorf("got %+v", checked)
	}
	time.Sleep(50 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("process %d outlived the dashboard", pid)
	}
}

type fakeCatalog struct{}

func (fakeCatalog) ReadPlan() inventory.Plan { return inventory.Plan{Providers: []string{"lima"}} }

func (fakeCatalog) Discover(_ context.Context, name string) ([]inventory.Discovered, error) {
	return nil, errors.New(name + " failed")
}

func TestRunnerReadsConfigAndDiscovers(t *testing.T) {
	runner := NewRunner(Commands{}, fakeCatalog{})
	if read := runner.Run(launcher.ReadConfig{Generation: 3, Notice: "n"}).(launcher.ConfigRead); read.Generation != 3 || read.Notice != "n" || len(read.Plan.Providers) != 1 {
		t.Errorf("got %+v", read)
	}
	done := runner.Run(launcher.Discover{Provider: "lima", Epoch: 2}).(launcher.DiscoveryDone)
	if done.Provider != "lima" || done.Epoch != 2 || done.Err == nil {
		t.Errorf("got %+v", done)
	}
}

func box(name string) target.Target {
	return target.Target{ID: target.ID{Source: target.ConfigSource, Key: name}, Name: name, Modes: []target.Mode{{Name: "shell", Argv: []string{"sh"}}}}
}
