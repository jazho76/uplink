package tui

import (
	"cmp"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inspect"
	"github.com/jazho76/uplink/internal/multiplexer"
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
)

const sessionsRule = "sessions ─"

func withSessions(m model, name string, sessions ...multiplexer.Session) model {
	owned := slices.Clone(sessions)
	for i := range owned {
		owned[i].Tool = cmp.Or(owned[i].Tool, "tmux")
	}
	next, _ := m.Update(liveMsg{id: configID(name), up: true, full: true,
		snapshot: inspect.Snapshot{Tools: []string{"tmux"}, Sessions: owned}, inspectOK: true})
	return next.(model)
}

func focusedPreview(t *testing.T, sessions ...multiplexer.Session) model {
	t.Helper()
	m, _ := newTestModel()
	m = focus(sized(m), "forge")
	return withSessions(m, "forge", sessions...)
}

func preview(m model) string { return ansi.Strip(m.previewBody(60, 30)) }

func TestSessionsSitBetweenLiveAndLogs(t *testing.T) {
	m := focusedPreview(t, multiplexer.Session{Name: "main"})
	m.logPeek = peek{id: configID("forge"), text: "boot line"}

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
	quiet.Stats = nil
	m = focus(sized(withTargets(m, quiet)), "dojo")

	if got := preview(m); strings.Contains(got, sessionsRule) {
		t.Errorf("an uninspected target has no section, got:\n%s", got)
	}
	got := preview(withSessions(m, "dojo", multiplexer.Session{Name: "main"}))
	if !strings.Contains(got, sessionsRule) || !strings.Contains(got, "main") || strings.Contains(got, "live ─") {
		t.Errorf("want sessions and no live block, got:\n%s", got)
	}
}

