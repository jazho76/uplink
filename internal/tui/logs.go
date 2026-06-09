package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b[@-Z\\\\-_]")

func sanitizeLog(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, s)
}

const (
	logInterval    = time.Second
	logScreenLines = 500
)

type logTickMsg struct{}

func logTickCmd() tea.Cmd {
	return tea.Tick(logInterval, func(time.Time) tea.Msg { return logTickMsg{} })
}

func readLogPeek(name string, n int) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".lima", name, "serial.log")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	const window = 64 << 10
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	offset := info.Size() - window
	if offset < 0 {
		offset = 0
	}
	buf := make([]byte, info.Size()-offset)
	if _, err := f.ReadAt(buf, offset); err != nil && len(buf) == 0 {
		return ""
	}
	return tail(sanitizeLog(string(buf)), n)
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
