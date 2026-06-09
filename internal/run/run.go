package run

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

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

func Exec(bin string, args ...string) error {
	path, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("%s: %w", bin, err)
	}
	argv := append([]string{path}, args...)
	return syscall.Exec(path, argv, os.Environ())
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
