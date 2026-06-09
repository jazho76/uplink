package clipboard

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jazho76/vmm/internal/lima"
	"github.com/jazho76/vmm/internal/run"
)

func Push(name string) (string, error) {
	if _, err := exec.LookPath("wl-paste"); err != nil {
		return "", fmt.Errorf("wl-clipboard not installed (wl-paste required)")
	}
	if _, err := exec.LookPath("wl-copy"); err != nil {
		return "", fmt.Errorf("wl-clipboard not installed (wl-copy required)")
	}

	if name == "" {
		var err error
		if name, err = soleRunning(); err != nil {
			return "", err
		}
	}

	mime, err := pickType()
	if err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp("", "vmm-clipboard-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := paste(mime, tmp.Name()); err != nil {
		return "", err
	}
	info, err := os.Stat(tmp.Name())
	if err != nil || info.Size() == 0 {
		return "", fmt.Errorf("failed to read clipboard (%s)", mime)
	}

	remote := "/tmp/clipboard"
	if mime != "text/plain" {
		remote = fmt.Sprintf("/tmp/clipboard-%s.%s", token(), ext(mime))
	}

	if err := lima.Copy(tmp.Name(), name, remote); err != nil {
		return "", fmt.Errorf("limactl copy failed: %w", err)
	}

	if mime != "text/plain" {
		if err := copyToHost(remote); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%s -> %s:%s (%d bytes)", mime, name, remote, info.Size()), nil
}

func soleRunning() (string, error) {
	instances, err := lima.List()
	if err != nil {
		return "", err
	}
	var running []string
	for _, i := range instances {
		if i.Running() {
			running = append(running, i.Name)
		}
	}
	switch len(running) {
	case 1:
		return running[0], nil
	case 0:
		return "", fmt.Errorf("no single running VM (none)")
	default:
		return "", fmt.Errorf("no single running VM (%s)", strings.Join(running, " "))
	}
}

func pickType() (string, error) {
	out, err := run.Output("wl-paste", "-l")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("clipboard empty")
	}
	types := strings.Fields(out)
	for _, t := range types {
		if strings.HasPrefix(t, "image/") {
			return t, nil
		}
	}
	for _, t := range types {
		if t == "text/plain" {
			return t, nil
		}
	}
	return types[0], nil
}

func paste(mime, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command("wl-paste", "-t", mime)
	cmd.Stdout = f
	return cmd.Run()
}

func copyToHost(s string) error {
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

func token() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
