package inspect

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/stats"
)

var uptime = stats.Metric{Type: stats.TypeText, Label: "up", Value: "echo 42"}

func TestParseReadsEverySection(t *testing.T) {
	out := "0 value 42\ntool tmux\nsession 3 1 main\nsession 1 0 my long name\n"
	snapshot := Parse(out, []stats.Metric{uptime})

	if len(snapshot.Readings) != 1 || snapshot.Readings[0].Text != "42" {
		t.Errorf("want the metric read alongside the rest, got %+v", snapshot.Readings)
	}
	if !slices.Equal(snapshot.Tools, []string{"tmux"}) {
		t.Errorf("want tmux reported, got %q", snapshot.Tools)
	}
	want := []Session{
		{Name: "main", Windows: 3, Attached: true},
		{Name: "my long name", Windows: 1},
	}
	if !slices.Equal(snapshot.Sessions, want) {
		t.Errorf("want %+v, got %+v", want, snapshot.Sessions)
	}
}

func TestParseStripsEscapesFromSessionNames(t *testing.T) {
	snapshot := Parse("session 1 0 \x1b[31mred\x1b[0m\n", nil)
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Name != "red" {
		t.Errorf("a target must not be able to style the dashboard, got %+v", snapshot.Sessions)
	}
}

func TestParseSkipsWhatItCannotRead(t *testing.T) {
	snapshot := Parse("motd banner\nsession\nsession x 0 name\nsession 1 y name\nsession 1 0\n", nil)
	if len(snapshot.Sessions) != 0 || len(snapshot.Tools) != 0 {
		t.Errorf("malformed lines should be dropped, got %+v", snapshot)
	}
}

func pathWith(t *testing.T, stubs map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func inspectLocally(t *testing.T) Snapshot {
	t.Helper()
	snapshot, err := Run(10*time.Second, []stats.Metric{uptime}, []string{"sh"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(snapshot.Readings) != 1 || snapshot.Readings[0].Text != "42" {
		t.Errorf("stats should come back whatever tmux does, got %+v", snapshot.Readings)
	}
	return snapshot
}

func TestRunReportsSessions(t *testing.T) {
	pathWith(t, map[string]string{"tmux": `echo "session 2 1 work"`})

	snapshot := inspectLocally(t)
	if !slices.Contains(snapshot.Tools, "tmux") {
		t.Errorf("want tmux reported, got %q", snapshot.Tools)
	}
	if want := []Session{{Name: "work", Windows: 2, Attached: true}}; !slices.Equal(snapshot.Sessions, want) {
		t.Errorf("want %+v, got %+v", want, snapshot.Sessions)
	}
}

func TestRunSurvivesATmuxWithNoServer(t *testing.T) {
	pathWith(t, map[string]string{"tmux": "echo 'no server running' >&2\nexit 1"})

	snapshot := inspectLocally(t)
	if !slices.Contains(snapshot.Tools, "tmux") {
		t.Errorf("tmux is installed even with no server, got %q", snapshot.Tools)
	}
	if len(snapshot.Sessions) != 0 {
		t.Errorf("no server means no sessions, got %+v", snapshot.Sessions)
	}
}

func TestRunSurvivesAMissingTmux(t *testing.T) {
	dir := t.TempDir()
	for _, tool := range []string{"sh", "head"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(dir, tool)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)

	snapshot := inspectLocally(t)
	if len(snapshot.Tools) != 0 || len(snapshot.Sessions) != 0 {
		t.Errorf("want no tmux and no sessions, got %+v", snapshot)
	}
}

func TestRunNeedsACommand(t *testing.T) {
	if _, err := Run(time.Second, nil, nil); err == nil {
		t.Error("no argv cannot run")
	}
}
