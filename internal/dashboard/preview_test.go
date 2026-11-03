package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

func preview(m model) string { return ansi.Strip(m.previewBody(60, 30)) }

func TestPreviewHeaderNamesTheMode(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	header := func(m model) string { return strings.SplitN(ansi.Strip(m.previewBody(60, 30)), "\n", 2)[0] }

	m = focus(m, "forge")
	if got := header(m); !strings.Contains(got, "tmux") || !strings.Contains(got, "1/") {
		t.Fatalf("the header should name the selected mode: %q", got)
	}
	if got := header(key(m, "tab")); !strings.Contains(got, "2/") {
		t.Errorf("the header mode should follow tab: %q", got)
	}
}

func TestModeSurfacedInListAndPreview(t *testing.T) {
	m, _ := newTestModel()
	m = sized(m)
	m = focus(m, "forge")

	if strings.Contains(m.renderPanes(30, 12), "[tmux]") {
		t.Errorf("the default mode should stay out of the list row")
	}
	if !strings.Contains(m.renderFooter(), "tab") {
		t.Errorf("footer should advertise tab for a multi-mode target")
	}

	m = key(m, "tab")
	if !strings.Contains(m.renderPanes(30, 12), "[shell]") {
		t.Errorf("an off-default mode should be marked on the row: %s", m.renderPanes(30, 12))
	}
	preview := m.previewBody(50, 20)
	for _, want := range []string{"shell", "2/3"} {
		if !strings.Contains(preview, want) {
			t.Errorf("preview missing %q: %s", want, preview)
		}
	}
}

func TestPreviewExplainsUnknown(t *testing.T) {
	m := focus(sized(func() model { m, _ := newTestModel(); return m }()), "forge")
	next, _ := m.Update(launcher.Checked{ID: configID("forge"), Status: target.StatusUnknown, Reason: "Permission denied\x1b[31m"})
	m = next.(model)
	if got := preview(m); !strings.Contains(got, "Permission denied") {
		t.Errorf("want the reason, got:\n%s", got)
	}
	if got := next.(model).state.Observation(configID("forge")).Reason; got != "Permission denied" {
		t.Errorf("a target must not be able to style the dashboard through it, got %q", got)
	}
	next, _ = m.Update(launcher.Checked{ID: configID("forge"), Status: target.StatusDown})
	if got := preview(next.(model)); strings.Contains(got, "Permission denied") {
		t.Errorf("a known status needs no explaining, got:\n%s", got)
	}
}

func TestPreviewShowsConnectingWithAWayOut(t *testing.T) {
	m, _ := waitingOnStartable(t)
	if got := preview(m); !strings.Contains(got, "connecting") || !strings.Contains(got, "cancel") {
		t.Errorf("want connecting and its hint, got:\n%s", got)
	}
}

func modeTagOf(m model) string { return ansi.Strip(m.modeTag(m.selected())) }

func wentDown(m model, name string) model {
	next, _ := m.Update(launcher.Checked{ID: configID(name), Status: target.StatusDown})
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
	m := probed(focusedOnAutomaticModes("tmux", "zellij", "shell"), "box", false)
	if got := modeTagOf(m); !strings.Contains(got, "tmux?") || !strings.Contains(got, "1/2") {
		t.Errorf("want tmux? 1/2, got %q", got)
	}
}

func TestUpTargetStillBeingInspectedShowsNoDoubt(t *testing.T) {
	m, _ := focusedOn("box")
	next, _ := m.Update(launcher.Checked{ID: configID("box"), Status: target.StatusUp})
	if got := modeTagOf(next.(model)); !strings.Contains(got, "tmux") || strings.Contains(got, "?") {
		t.Errorf("want plain tmux while up, got %q", got)
	}
}

func TestConfirmedModeStaysCertainOnceDown(t *testing.T) {
	m, _ := focusedOn("box")
	m = wentDown(inspected(m, "box", "tmux"), "box")
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
