package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jazho76/vms/internal/lima"
	"github.com/jazho76/vms/internal/profiles"
)

func newTestModel() model {
	m := model{
		self:     "/tmp/vms",
		profiles: []profiles.Profile{{Name: "forge"}, {Name: "tokyo"}},
		spinner:  spinner.New(),
		input:    textinput.New(),
		host:     hostInfo{name: "testhost", os: "Linux", uptime: "up 1 hour"},
		tasks:    map[string]string{},
	}
	m.rebuild(nil)
	return m
}

func sized(m model) model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 24})
	return next.(model)
}

func TestRebuildOrdersHostFirst(t *testing.T) {
	m := newTestModel()
	if len(m.items) != 3 {
		t.Fatalf("want 3 items (host + 2 vms), got %d", len(m.items))
	}
	if m.items[0].kind != kindHost {
		t.Fatalf("first item should be host")
	}
	if m.items[1].name != "forge" || m.items[2].name != "tokyo" {
		t.Fatalf("unexpected vm order: %q %q", m.items[1].name, m.items[2].name)
	}
}

func TestLoadedMsgSetsStatus(t *testing.T) {
	m := sized(newTestModel())
	loaded := loadedMsg{instances: map[string]lima.Instance{
		"forge": {Name: "forge", Status: "Running", CPUs: "6"},
	}}
	next, _ := m.Update(loaded)
	m = next.(model)
	m.cursor = 1

	forge := m.items[1]
	if !forge.running() {
		t.Fatalf("forge should be running, got status %q", forge.status)
	}
	if m.items[2].status != "" {
		t.Fatalf("tokyo should be uncreated, got %q", m.items[2].status)
	}

	view := m.View()
	for _, want := range []string{"forge", "tokyo", "host", "running"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}

	m.cursor = 0
	if !strings.Contains(m.View(), "testhost") {
		t.Errorf("host preview missing hostname")
	}
}

func TestCursorBounds(t *testing.T) {
	m := sized(newTestModel())

	m = key(m, "up")
	if m.cursor != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", m.cursor)
	}

	for i := 0; i < 10; i++ {
		m = key(m, "down")
	}
	if m.cursor != len(m.items)-1 {
		t.Fatalf("cursor should clamp at %d, got %d", len(m.items)-1, m.cursor)
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	m := sized(newTestModel())
	m = applyLoaded(m, "forge", "Running")
	m.cursor = 1

	m = key(m, "ctrl+x")
	if m.mode != modeConfirmDelete {
		t.Fatalf("ctrl+x on a created vm should enter confirm mode")
	}

	m.input.SetValue("nope")
	m = key(m, "enter")
	if m.mode != modeNormal || m.status != "aborted" {
		t.Fatalf("mismatched name should abort, got mode=%v status=%q", m.mode, m.status)
	}
}

func TestDeleteRequiresCreated(t *testing.T) {
	m := sized(newTestModel())
	m.cursor = 2
	m = key(m, "ctrl+x")
	if m.mode != modeNormal {
		t.Fatalf("ctrl+x on an uncreated vm should be a no-op")
	}
}

func TestLogsModeToggle(t *testing.T) {
	m := sized(newTestModel())
	m = applyLoaded(m, "forge", "Running")
	m.cursor = 1

	m = key(m, "ctrl+l")
	if m.mode != modeLogs {
		t.Fatalf("ctrl+l on a created vm should open the log pager")
	}
	if m.logName != "forge" {
		t.Fatalf("log pager should target forge, got %q", m.logName)
	}

	m = key(m, "esc")
	if m.mode != modeNormal {
		t.Fatalf("esc should close the log pager")
	}
}

func TestLogsRequiresCreated(t *testing.T) {
	m := sized(newTestModel())
	m.cursor = 2
	m = key(m, "ctrl+l")
	if m.mode != modeNormal {
		t.Fatalf("ctrl+l on an uncreated vm should be a no-op")
	}
}

func TestTerminalTooSmall(t *testing.T) {
	m := newTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m = next.(model)
	if !strings.Contains(m.View(), "too small") {
		t.Fatalf("sub-minimum terminal should show the too-small notice")
	}
}

func TestViewWithinBounds(t *testing.T) {
	sizes := []struct{ w, h int }{{40, 14}, {80, 24}, {120, 40}, {52, 16}}
	for _, sz := range sizes {
		m := newTestModel()
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m = next.(model)
		m = applyLoaded(m, "forge", "Running")
		m.cursor = 1

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

func TestConcurrentTaskGuards(t *testing.T) {
	m := sized(newTestModel())
	m = applyLoaded(m, "forge", "Running")
	m.cursor = 1

	m = key(m, "ctrl+r")
	if m.tasks["forge"] != verbRestart {
		t.Fatalf("ctrl+r should mark forge restarting, got %q", m.tasks["forge"])
	}
	m = key(m, "ctrl+s")
	if m.tasks["forge"] != verbRestart {
		t.Fatalf("a second action on a busy VM must not change its task")
	}

	view := m.View()
	if !strings.Contains(view, verbRestart) {
		t.Errorf("list should show the inline %q verb", verbRestart)
	}
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 90 {
			t.Errorf("inline task overran width: %d", w)
		}
	}

	next, _ := m.Update(actionMsg{name: "forge", status: "restarted forge"})
	m = next.(model)
	if m.hasTask("forge") {
		t.Fatalf("actionMsg should clear the task")
	}
}

func applyLoaded(m model, name, status string) model {
	next, _ := m.Update(loadedMsg{instances: map[string]lima.Instance{
		name: {Name: name, Status: status},
	}})
	return next.(model)
}

func key(m model, s string) model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	if special, ok := specialKeys[s]; ok {
		next, _ = m.Update(tea.KeyMsg{Type: special})
	}
	return next.(model)
}

var specialKeys = map[string]tea.KeyType{
	"up":     tea.KeyUp,
	"down":   tea.KeyDown,
	"enter":  tea.KeyEnter,
	"esc":    tea.KeyEsc,
	"ctrl+x": tea.KeyCtrlX,
	"ctrl+l": tea.KeyCtrlL,
	"ctrl+r": tea.KeyCtrlR,
	"ctrl+s": tea.KeyCtrlS,
}
