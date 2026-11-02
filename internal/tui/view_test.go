package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inspect"
)

const sessionsRule = "tmux ─"

func withSessions(m model, name string, sessions ...inspect.Session) model {
	next, _ := m.Update(liveMsg{id: configID(name), up: true, full: true,
		snapshot: inspect.Snapshot{Tools: []string{"tmux"}, Sessions: sessions}, inspectOK: true})
	return next.(model)
}

func focusedPreview(t *testing.T, sessions ...inspect.Session) model {
	t.Helper()
	m, _ := newTestModel()
	m = focus(sized(m), "forge")
	return withSessions(m, "forge", sessions...)
}

func preview(m model) string { return ansi.Strip(m.previewBody(60, 30)) }

func TestSessionsSitBetweenLiveAndLogs(t *testing.T) {
	m := focusedPreview(t, inspect.Session{Name: "main", Windows: 1})
	m.logPeek = peek{id: configID("forge"), text: "boot line"}

	got := preview(m)
	live, sessions, logs := strings.Index(got, "live ─"), strings.Index(got, sessionsRule), strings.Index(got, "logs ─")
	if live < 0 || sessions < live || logs < sessions {
		t.Errorf("want live, tmux, logs in order, got:\n%s", got)
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
	quiet.Stats = nil
	m = focus(sized(withTargets(m, quiet)), "dojo")

	if got := preview(m); strings.Contains(got, sessionsRule) {
		t.Errorf("an uninspected target has no section, got:\n%s", got)
	}
	got := preview(withSessions(m, "dojo", inspect.Session{Name: "main", Windows: 1}))
	if !strings.Contains(got, sessionsRule) || !strings.Contains(got, "main") || strings.Contains(got, "live ─") {
		t.Errorf("want sessions and no live block, got:\n%s", got)
	}
}

func TestSessionsDisappearWhenTheyCannotBeTrusted(t *testing.T) {
	sessions := inspect.Session{Name: "main", Windows: 1}
	m := focusedPreview(t, sessions)

	down := probed(m, "forge", false)
	if got := preview(down); strings.Contains(got, sessionsRule) {
		t.Errorf("a stopped target has no current sessions, got:\n%s", got)
	}

	next, _ := m.Update(liveMsg{id: configID("forge"), up: true, full: true})
	failed := next.(model)
	if got := preview(failed); strings.Contains(got, sessionsRule) {
		t.Errorf("a failed inspection has no current sessions, got:\n%s", got)
	}

	if got := preview(withSessions(failed, "forge", sessions)); !strings.Contains(got, sessionsRule) {
		t.Errorf("sessions should return with the next inspection, got:\n%s", got)
	}
}

func TestSessionRowsDescribeTheSession(t *testing.T) {
	got := preview(focusedPreview(t,
		inspect.Session{Name: "work", Windows: 3, Attached: true},
		inspect.Session{Name: "my notes", Windows: 1},
	))
	lines := strings.Split(got, "\n")
	row := func(name string) string {
		i := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, name) })
		if i < 0 {
			t.Fatalf("no row for %q in:\n%s", name, got)
		}
		return lines[i]
	}
	if work := row("work"); !strings.Contains(work, "3 windows") || !strings.Contains(work, "attached") {
		t.Errorf("want plural and attached, got %q", work)
	}
	if notes := row("my notes"); !strings.Contains(notes, "1 window") || strings.Contains(notes, "windows") || strings.Contains(notes, "attached") {
		t.Errorf("want singular and detached, got %q", notes)
	}
}

func TestLongSessionNameLeavesRoomForDetails(t *testing.T) {
	got := preview(focusedPreview(t,
		inspect.Session{Name: strings.Repeat("x", 100), Windows: 2},
		inspect.Session{Name: "main", Windows: 3, Attached: true},
	))
	for _, want := range []string{"2 windows", "3 windows", "attached"} {
		if !strings.Contains(got, want) {
			t.Errorf("a long name must not push out %q:\n%s", want, got)
		}
	}
}

func TestNarrowSessionsStayInThePane(t *testing.T) {
	m := focusedPreview(t, inspect.Session{Name: "a session with a long name", Windows: 12, Attached: true})
	const width = 20
	for _, line := range strings.Split(m.previewBody(width, 30), "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line is %d cells, pane is %d: %q", got, width, ansi.Strip(line))
		}
	}
}

func TestLogsShrinkToFitSessions(t *testing.T) {
	m := focusedPreview(t, inspect.Session{Name: "one", Windows: 1}, inspect.Session{Name: "two", Windows: 1})
	m.logPeek = peek{id: configID("forge"), text: strings.Repeat("old line\n", 30) + "newest line"}

	const height = 16
	got := ansi.Strip(m.previewBody(60, height))
	if lines := strings.Count(got, "\n") + 1; lines > height {
		t.Errorf("want at most %d lines, got %d:\n%s", height, lines, got)
	}
	for _, want := range []string{sessionsRule, "two", "logs ─", "newest line"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q to fit:\n%s", want, got)
		}
	}
}

func TestNarrowLiveBlockDropsTheBarNotTheReading(t *testing.T) {
	gauges := []gaugeRow{{label: "memory", fraction: 0.5, reading: "7.6/31GiB"}}

	cramped := ansi.Strip(gaugeFields(gauges, 24)[0].value)
	if strings.ContainsAny(cramped, meterFill+meterTrack) {
		t.Errorf("a pane with no room for a reading draws no bar, got %q", cramped)
	}
	if !strings.Contains(cramped, "7.6/31GiB") {
		t.Errorf("the reading survives however narrow the pane, got %q", cramped)
	}

	roomy := ansi.Strip(gaugeFields(gauges, 70)[0].value)
	if !strings.Contains(roomy, meterFill) {
		t.Errorf("a roomy pane should draw a bar, got %q", roomy)
	}

	if got := ansi.Strip(gauge("", "7.6GiB / 31GiB", gaugeGap)); got != "7.6GiB / 31GiB" {
		t.Errorf("an absent bar leaves the reading alone, got %q", got)
	}
	if got := ansi.Strip(gauge(meter(0.5, 4), "1.32", gaugeGap)); got != "██░░   1.32" {
		t.Errorf("a bar keeps its reading beside it, got %q", got)
	}
}

func TestAmountCollapsesASharedUnit(t *testing.T) {
	for _, c := range []struct {
		used, total uint64
		want        string
	}{
		{used: 780 << 30, total: 930 << 30, want: "780/930GiB"},
		{used: 8 << 30, total: 31 << 30, want: "8/31GiB"},
		{used: 900 << 20, total: 31 << 30, want: "900MiB/31GiB"},
		{used: 0, total: 0, want: "0/0B"},
	} {
		if got := amount(c.used, c.total); got != c.want {
			t.Errorf("amount(%d, %d) = %q, want %q", c.used, c.total, got, c.want)
		}
	}
}