func TestTabToAModeWithoutSessionsHidesTheSection(t *testing.T) {
	m, backend := newTestModel()
	m = withSessions(focus(sized(m), "forge"), "forge", multiplexer.Session{Name: "main"})
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

func modesNamed(names ...string) []target.Mode {
	modes := make([]target.Mode, len(names))
	for i, name := range names {
		modes[i] = target.Mode{Name: name, Argv: []string{"sh"}}
	}
	return modes
}

func focusedWithModes(names ...string) model {
	box := vm("box")
	box.Modes = modesNamed(names...)
	m, _ := newTestModel()
	return focus(sized(withTargets(m, box)), "box")
}

func TestSessionsFollowTheSelectedMode(t *testing.T) {
	m := withSessions(focusedWithModes("tmux", "zellij"), "box",
		multiplexer.Session{Tool: "tmux", Name: "from-tmux"},
		multiplexer.Session{Tool: "zellij", Name: "from-zellij"},
	)

	if got := preview(m); !strings.Contains(got, "from-tmux") || strings.Contains(got, "from-zellij") {
		t.Errorf("tmux mode should show only tmux sessions, got:\n%s", got)
	}
	if got := preview(key(m, "tab")); !strings.Contains(got, "from-zellij") || strings.Contains(got, "from-tmux") {
		t.Errorf("zellij mode should show only zellij sessions, got:\n%s", got)
	}
}

func TestDeclaredModeMatchesSessionsByName(t *testing.T) {
	m := withSessions(focusedWithModes("tmux", "root", "shell"), "box", multiplexer.Session{Name: "main"})

	if got := preview(m); !strings.Contains(got, "main") {
		t.Errorf("a declared tmux mode previews tmux sessions, got:\n%s", got)
	}
	if got := preview(key(m, "tab")); strings.Contains(got, sessionsRule) {
		t.Errorf("root runs its own server, got:\n%s", got)
	}
}

func TestSessionsWithoutAMatchingModeShowNowhere(t *testing.T) {
	m := withSessions(focusedWithModes("tmux", "shell"), "box", multiplexer.Session{Tool: "zellij", Name: "stranger"})

	for _, mode := range []model{m, key(m, "tab")} {
		if got := preview(mode); strings.Contains(got, sessionsRule) || strings.Contains(got, "stranger") {
			t.Errorf("no mode attaches to these sessions, got:\n%s", got)
		}
	}
}

func TestHiddenSessionsGiveTheirLinesToLogs(t *testing.T) {
	m := focusedPreview(t, multiplexer.Session{Name: "one"}, multiplexer.Session{Name: "two"})
	m.logPeek = peek{id: configID("forge"), text: strings.TrimSuffix(strings.Repeat("log line\n", 40), "\n")}

	logLines := func(m model) int { return strings.Count(ansi.Strip(m.previewBody(60, 20)), "log line") }
	if with, without := logLines(m), logLines(key(m, "tab")); without <= with {
		t.Errorf("want more log lines once sessions are hidden, got %d with and %d without", with, without)
	}
}

func TestSessionsDisappearWhenTheyCannotBeTrusted(t *testing.T) {
	sessions := multiplexer.Session{Name: "main"}
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
		multiplexer.Session{Name: "work", Attached: true, Windows: []multiplexer.Window{
			{Name: "editor", Panes: 1},
			{Name: "dev server", Panes: 2},
		}},
		multiplexer.Session{Name: "my notes", Windows: []multiplexer.Window{{Name: "vim", Panes: 1}}},
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
	var windows []multiplexer.Window
	for _, name := range strings.Fields("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike") {
		windows = append(windows, multiplexer.Window{Name: name, Panes: 1})
	}
	got := preview(focusedPreview(t, multiplexer.Session{Name: "main", Attached: true, Windows: windows}))
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
	got := preview(focusedPreview(t, multiplexer.Session{Name: "main", Windows: []multiplexer.Window{{Name: long, Panes: 2}, {Name: "next", Panes: 1}}}))
	if strings.Contains(got, strings.Repeat("w", maxWindowName+1)) || !strings.Contains(got, "(2)") || !strings.Contains(got, "next") {
		t.Errorf("one long name must not hide the rest:\n%s", got)
	}
}

func TestLongSessionNameLeavesRoomForDetails(t *testing.T) {
	got := preview(focusedPreview(t,
		multiplexer.Session{Name: strings.Repeat("x", 100), Windows: []multiplexer.Window{{Name: "editor", Panes: 1}}},
		multiplexer.Session{Name: "main", Attached: true, Windows: []multiplexer.Window{{Name: "vim", Panes: 1}}},
	))
	for _, want := range []string{"editor", "vim", attachedGlyph + " main"} {
		if !strings.Contains(got, want) {
			t.Errorf("a long name must not push out %q:\n%s", want, got)
		}
	}
}

func TestNarrowSessionsStayInThePane(t *testing.T) {
	m := focusedPreview(t, multiplexer.Session{Name: "a session with a long name", Attached: true, Windows: []multiplexer.Window{
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

func TestLogsShrinkToFitSessions(t *testing.T) {
	m := focusedPreview(t, multiplexer.Session{Name: "one"}, multiplexer.Session{Name: "two"})
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

const liveRule = "live ─"

func focusedOnBuiltin() model {
	box := remote("box")
	box.Stats, box.BuiltinStats = nil, true
	m, _ := newTestModel()
	return focus(sized(withTargets(m, box)), "box")
}

func inspectedWith(m model, readings []stats.Reading) model {
	next, _ := m.Update(liveMsg{id: configID("box"), up: true, full: true, snapshot: inspect.Snapshot{OS: "Linux", Readings: readings}, inspectOK: true})
	return next.(model)
}

func TestBuiltinStatsWaitForTheFirstCheck(t *testing.T) {
	got := preview(focusedOnBuiltin())
	if !strings.Contains(got, liveRule) || !strings.Contains(got, "…") {
		t.Errorf("want the live block pending, got:\n%s", got)
	}
}

func TestBuiltinStatsShowTheirReadings(t *testing.T) {
	m := inspectedWith(focusedOnBuiltin(), linuxReadings(1, 4, 1<<30, 4<<30, 0))
	if got := preview(m); !strings.Contains(got, liveRule) || !strings.Contains(got, "memory") {
		t.Errorf("want readings, got:\n%s", got)
	}
	if got := preview(probed(m, "box", false)); !strings.Contains(got, "unavailable") {
		t.Errorf("a known target going down has its stats unavailable, got:\n%s", got)
	}
}

func TestUnsupportedKernelHidesTheLiveBlock(t *testing.T) {
	m := inspectedWith(focusedOnBuiltin(), nil)
	if got := preview(m); strings.Contains(got, liveRule) {
		t.Errorf("no built-in stats for this kernel, got:\n%s", got)
	}
	m.markStale(configID("box"))
	if got := preview(m); strings.Contains(got, liveRule) {
		t.Errorf("going stale must not flash the block back, got:\n%s", got)
	}
}

func TestBuiltinStatsStayHiddenWhenNeverInspected(t *testing.T) {
	if got := preview(probed(focusedOnBuiltin(), "box", false)); strings.Contains(got, liveRule) {
		t.Errorf("a stopped target of unknown kernel has nothing to promise, got:\n%s", got)
	}
	next, _ := focusedOnBuiltin().Update(liveMsg{id: configID("box"), up: true, full: true})
	if got := preview(next.(model)); strings.Contains(got, liveRule) {
		t.Errorf("a failed first inspection has nothing to promise, got:\n%s", got)
	}
}

func TestReloadDropsReadingsWhenStatsBecomeBuiltin(t *testing.T) {
	m, _ := newTestModel()
	m = probed(sized(m), "forge", true)

	changed := vm("forge")
	changed.Stats, changed.BuiltinStats = nil, true
	m.load = func() ([]target.Target, error) { return []target.Target{hostTarget(), changed, vm("tokyo")}, nil }

	next, _ := m.Update(configEditedMsg{})
	if got := next.(model).live[configID("forge")].snapshot.Readings; got != nil {
		t.Errorf("the old metrics no longer apply, got %+v", got)
	}
}

func focusedOnGuestLogs() (model, *fakeBackend) {
	box := remote("box")
	box.Logs, box.GuestLogs = "", true
	m, backend := newTestModel()
	backend.logs["box"] = "journal line"
	return focus(sized(withTargets(m, box)), "box"), backend
}

func inspectedTools(m model, tools ...string) (model, tea.Cmd) {
	next, cmd := m.Update(liveMsg{id: configID("box"), up: true, full: true, snapshot: inspect.Snapshot{Tools: tools}, inspectOK: true})
	return next.(model), cmd
}

func TestGuestLogsWaitForTheJournalToBeReported(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	m = applyCmd(m, m.peekSelectedLogs())
	if backend.logCalls != 0 || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("nothing is known yet: %d calls, footer %s", backend.logCalls, m.renderFooter())
	}
	if got := key(m, "ctrl+l"); got.screen == screenLogs {
		t.Error("ctrl+l has nothing to open")
	}

	m, cmd := inspectedTools(m)
	m = applyCmd(m, cmd)
	if backend.logCalls != 0 || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("no journal here: %d calls, footer %s", backend.logCalls, m.renderFooter())
	}
}

func TestReportedJournalIsPeekedStraightAway(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	m, cmd := inspectedTools(m, inspect.ToolJournal)
	m = applyCmd(m, cmd)

	if backend.logCalls != 1 || !strings.Contains(preview(m), "journal line") {
		t.Errorf("want one peek landing with the snapshot, got %d calls:\n%s", backend.logCalls, preview(m))
	}
	if !strings.Contains(m.renderFooter(), "logs") || key(m, "ctrl+l").screen != screenLogs {
		t.Errorf("logs should now be offered: %s", m.renderFooter())
	}
}

func TestGuestLogsStopWhileDown(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	m, cmd := inspectedTools(m, inspect.ToolJournal)
	m = applyCmd(m, cmd)

	m = probed(m, "box", false)
	calls := backend.logCalls
	m = applyCmd(m, m.peekSelectedLogs())
	if backend.logCalls != calls || strings.Contains(preview(m), "journal line") || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("a stopped target has no journal to read, %d calls:\n%s", backend.logCalls-calls, preview(m))
	}
}

func TestHostSideLogsSurviveDown(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "serial line"
	next, cmd := focus(sized(m), "forge").Update(liveMsg{id: configID("forge"), up: false, full: true})
	m = applyCmd(next.(model), cmd)
	if !strings.Contains(preview(m), "serial line") || !strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("host-side logs do not need the target up:\n%s", preview(m))
	}
}

func TestNoLogsOffersNothing(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	box := m.selected().target
	box.GuestLogs = false
	m = focus(withTargets(m, box), "box")
	m, cmd := inspectedTools(m, inspect.ToolJournal)
	m = applyCmd(m, cmd)
	if backend.logCalls != 0 || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf(`logs: "" reads nothing: %d calls, footer %s`, backend.logCalls, m.renderFooter())
	}
}

func TestOnlyOneLogPeekRunsAtATime(t *testing.T) {
	m, _ := newTestModel()
	m = focus(sized(m), "forge")
	if m.peekSelectedLogs() == nil {
		t.Fatal("the first peek should go out")
	}
	if m.peekSelectedLogs() != nil {
		t.Error("a slow host must not stack up peeks")
	}

	next, _ := m.Update(logPeekMsg{id: configID("forge")})
	m = next.(model)
	if m.peekSelectedLogs() == nil {
		t.Error("the next peek may go out once the last one lands")
	}

	m = focus(m, "tokyo")
	next, _ = m.Update(logPeekMsg{id: configID("forge")})
	m = focus(next.(model), "forge")
	if m.peekSelectedLogs() == nil {
		t.Error("a result landing after moving away still frees its target")
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

func modeTagOf(m model) string { return ansi.Strip(m.modeTag(m.selected())) }

func wentDown(m model, name string) model {
	next, _ := m.Update(liveMsg{id: configID(name), up: false})
	return next.(model)
}

func TestDownTargetDoubtsAModeItCouldNotConfirm(t *testing.T) {
	m, _ := focusedOn("box")
	if got := modeTagOf(m); strings.Contains(got, "?") {
		t.Errorf("no doubt before the first check, got %q", got)
	}

	m = wentDown(m, "box")
	if got := modeTagOf(m); !strings.Contains(got, "tmux?") {
		t.Errorf("a down target cannot confirm tmux, got %q", got)
	}
	if got := modeTagOf(key(m, "tab")); !strings.Contains(got, "shell") || strings.Contains(got, "?") {
		t.Errorf("shell requires nothing, got %q", got)
	}
}

func TestDownTargetTagCountsOnlyWhatItOffers(t *testing.T) {
	m := probed(focusedOnAutomaticModes("tmux", "shell", "zellij"), "box", false)
	if got := modeTagOf(m); !strings.Contains(got, "tmux?") || !strings.Contains(got, "1/2") {
		t.Errorf("want tmux? 1/2, got %q", got)
	}
}

func TestUpTargetStillBeingInspectedShowsNoDoubt(t *testing.T) {
	m, _ := focusedOn("box")
	next, _ := m.Update(liveMsg{id: configID("box"), up: true})
	if got := modeTagOf(next.(model)); !strings.Contains(got, "tmux") || strings.Contains(got, "?") {
		t.Errorf("want plain tmux while up, got %q", got)
	}
}

func TestConfirmedModeStaysCertainOnceDown(t *testing.T) {
	m, _ := focusedOn("box")
	m = wentDown(inspected(m, "box", multiplexer.Tmux{}.Tool()), "box")
	if got := modeTagOf(m); !strings.Contains(got, "tmux") || strings.Contains(got, "?") {
		t.Errorf("tmux was confirmed before it went down, got %q", got)
	}
}

func TestDeclaredModeOnDownTargetShowsNoDoubt(t *testing.T) {
	m, _ := newTestModel()
	m = wentDown(focus(sized(m), "forge"), "forge")
	if got := modeTagOf(m); strings.Contains(got, "?") {
		t.Errorf("a declared mode requires nothing, got %q", got)
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
