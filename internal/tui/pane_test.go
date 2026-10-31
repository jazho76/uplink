package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/target"
)

func TestSelectedRowFillsEveryCell(t *testing.T) {
	stylingEnabled(t)

	m := model{live: map[target.ID]liveEntry{configID("kyoto"): {statsAt: time.Now()}}}
	it := named("kyoto")
	it.key = 'k'

	const width = 30
	filled := m.paneRow(it, true, true, width)
	if got := lipgloss.Width(filled); got != width {
		t.Fatalf("a filled row spans %d cells, want %d", got, width)
	}

	for _, run := range visibleRuns(filled) {
		if !run.styled {
			t.Errorf("an unstyled run breaks the fill: %q in %q", run.text, filled)
		}
	}

	plain := m.paneRow(it, false, true, width)
	if lipgloss.Width(plain) != width {
		t.Errorf("an unselected row still spans its width, got %d", lipgloss.Width(plain))
	}
	unstyled := 0
	for _, run := range visibleRuns(plain) {
		if !run.styled {
			unstyled++
		}
	}
	if unstyled == 0 {
		t.Errorf("only the selected row should be filled end to end: %q", plain)
	}
}

type textRun struct {
	text   string
	styled bool
}

func visibleRuns(line string) []textRun {
	var runs []textRun
	styled := false
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			runs = append(runs, textRun{current.String(), styled})
			current.Reset()
		}
	}
	for i := 0; i < len(line); i++ {
		if line[i] == 0x1b {
			j := strings.IndexByte(line[i:], 'm')
			flush()
			styled = !strings.HasSuffix(line[i:i+j+1], "[0m")
			i += j
			continue
		}
		current.WriteByte(line[i])
	}
	flush()
	return runs
}
