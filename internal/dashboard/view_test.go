package dashboard

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/uplink/internal/target"
)

func TestFooterTracksCapabilities(t *testing.T) {
	m, _ := newTestModel()
	quiet := remote("dojo")
	quiet.Logs = target.LogSource{}
	m = sized(withTargets(m, vm("forge"), quiet))

	m = focus(m, "forge")
	vmFooter := m.renderFooter()
	for _, want := range []string{"connect", "logs", "quit"} {
		if !strings.Contains(vmFooter, want) {
			t.Errorf("vm footer missing %q: %s", want, vmFooter)
		}
	}
	for _, gone := range []string{"stop", "restart", "auto", "del"} {
		if strings.Contains(vmFooter, gone) {
			t.Errorf("lifecycle lives outside uplink, footer offers %q: %s", gone, vmFooter)
		}
	}

	m = focus(m, "dojo")
	if strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("a target without logs should not offer them: %s", m.renderFooter())
	}
}

func TestMultiLineStatusStaysOnOneRow(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = withNotice(m, errors.Join(errBoom{}, errBoom{}).Error())

	if lines := strings.Count(m.renderFooter(), "\n"); lines != 1 {
		t.Errorf("footer must stay two rows regardless of error count, got %d newlines", lines)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) > 24 {
		t.Errorf("a joined error overflowed the height budget: %d lines", len(lines))
	}
}

func TestGlyphDistinguishesState(t *testing.T) {
	shape := func(s string) string {
		mark, _ := glyphOf(s)
		return mark
	}

	up := shape(statusUp)
	down := shape(statusDown)
	unknown := shape(statusUnknown)
	starting := shape(statusStarting)

	distinct := map[string]string{
		"up vs down":          up + down,
		"down vs unknown":     down + unknown,
		"starting vs down":    starting + down,
		"starting vs unknown": starting + unknown,
	}
	for name, pair := range distinct {
		half := len(pair) / 2
		if pair[:half] == pair[half:] {
			t.Errorf("%s should be visually distinct, both render %q", name, pair[:half])
		}
	}

	for name, g := range map[string]string{"up": up, "down": down, "unknown": unknown, "starting": starting} {
		if w := lipgloss.Width(g); w != 1 {
			t.Errorf("%s glyph occupies %d columns, want 1", name, w)
		}
	}
}

func TestTerminalTooSmall(t *testing.T) {
	m, _ := newTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m = next.(model)
	if !strings.Contains(m.View(), "too small") {
		t.Fatalf("sub-minimum terminal should show the too-small notice")
	}
}

func TestViewWithinBounds(t *testing.T) {
	sizes := []struct{ w, h int }{{minWidth, minHeight}, {40, 14}, {80, 24}, {120, 40}, {52, 16}}
	for _, sz := range sizes {
		m, backend := newTestModel()
		backend.logs["forge"] = strings.Repeat("a log line that is quite long indeed\n", 20)
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m = next.(model)
		m = focus(m, "forge")
		m = m.withSelectionRefreshed()

		lines := strings.Split(m.View(), "\n")
		if len(lines) > sz.h {
			t.Errorf("%dx%d: rendered %d lines, exceeds height", sz.w, sz.h, len(lines))
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > sz.w {
				t.Errorf("%dx%d: line %d width %d exceeds %d", sz.w, sz.h, i, w, sz.w)
			}
		}
	}
}
