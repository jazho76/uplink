package process

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNonInteractiveTimeoutKillsTheWholeGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	_, err := NonInteractive(context.Background(), 200*time.Millisecond, "", "sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
	if err == nil {
		t.Fatal("a command past its limit must fail")
	}

	data, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("the child never started: %v", readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(pid, 0) == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("child %d outlived the timeout", pid)
	}
}

func TestTimeoutIsVisibleAndAFailureIsNot(t *testing.T) {
	_, err := NonInteractive(context.Background(), 50*time.Millisecond, "", "sleep", "5")
	if !Interrupted(err) {
		t.Errorf("a command past its limit should read as interrupted, got %v", err)
	}
	if _, err := NonInteractive(context.Background(), time.Second, "", "false"); err == nil || Interrupted(err) {
		t.Errorf("a command that fails by itself is not interrupted, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NonInteractive(ctx, time.Second, "", "sleep", "5"); !Interrupted(err) {
		t.Errorf("a cancelled command should read as interrupted, got %v", err)
	}
}

func TestOutputKeepsOnlyTheTailOfAFlood(t *testing.T) {
	out, err := NonInteractive(context.Background(), 20*time.Second, "", "sh", "-c", "head -c 12000000 /dev/zero | tr '\\0' x; echo; echo the-end")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > maxOutput || !strings.HasSuffix(out, "the-end") {
		t.Errorf("want the last %d bytes with the end kept, got %d", maxOutput, len(out))
	}
}

func TestFailureStillCarriesItsStderr(t *testing.T) {
	_, err := NonInteractive(context.Background(), time.Second, "", "sh", "-c", "echo first >&2; echo why >&2; exit 3")
	if got := StderrTail(err); got != "why" {
		t.Errorf("got %q from %v", got, err)
	}
	if code, ok := ExitCode(err); !ok || code != 3 {
		t.Errorf("got %d, %v", code, ok)
	}
}
