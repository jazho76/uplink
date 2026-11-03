package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const maxOutput = 4 << 20

func Stream(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", line(bin, args), err)
	}
	return nil
}

func NonInteractive(ctx context.Context, limit time.Duration, stdin, bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), "SSH_ASKPASS_REQUIRE=never")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	stdout, stderr := &tailBuffer{limit: maxOutput}, &tailBuffer{limit: maxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitErr.Stderr = stderr.Bytes()
	}
	out := strings.TrimSpace(string(stdout.Bytes()))
	switch {
	case err == nil:
		return out, nil
	case ctx.Err() != nil:
		return out, fmt.Errorf("%s: %w: %w", line(bin, args), ctx.Err(), err)
	default:
		return out, fmt.Errorf("%s: %w", line(bin, args), err)
	}
}

type tailBuffer struct {
	data  []byte
	limit int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > 2*b.limit {
		b.data = append(b.data[:0], b.data[len(b.data)-b.limit:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) Bytes() []byte {
	if len(b.data) > b.limit {
		return b.data[len(b.data)-b.limit:]
	}
	return b.data
}

func ExitCode(err error) (int, bool) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, false
	}
	return exitErr.ExitCode(), true
}

func Describe(err error) string {
	switch {
	case err == nil:
		return ""
	case Interrupted(err):
		return "timed out"
	}
	if tail := StderrTail(err); tail != "" {
		return tail
	}
	return err.Error()
}

func Interrupted(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func StderrTail(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(exitErr.Stderr)), "\n")
	return lines[len(lines)-1]
}

func Output(bin string, args ...string) (string, error) {
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", line(bin, args), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func Silent(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", line(bin, args), err)
	}
	return nil
}

func line(bin string, args []string) string {
	if len(args) == 0 {
		return bin
	}
	return bin + " " + strings.Join(args, " ")
}
