package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/target"
)

const sessionsRule = "sessions ─"

func withSessions(m model, name string, sessions ...target.Session) model {
	owned := slices.Clone(sessions)
	for i := range owned {
		owned[i].Tool = cmp.Or(owned[i].Tool, "tmux")
	}
	return reported(m, configID(name), target.Environment{Tools: []string{"tmux"}, Sessions: owned})
}

func focusedPreview(t *testing.T, sessions ...target.Session) model {
	t.Helper()
	m, _ := newTestModel()
	m = focus(sized(m), "forge")
	return withSessions(m, "forge", sessions...)
}

func modesNamed(names ...string) []target.Mode {
	modes := make([]target.Mode, len(names))
	for i, name := range names {
		modes[i] = target.Mode{Name: name, Argv: []string{"sh"}}
		if name == "tmux" || name == "zellij" {
			modes[i].Multiplexer = name
		}
	}
	return modes
}

func focusedWithModes(names ...string) model {
	box := vm("box")
	box.Modes = modesNamed(names...)
	m, _ := newTestModel()
	return focus(sized(withTargets(m, box)), "box")
}

func TestSessionsSitBetweenLiveAndLogs(t *testing.T) {
	m := focusedPreview(t, target.Session{Name: "main"})
	m = withLogs(m, configID("forge"), "boot line")

	got := preview(m)
	live, sessions, logs := strings.Index(got, "live ─"), strings.Index(got, sessionsRule), strings.Index(got, "logs ─")
	if live < 0 || sessions < live || logs < sessions {
		t.Errorf("want live, sessions, logs in order, got:\n%s", got)
	}
}

func TestNoSessionsMeansNoSection(t *testing.T) {
	if got := preview(focusedPreview(t)); strings.Contains(got, sessionsRule) {
		t.Errorf("no sessions should show no section, got:\n%s", got)
	}
}

func TestSessionsShowWithoutStats(t *testing.T) {
	m, _ := newTestModel()
	quiet := remote("dojo")
	quiet.Metrics = []target.Metric{}
	m = focus(sized(withTargets(m, quiet)), "dojo")

	if got := preview(m); strings.Contains(got, sessionsRule) {
		t.Errorf("an uninspected target has no section, got:\n%s", got)
	}
	got := preview(withSessions(m, "dojo", target.Session{Name: "main"}))
	if !strings.Contains(got, sessionsRule) || !strings.Contains(got, "main") || strings.Contains(got, "live ─") {
		t.Errorf("want sessions and no live block, got:\n%s", got)
	}
}

func TestTabToAModeWithoutSessionsHidesTheSection(t *testing.T) {
	m, backend := newTestModel()
	m = withSessions(focus(sized(m), "forge"), "forge", target.Session{Name: "main"})
	inspections := backend.inspections

	shell := key(m, "tab")
	if got := preview(shell); strings.Contains(got, sessionsRule) || strings.Contains(got, "main") {
		t.Errorf("shell attaches to no session, got:\n%s", got)
	}
	if got := preview(key(shell, "shift+tab")); !strings.Contains(got, "main") {
		t.Errorf("back on tmux the sessions should return, got:\n%s", got)
	}
	if backend.inspections != inspections {
		t.Errorf("tab must not inspect, ran %d more", backend.inspections-inspections)
	}
}

func TestSessionsFollowTheSelectedMode(t *testing.T) {
	m := withSessions(focusedWithModes("tmux", "zellij"), "box",
		target.Session{Tool: "tmux", Name: "from-tmux"},
		target.Session{Tool: "zellij", Name: "from-zellij"},
	)

	if got := preview(m); !strings.Contains(got, "from-tmux") || strings.Contains(got, "from-zellij") {
		t.Errorf("tmux mode should show only tmux sessions, got:\n%s", got)
	}
	if got := preview(key(m, "tab")); !strings.Contains(got, "from-zellij") || strings.Contains(got, "from-tmux") {
		t.Errorf("zellij mode should show only zellij sessions, got:\n%s", got)
	}
}

func TestOnlyModesNamingAMultiplexerShowSessions(t *testing.T) {
	m := withSessions(focusedWithModes("tmux", "root", "shell"), "box", target.Session{Name: "main"})

	if got := preview(m); !strings.Contains(got, "main") {
		t.Errorf("a declared tmux mode previews tmux sessions, got:\n%s", got)
	}
	if got := preview(key(m, "tab")); strings.Contains(got, sessionsRule) {
		t.Errorf("root runs its own server, got:\n%s", got)
	}
}

func TestSessionsFollowTheModesMultiplexerNotItsName(t *testing.T) {
	m := focusedWithModes("placeholder")
	box := itemNamed(m, "box").target
	box.Modes = []target.Mode{
		{Name: "work", Argv: []string{"sh"}, Multiplexer: "tmux"},
		{Name: "tmux", Argv: []string{"sh"}},
	}
	m = focus(sized(withTargets(m, box)), "box")
	m = withSessions(m, "box", target.Session{Name: "main"})

	if got := preview(m); !strings.Contains(got, "main") {
		t.Errorf("a mode naming tmux shows its sessions:\n%s", got)
	}
	m = key(m, "tab")
	if got := preview(m); strings.Contains(got, "main") {
		t.Errorf("a mode only called tmux shows none:\n%s", got)
	}
}

