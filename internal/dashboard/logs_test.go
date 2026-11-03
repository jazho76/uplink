package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

func focusedOnGuestLogs() (model, *fakeBackend) {
	box := remote("box")
	box.Logs = target.DetectedLogs()
	m, backend := newTestModel()
	backend.logs["box"] = "journal line"
	return focus(sized(withTargets(m, box)), "box"), backend
}

func inspectedLogSystem(m model, system target.LogSystem) (model, tea.Cmd) {
	up, _ := m.Update(launcher.Checked{ID: configID("box"), Status: target.StatusUp, Full: true})
	next, cmd := up.(model).Update(launcher.Inspected{ID: configID("box"), Env: target.Environment{Profile: target.Profile{LogSystem: system}}})
	return next.(model), cmd
}

func TestLogsScreenToggle(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "boot line"
	m = sized(m)
	m = focus(m, "forge")

	m = keyThenApply(m, "ctrl+l")
	if !m.state.LogsOpen() {
		t.Fatalf("ctrl+l on a vm should open the log pager")
	}
	if m.state.LogTarget().Name != "forge" {
		t.Fatalf("log pager should target forge, got %q", m.state.LogTarget().Name)
	}
	if !strings.Contains(m.View(), "boot line") {
		t.Errorf("log pager should render the tail")
	}

	m = key(m, "esc")
	if m.state.LogsOpen() {
		t.Fatalf("esc should close the log pager")
	}

	backend.logs["forge"] = ""
	m = keyThenApply(m, "ctrl+l")
	if !strings.Contains(m.View(), "no log output") {
		t.Errorf("a target with nothing to tail should say so:\n%s", m.View())
	}
}

func TestReopenedLogsDropTheEarlierRound(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "fresh"
	m = sized(m)
	m = focus(m, "forge")

	m = key(m, "ctrl+l")
	stale := 1
	m = key(key(m, "esc"), "ctrl+l")

	next, tick := m.Update(launcher.LogsViewed{Round: stale, Text: "stale"})
	m = next.(model)
	if m.state.LogText() == "stale" || tick != nil {
		t.Errorf("a reply from a closed log view must neither land nor reschedule")
	}
	if _, tick = m.Update(launcher.LogTicked{Round: stale}); tick != nil {
		t.Errorf("a tick from a closed log view must not fetch again")
	}
}

func TestLogPeekBelongsToItsTarget(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	next, _ := m.Update(launcher.LogsPeeked{ID: configID("tokyo"), Text: "tokyo boot"})
	if m = next.(model); m.state.Logs(m.selected().target) != "" {
		t.Errorf("a tail for another target must be dropped, got %q", m.state.Logs(m.selected().target))
	}

	next, _ = m.Update(launcher.LogsPeeked{ID: configID("forge"), Text: "forge boot"})
	if m = next.(model); m.state.Logs(m.selected().target) != "forge boot" {
		t.Errorf("the selected target's tail should land, got %q", m.state.Logs(m.selected().target))
	}
}

func TestGuestLogsWaitForTheJournalToBeReported(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	m = runCmd(m, peekCmd)
	if backend.logCalls != 0 || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("nothing is known yet: %d calls, footer %s", backend.logCalls, m.renderFooter())
	}
	if got := key(m, "ctrl+l"); got.state.LogsOpen() {
		t.Error("ctrl+l has nothing to open")
	}

	m, cmd := inspectedLogSystem(m, "")
	m = applyCmd(m, cmd)
	if backend.logCalls != 0 || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("no log system here: %d calls, footer %s", backend.logCalls, m.renderFooter())
	}
}

func TestReportedLogSystemIsPeekedStraightAway(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	m, cmd := inspectedLogSystem(m, target.LogSystemJournal)
	m = applyCmd(m, cmd)

	if backend.logCalls != 1 || !strings.Contains(preview(m), "journal line") {
		t.Errorf("want one peek landing with the snapshot, got %d calls:\n%s", backend.logCalls, preview(m))
	}
	if !strings.Contains(m.renderFooter(), "logs") || !key(m, "ctrl+l").state.LogsOpen() {
		t.Errorf("logs should now be offered: %s", m.renderFooter())
	}
}

func TestGuestLogsStopWhileDown(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	m, cmd := inspectedLogSystem(m, target.LogSystemJournal)
	m = applyCmd(m, cmd)

	m = probed(m, "box", false)
	calls := backend.logCalls
	m = runCmd(m, peekCmd)
	if backend.logCalls != calls || strings.Contains(preview(m), "journal line") || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("a stopped target has no journal to read, %d calls:\n%s", backend.logCalls-calls, preview(m))
	}
}

func TestHostSideLogsSurviveDown(t *testing.T) {
	m, backend := newTestModel()
	backend.logs["forge"] = "serial line"
	next, cmd := focus(sized(m), "forge").Update(launcher.Checked{ID: configID("forge"), Full: true, Status: target.StatusDown})
	m = applyCmd(next.(model), cmd)
	if !strings.Contains(preview(m), "serial line") || !strings.Contains(m.renderFooter(), "logs") {
		t.Errorf("host-side logs do not need the target up:\n%s", preview(m))
	}
}

func TestNoLogsOffersNothing(t *testing.T) {
	m, backend := focusedOnGuestLogs()
	box := m.selected().target
	box.Logs = target.LogSource{}
	m = focus(withTargets(m, box), "box")
	m, cmd := inspectedLogSystem(m, target.LogSystemJournal)
	m = applyCmd(m, cmd)
	if backend.logCalls != 0 || strings.Contains(m.renderFooter(), "logs") {
		t.Errorf(`logs: "" reads nothing: %d calls, footer %s`, backend.logCalls, m.renderFooter())
	}
}

func TestHiddenSessionsGiveTheirLinesToLogs(t *testing.T) {
	m := focusedPreview(t, target.Session{Name: "one"}, target.Session{Name: "two"})
	m = withLogs(m, configID("forge"), strings.TrimSuffix(strings.Repeat("log line\n", 40), "\n"))

	logLines := func(m model) int { return strings.Count(ansi.Strip(m.previewBody(60, 20)), "log line") }
	if with, without := logLines(m), logLines(key(m, "tab")); without <= with {
		t.Errorf("want more log lines once sessions are hidden, got %d with and %d without", with, without)
	}
}

func TestLogsShrinkToFitSessions(t *testing.T) {
	m := focusedPreview(t, target.Session{Name: "one"}, target.Session{Name: "two"})
	m = withLogs(m, configID("forge"), strings.Repeat("old line\n", 30)+"newest line")

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
