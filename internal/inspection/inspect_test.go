package inspection

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/jazho76/uplink/internal/inspection/multiplexer"
	"github.com/jazho76/uplink/internal/target"
)

var uptime = target.Metric{Type: target.TypeText, Label: "up", Value: "echo 42"}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "uplink-inspect")
	if err != nil {
		panic(err)
	}
	for _, m := range multiplexer.All {
		if err := os.WriteFile(filepath.Join(dir, m.Tool()), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			panic(err)
		}
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.Setenv("SHELL", filepath.Join(dir, "no-login-shell"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const tmuxStub = `case "$1" in
list-sessions) echo 'session $0 1 work' ;;
list-windows) echo 'window $0 2 my editor'; echo 'window $0 1 logs' ;;
esac`

func TestParseReadsEverySection(t *testing.T) {
	out := "0 value 42\ntool tmux\nmux tmux session $1 1 main\nmux tmux window $1 2 my editor\n"
	snapshot := Parse(out, target.StatsPolicy{Metrics: []target.Metric{uptime}})

	if len(snapshot.Readings) != 1 || snapshot.Readings[0].Text != "42" {
		t.Errorf("want the metric read alongside the rest, got %+v", snapshot.Readings)
	}
	if !slices.Equal(snapshot.Tools, []string{"tmux"}) {
		t.Errorf("want tmux reported, got %q", snapshot.Tools)
	}
	want := []target.Session{{Tool: "tmux", Name: "main", Attached: true, Windows: []target.Window{{Name: "my editor", Panes: 2}}}}
	if !reflect.DeepEqual(snapshot.Sessions, want) {
		t.Errorf("want %+v, got %+v", want, snapshot.Sessions)
	}
}

func TestBannerCannotProduceSessions(t *testing.T) {
	snapshot := Parse("session $0 0 fake\nwindow $0 1 fake\nmux\nmux tmux\n", target.StatsPolicy{})
	if len(snapshot.Sessions) != 0 {
		t.Errorf("only rows tagged by the core count, got %+v", snapshot.Sessions)
	}
}

func TestCoreStripsEscapesFromNames(t *testing.T) {
	out := "mux tmux session $0 0 \x1b[31mred\x1b[0m\nmux tmux window $0 1 \x1b[32mgreen\x1b[0m\n" +
		"mux tmux session $1 0 \x1b[31m\x1b[0m\nmux tmux window $0 1 \x1b[0m\n"
	want := []target.Session{{Tool: "tmux", Name: "red", Windows: []target.Window{{Name: "green", Panes: 1}}}}
	if got := Parse(out, target.StatsPolicy{}).Sessions; !reflect.DeepEqual(got, want) {
		t.Errorf("a target must not be able to style the dashboard, want %+v, got %+v", want, got)
	}
}

type fakeMultiplexer struct {
	tool   string
	script string
}

func (f fakeMultiplexer) Tool() string   { return f.tool }
func (f fakeMultiplexer) Attach() string { return f.tool }
func (f fakeMultiplexer) Script() string { return f.script }
func (f fakeMultiplexer) Parse(rows []string) []target.Session {
	sessions := make([]target.Session, len(rows))
	for i, row := range rows {
		sessions[i].Name = row
	}
	return sessions
}

func withMultiplexers(t *testing.T, all ...multiplexer.Multiplexer) {
	t.Helper()
	previous := multiplexer.All
	t.Cleanup(func() { multiplexer.All = previous })
	multiplexer.All = all
}

func TestAbsentToolDoesNotRunItsScript(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	withMultiplexers(t, fakeMultiplexer{tool: "uplink-absent-tool", script: ": > " + marker})

	if snapshot := inspectLocally(t); len(snapshot.Sessions) != 0 {
		t.Errorf("an absent tool has no sessions, got %+v", snapshot.Sessions)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the script of an absent tool must not run")
	}
}

func TestScriptCannotConsumeTheInspection(t *testing.T) {
	pathWith(t, map[string]string{"greedy": "exit 0", "polite": "exit 0"})
	withMultiplexers(t,
		fakeMultiplexer{tool: "greedy", script: "cat >/dev/null; echo first"},
		fakeMultiplexer{tool: "polite", script: "echo second"},
	)

	got := inspectLocally(t).Sessions
	want := []target.Session{{Tool: "greedy", Name: "first"}, {Tool: "polite", Name: "second"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestTaggedKeepsOnlyItsOwnLines(t *testing.T) {
	got := tagged("banner\ntool tmux\ntools are fun\nsession 1 0 tool x\ntool two words\n", "tool")
	if want := []string{"tmux", "two words"}; !slices.Equal(got, want) {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFailingSectionDoesNotFailTheInspection(t *testing.T) {
	previous := sections
	t.Cleanup(func() { sections = previous })
	sections = append(slices.Clone(previous), section{
		script: func(target.StatsPolicy) string { return "false" },
		parse:  func(target.StatsPolicy, string, *target.Environment) {},
	})

	inspectLocally(t)
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

func inspectLocally(t *testing.T) target.Environment {
	t.Helper()
	snapshot, err := Run(10*time.Second, target.StatsPolicy{Metrics: []target.Metric{uptime}}, []string{"sh"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if len(snapshot.Readings) != 1 || snapshot.Readings[0].Text != "42" {
		t.Errorf("stats should come back whatever tmux does, got %+v", snapshot.Readings)
	}
	return snapshot
}

func TestRunReportsSessions(t *testing.T) {
	pathWith(t, map[string]string{"tmux": tmuxStub})

	snapshot := inspectLocally(t)
	if !slices.Contains(snapshot.Tools, "tmux") {
		t.Errorf("want tmux reported, got %q", snapshot.Tools)
	}
	want := []target.Session{{Tool: "tmux", Name: "work", Attached: true, Windows: []target.Window{{Name: "my editor", Panes: 2}, {Name: "logs", Panes: 1}}}}
	if !reflect.DeepEqual(snapshot.Sessions, want) {
		t.Errorf("want %+v, got %+v", want, snapshot.Sessions)
	}
}

const zellijStub = `cat >/dev/null
case "$*" in
list-sessions*) echo 'my beta'; echo alpha ;;
*list-clients*) echo 'CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND'; [ "$2" = alpha ] && echo '1 terminal_0 N/A' ;;
*list-panes*) printf '[{"tab_id":0,"tab_position":0,"tab_name":"tab of %s"}]\n' "$2" ;;
esac`

func TestRunReportsZellijSessions(t *testing.T) {
	pathWith(t, map[string]string{"zellij": zellijStub})

	snapshot := inspectLocally(t)
	if !slices.Contains(snapshot.Tools, "zellij") {
		t.Errorf("want zellij reported, got %q", snapshot.Tools)
	}
	want := []target.Session{
		{Tool: "zellij", Name: "alpha", Attached: true, Windows: []target.Window{{Name: "tab of alpha", Panes: 1}}},
		{Tool: "zellij", Name: "my beta", Windows: []target.Window{{Name: "tab of my beta", Panes: 1}}},
	}
	if !reflect.DeepEqual(snapshot.Sessions, want) {
		t.Errorf("want %+v, got %+v", want, snapshot.Sessions)
	}
}

func TestRunSurvivesAZellijWithNoSessions(t *testing.T) {
	pathWith(t, map[string]string{"zellij": "echo 'No active zellij sessions found.' >&2\nexit 1"})

	snapshot := inspectLocally(t)
	if !slices.Contains(snapshot.Tools, "zellij") {
		t.Errorf("zellij is installed even with no sessions, got %q", snapshot.Tools)
	}
	if len(snapshot.Sessions) != 0 {
		t.Errorf("want no sessions, got %+v", snapshot.Sessions)
	}
}

func TestRunReportsEachToolsOwnSessions(t *testing.T) {
	pathWith(t, map[string]string{"tmux": tmuxStub, "zellij": zellijStub})

	var got []string
	for _, s := range inspectLocally(t).Sessions {
		got = append(got, s.Tool+"/"+s.Name)
	}
	if want := []string{"tmux/work", "zellij/alpha", "zellij/my beta"}; !slices.Equal(got, want) {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestRunReportsTheJournal(t *testing.T) {
	pathWith(t, map[string]string{target.ToolJournal: "exit 0"})
	if snapshot := inspectLocally(t); !slices.Contains(snapshot.Tools, target.ToolJournal) {
		t.Errorf("want journalctl reported, got %q", snapshot.Tools)
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

func builtinLabels(t *testing.T, kernel string) target.Environment {
	t.Helper()
	pathWith(t, map[string]string{"uname": "echo " + kernel, "tmux": tmuxStub})
	snapshot, err := Run(30*time.Second, target.StatsPolicy{Builtin: true}, []string{"sh"})
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if snapshot.OS != kernel {
		t.Errorf("want %s reported, got %q", kernel, snapshot.OS)
	}
	return snapshot
}

func labels(readings []target.Reading) []string {
	names := make([]string, len(readings))
	for i, r := range readings {
		names[i] = r.Metric.Label
	}
	return names
}

func metricLabels(metrics []target.Metric) []string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.Label
	}
	return names
}

func TestBuiltinStatsFollowTheKernel(t *testing.T) {
	for _, kernel := range []string{"Linux", "Darwin"} {
		t.Run(kernel, func(t *testing.T) {
			snapshot := builtinLabels(t, kernel)
			want := metricLabels(builtinMetrics(kernel))
			if len(want) == 0 || !slices.Equal(labels(snapshot.Readings), want) {
				t.Errorf("want %q, got %q", want, labels(snapshot.Readings))
			}
		})
	}
}

func TestBuiltinStatsReadThisLinuxMachine(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("no linux here: %s", runtime.GOOS)
	}
	snapshot := builtinLabels(t, "Linux")
	for _, r := range snapshot.Readings {
		if !r.OK {
			t.Errorf("%s gave no reading", r.Metric.Label)
		}
	}
}

func TestUnknownKernelHasNoStatsButTheRestStillWorks(t *testing.T) {
	snapshot := builtinLabels(t, "Plan9")
	if len(snapshot.Readings) != 0 {
		t.Errorf("want no readings, got %+v", snapshot.Readings)
	}
	if !slices.Contains(snapshot.Tools, "tmux") || len(snapshot.Sessions) != 1 {
		t.Errorf("tools and sessions do not depend on the kernel, got %+v", snapshot)
	}
}

func TestDeclaredStatsIgnoreTheKernel(t *testing.T) {
	pathWith(t, map[string]string{"uname": "echo Plan9"})
	snapshot := inspectLocally(t)
	if snapshot.OS != "Plan9" {
		t.Errorf("the OS is still reported, got %q", snapshot.OS)
	}
}

func TestBannerCannotOverrideTheKernel(t *testing.T) {
	snapshot := Parse("os Linux and friends\nos Darwin\n", target.StatsPolicy{Builtin: true})
	if snapshot.OS != "Darwin" {
		t.Errorf("the script's line comes last and wins, got %q", snapshot.OS)
	}
}

func loginShellPrinting(t *testing.T, output string) {
	t.Helper()
	shell := filepath.Join(t.TempDir(), "login-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\ncat <<'EOF'\n"+output+"\nEOF\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
}

func toolOnlyOnTheLoginPath(t *testing.T) string {
	t.Helper()
	const tool = "uplink-login-tool"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	withMultiplexers(t, fakeMultiplexer{tool: tool, script: "echo found"})
	return dir
}

func TestRunFindsToolsOnTheLoginPath(t *testing.T) {
	dir := toolOnlyOnTheLoginPath(t)
	for name, output := range map[string]string{
		"only the path":  dir,
		"after a banner": "Welcome to the machine\n" + dir,
	} {
		t.Run(name, func(t *testing.T) {
			loginShellPrinting(t, output)

			snapshot := inspectLocally(t)
			if !slices.Contains(snapshot.Tools, "uplink-login-tool") {
				t.Errorf("want the tool reported, got %q", snapshot.Tools)
			}
			want := []target.Session{{Tool: "uplink-login-tool", Name: "found"}}
			if !reflect.DeepEqual(snapshot.Sessions, want) {
				t.Errorf("want %+v, got %+v", want, snapshot.Sessions)
			}
		})
	}
}

func TestRunIgnoresAGarbageLoginPath(t *testing.T) {
	toolOnlyOnTheLoginPath(t)
	loginShellPrinting(t, "not a path")

	if snapshot := inspectLocally(t); slices.Contains(snapshot.Tools, "uplink-login-tool") {
		t.Errorf("want the tool missing, got %q", snapshot.Tools)
	}
}

func TestLoginPathCannotHideWhatIsAlreadyFound(t *testing.T) {
	pathWith(t, map[string]string{"tmux": tmuxStub})
	shadow := t.TempDir()
	if err := os.WriteFile(filepath.Join(shadow, "tmux"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	loginShellPrinting(t, shadow)

	if snapshot := inspectLocally(t); len(snapshot.Sessions) != 1 {
		t.Errorf("want the tmux already on PATH used, got %+v", snapshot.Sessions)
	}
}

func TestRunNeedsACommand(t *testing.T) {
	if _, err := Run(time.Second, target.StatsPolicy{}, nil); err == nil {
		t.Error("no argv cannot run")
	}
}
