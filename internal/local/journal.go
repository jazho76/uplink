package local

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jazho76/uplink/internal/run"
)

const (
	journalBin     = "journalctl"
	journalTimeout = 2 * time.Second
	journalEmpty   = "-- No entries --"
)

func journalAvailable() bool {
	_, err := exec.LookPath(journalBin)
	return err == nil
}

func (p *Provider) Tail(_ string, lines int) string {
	if !p.hasJournal {
		return ""
	}
	out, err := run.OutputWithin(journalTimeout, journalBin, "--system",
		"-n", strconv.Itoa(lines), "--no-pager", "--no-hostname", "-o", "short")
	if err != nil {
		return ""
	}
	return dropEmptyMarker(out)
}

func dropEmptyMarker(out string) string {
	if strings.TrimSpace(out) == journalEmpty {
		return ""
	}
	return out
}
