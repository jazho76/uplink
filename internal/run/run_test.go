package run

import (
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
	_, err := NonInteractive(200*time.Millisecond, "", "sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
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