func TestSessionsWithoutAMatchingModeShowNowhere(t *testing.T) {
	m := withSessions(focusedWithModes("tmux", "shell"), "box", target.Session{Tool: "zellij", Name: "stranger"})

	for _, mode := range []model{m, key(m, "tab")} {
		if got := preview(mode); strings.Contains(got, sessionsRule) || strings.Contains(got, "stranger") {
			t.Errorf("no mode attaches to these sessions, got:\n%s", got)
		}
	}
}

func TestSessionsDisappearWhenTheyCannotBeTrusted(t *testing.T) {
	sessions := target.Session{Name: "main"}
	m := focusedPreview(t, sessions)

	down := probed(m, "forge", false)
	if got := preview(down); strings.Contains(got, sessionsRule) {
		t.Errorf("a stopped target has no current sessions, got:\n%s", got)
	}

	failed := failedInspection(m, configID("forge"))
	if got := preview(failed); strings.Contains(got, sessionsRule) {
		t.Errorf("a failed inspection has no current sessions, got:\n%s", got)
	}

	if got := preview(withSessions(failed, "forge", sessions)); !strings.Contains(got, sessionsRule) {
		t.Errorf("sessions should return with the next inspection, got:\n%s", got)
	}
}

func TestSessionRowsDescribeTheSession(t *testing.T) {
	got := preview(focusedPreview(t,
		target.Session{Name: "work", Attached: true, Windows: []target.Window{
			{Name: "editor", Panes: 1},
			{Name: "dev server", Panes: 2},
		}},
		target.Session{Name: "my notes", Windows: []target.Window{{Name: "vim", Panes: 1}}},
	))
	lines := strings.Split(got, "\n")
	row := func(name string) string {
		i := slices.IndexFunc(lines, func(line string) bool { return strings.Contains(line, name) })
		if i < 0 {
			t.Fatalf("no row for %q in:\n%s", name, got)
		}
		return lines[i]
	}
	if work := row("work"); !strings.HasPrefix(work, attachedGlyph+" work") || !strings.Contains(work, "editor · dev server (2)") {
		t.Errorf("want the attached session marked and panes above one counted, got %q", work)
	}
	if notes := row("my notes"); !strings.HasPrefix(notes, "  my notes") || !strings.Contains(notes, "vim") || strings.Contains(notes, "(1)") {
		t.Errorf("want a detached session with a blank margin and no single-pane count, got %q", notes)
	}
	column := func(line, text string) int { return lipgloss.Width(line[:strings.Index(line, text)]) }
	if work, notes := column(row("work"), "editor"), column(row("my notes"), "vim"); work != notes {
		t.Errorf("the mark must not shift the windows: %d and %d", work, notes)
	}
	if work, notes := column(row("work"), "work"), column(row("my notes"), "my notes"); work != notes {
		t.Errorf("the mark must not shift the names: %d and %d", work, notes)
	}
}

func TestWindowsThatDoNotFitAreCounted(t *testing.T) {
	var windows []target.Window
	for _, name := range strings.Fields("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike") {
		windows = append(windows, target.Window{Name: name, Panes: 1})
	}
	got := preview(focusedPreview(t, target.Session{Name: "main", Attached: true, Windows: windows}))
	for _, want := range []string{attachedGlyph + " main", "alpha", "+"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "mike") {
		t.Errorf("the last window should not fit:\n%s", got)
	}
}

func TestLongWindowNameIsClipped(t *testing.T) {
	long := strings.Repeat("w", 100)
	got := preview(focusedPreview(t, target.Session{Name: "main", Windows: []target.Window{{Name: long, Panes: 2}, {Name: "next", Panes: 1}}}))
	if strings.Contains(got, strings.Repeat("w", maxWindowName+1)) || !strings.Contains(got, "(2)") || !strings.Contains(got, "next") {
		t.Errorf("one long name must not hide the rest:\n%s", got)
	}
}

func TestLongSessionNameLeavesRoomForDetails(t *testing.T) {
	got := preview(focusedPreview(t,
		target.Session{Name: strings.Repeat("x", 100), Windows: []target.Window{{Name: "editor", Panes: 1}}},
		target.Session{Name: "main", Attached: true, Windows: []target.Window{{Name: "vim", Panes: 1}}},
	))
	for _, want := range []string{"editor", "vim", attachedGlyph + " main"} {
		if !strings.Contains(got, want) {
			t.Errorf("a long name must not push out %q:\n%s", want, got)
		}
	}
}

func TestNarrowSessionsStayInThePane(t *testing.T) {
	m := focusedPreview(t, target.Session{Name: "a session with a long name", Attached: true, Windows: []target.Window{
		{Name: "a window with a long name", Panes: 3},
		{Name: "another", Panes: 1},
	}})
	const width = 20
	for _, line := range strings.Split(m.previewBody(width, 30), "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line is %d cells, pane is %d: %q", got, width, ansi.Strip(line))
		}
	}
}
