package target

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/inspection/multiplexer"
	"github.com/jazho76/uplink/internal/inspection/stats"
)

const host = `
  - name: host
    modes:
      - { name: shell }
`

func load(t *testing.T, body string) ([]Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return loadConfig(path)
}

func loadConfig(path string) ([]Target, error) {
	doc, err := Read(path)
	if err != nil {
		return nil, err
	}
	return Merge(doc.Config())
}

func mustLoad(t *testing.T, body string) []Target {
	t.Helper()
	targets, err := load(t, body)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return targets
}

func find(t *testing.T, targets []Target, name string) Target {
	t.Helper()
	i := slices.IndexFunc(targets, func(found Target) bool { return found.Name == name })
	if i < 0 {
		t.Fatalf("no target %q", name)
	}
	return targets[i]
}

func modeNames(target Target) []string {
	names := make([]string, 0, len(target.Modes))
	for _, m := range target.Modes {
		names = append(names, m.Name)
	}
	return names
}

func modeNamed(t *testing.T, target Target, name string) Mode {
	t.Helper()
	i := slices.IndexFunc(target.Modes, func(m Mode) bool { return m.Name == name })
	if i < 0 {
		t.Fatalf("%s has no mode %q", target.Name, name)
	}
	return target.Modes[i]
}

func TestShellWords(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{in: "ssh -t box", want: []string{"ssh", "-t", "box"}},
		{in: "  ssh\t box  ", want: []string{"ssh", "box"}},
		{in: `ssh -o 'ProxyCommand=nc %h %p' box`, want: []string{"ssh", "-o", "ProxyCommand=nc %h %p", "box"}},
		{in: `docker exec "my box" sh -c`, want: []string{"docker", "exec", "my box", "sh", "-c"}},
		{in: `a\ b 'it'\''s' ""`, want: []string{"a b", "it's", ""}},
		{in: `"proxy\:1080" "say \"hi\" \\ \$HOME"`, want: []string{`proxy\:1080`, `say "hi" \ $HOME`}},
		{in: "", want: nil},
	} {
		got, err := shellWords(c.in)
		if err != nil {
			t.Errorf("shellWords(%q): %v", c.in, err)
			continue
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("shellWords(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, broken := range []string{`ssh 'box`, `ssh "box`, `ssh box\`} {
		if _, err := shellWords(broken); err == nil {
			t.Errorf("shellWords(%q) should reject the unterminated input", broken)
		}
	}
}

func TestEmptyConfigHasNoTargets(t *testing.T) {
	for name, body := range map[string]string{
		"empty file":   "",
		"only anchors": "x-vm: { via: ssh vm }\n",
	} {
		if targets, err := load(t, body); err != nil || len(targets) != 0 {
			t.Errorf("%s: want no targets and no error, got %v and %v", name, targets, err)
		}
	}
	if targets, err := loadConfig(filepath.Join(t.TempDir(), "missing.yaml")); err != nil || len(targets) != 0 {
		t.Errorf("a missing file gives no targets and no error, got %v and %v", targets, err)
	}
}

func TestConfigTargetsAreIdentifiedByName(t *testing.T) {
	got := find(t, mustLoad(t, "targets:"+host), "host").ID
	if want := (ID{Source: ConfigSource, Key: "host"}); got != want {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestDiscoverAbsentDiffersFromEmpty(t *testing.T) {
	for body, wantNil := range map[string]bool{"targets: []\n": true, "discover: []\n": false} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		doc, err := Read(path)
		if err != nil || (doc.Discover == nil) != wantNil {
			t.Errorf("%q: want nil discover %v, got %#v and %v", body, wantNil, doc.Discover, err)
		}
	}
}

func TestDefaultsFillWhatFactsLeaveOut(t *testing.T) {
	discovered := fakeSource{name: "lima", specs: []Spec{{
		Name:     "dev",
		Display:  "Dev box",
		Group:    "work",
		Defaults: &Spec{Group: "vms", Logs: new("tail {name}.log"), Modes: shellOnly},
	}}}

	targets, err := Merge(discovered)
	if err != nil || len(targets) != 1 {
		t.Fatalf("want one target, got %v and %v", targets, err)
	}
	got := targets[0]
	if got.Group != "work" || got.Logs != "tail dev.log" {
		t.Errorf("facts beat defaults, which still fill the rest, got %q and %q", got.Group, got.Logs)
	}
	if got.Name != "Dev box" || got.ID.Key != "dev" {
		t.Errorf("display changes the name only, got %q keyed %q", got.Name, got.ID.Key)
	}
}

func TestIDsPrintQualifiedOutsideConfig(t *testing.T) {
	for _, c := range []struct {
		id   ID
		want string
	}{
		{ID{Source: ConfigSource, Key: "prod"}, "prod"},
		{ID{Source: "lima", Key: "default"}, "lima/default"},
	} {
		if got := c.id.String(); got != c.want {
			t.Errorf("%+v printed %q, want %q", c.id, got, c.want)
		}
	}
}

type fakeSource struct {
	name  string
	specs []Spec
	err   error
}

func (f fakeSource) Name() string           { return f.name }
func (f fakeSource) Specs() ([]Spec, error) { return f.specs, f.err }

var shellOnly = []ModeSpec{{Name: "shell"}}

func TestSourcesMergeInOrderWithDistinctIDs(t *testing.T) {
	config := configSource{{Name: "box", Modes: shellOnly}}
	lima := fakeSource{name: "lima", specs: []Spec{{Name: "box", Modes: shellOnly}, {Name: "dev", Modes: shellOnly}}}

	targets, err := Merge(config, lima)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, target := range targets {
		ids = append(ids, target.ID.String())
	}
	if got := strings.Join(ids, " "); got != "box lima/box lima/dev" {
		t.Errorf("want config first, then lima in its order, got %q", got)
	}
}

func TestFailingSourceOnlyLosesItsOwnTargets(t *testing.T) {
	config := configSource{{Name: "host", Modes: shellOnly}}
	lima := fakeSource{name: "lima", err: errors.New("limactl not found")}

	targets, err := Merge(config, lima)
	if len(targets) != 1 || targets[0].Name != "host" {
		t.Errorf("config targets should survive, got %v", targets)
	}
	if err == nil || err.Error() != "lima: limactl not found" {
		t.Errorf("want the failure prefixed with its source, got %v", err)
	}
}

func TestDuplicateSourcesAndBrokenSpecsAreReported(t *testing.T) {
	first := fakeSource{name: "lima", specs: []Spec{{Name: "x", Modes: []ModeSpec{{}}}, {Modes: shellOnly}}}
	second := fakeSource{name: "lima", specs: []Spec{{Name: "y", Modes: shellOnly}}}

	targets, err := Merge(first, second)
	if len(targets) != 0 {
		t.Errorf("want no targets, got %v", targets)
	}
	for _, want := range []string{`target "lima/x": mode 1 has no name`, "target lima #2: no name", `duplicate source "lima"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("errors should mention %q, got %v", want, err)
		}
	}
}

func TestMergedAnchorFillsWhatTheTargetLeavesOut(t *testing.T) {
	targets := mustLoad(t, `
x-lima: &lima
    group: vms
    watch: true
    via: ssh -F ~/.lima/{name}/ssh.config lima-{name}
    logs: tail ~/.lima/{name}/serial.log
    stats: [{ type: text, label: up, unit: seconds, value: "uptime {name}" }, { type: gauge, label: jobs, value: "jobs {name}", max: 8 }]
    modes:
      - { name: tmux, run: "tmux new-session -A -s {name}" }
      - { name: shell }
targets:`+host+`
  - { <<: *lima, name: forge }
  - <<: *lima
    name: tokyo
    logs: echo own
    stats: []
    modes:
      - { name: top, run: htop }
`)
	home, _ := os.UserHomeDir()

	forge := find(t, targets, "forge")
	if forge.Group != "vms" || !forge.Watch || len(forge.Stats) != 2 {
		t.Errorf("forge should inherit group, watch and stats, got %+v", forge)
	}
	if got := forge.Stats[0].Value + ", " + forge.Stats[1].Value; got != "uptime forge, jobs forge" {
		t.Errorf("metric commands should interpolate the name, got %q", got)
	}
	wantVia := []string{"ssh", "-F", filepath.Join(home, ".lima/forge/ssh.config"), "lima-forge"}
	if !slices.Equal(forge.Via, wantVia) {
		t.Errorf("via = %q, want %q", forge.Via, wantVia)
	}
	if forge.Logs != "tail ~/.lima/forge/serial.log" {
		t.Errorf("logs should interpolate the name and leave ~ to the shell, got %q", forge.Logs)
	}
	if got := forge.Modes[0].Argv; !slices.Equal(got, append(wantVia, "tmux new-session -A -s forge")) {
		t.Errorf("tmux argv = %q", got)
	}
	if shell := modeNamed(t, forge, "shell"); !slices.Equal(shell.Argv, append(wantVia, loginShell)) {
		t.Errorf("a mode without run opens the user's shell, got %q", shell.Argv)
	}

	tokyo := find(t, targets, "tokyo")
	if tokyo.Logs != "echo own" || tokyo.Stats == nil || len(tokyo.Stats) != 0 {
		t.Errorf("target fields override the anchor, got logs %q stats %v", tokyo.Logs, tokyo.Stats)
	}
	if got := modeNames(tokyo); !slices.Equal(got, []string{"top"}) {
		t.Errorf("target modes replace the anchor's wholesale, got %v", got)
	}
	if top := modeNamed(t, tokyo, "top"); top.Argv[len(top.Argv)-1] != "htop" {
		t.Errorf("a target mode runs its own command, got %q", top.Argv)
	}
}

func TestTargetsWithoutViaRunHere(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: host
    modes:
      - { name: tmux, run: tmux new-session -A -s host }
      - { name: shell }
`)
	local := find(t, targets, "host")
	if got := local.Modes[0].Argv; !slices.Equal(got, []string{"sh", "-c", "tmux new-session -A -s host"}) {
		t.Errorf("tmux argv = %q", got)
	}
	if shell := modeNamed(t, local, "shell"); !slices.Equal(shell.Argv, []string{"sh", "-c", loginShell}) {
		t.Errorf("shell argv = %q", shell.Argv)
	}
	if local.Group != DefaultGroup || local.Watch || local.Stats != nil {
		t.Errorf("unset fields should take their defaults, got %+v", local)
	}
}

func TestExecDefaultsToVia(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: box
    via: docker exec -it box sh -c
    exec: docker exec -i box sh -c
    modes: [{ name: shell }]
  - name: plain
    via: ssh -t plain
    modes: [{ name: shell }]
`)
	if got := find(t, targets, "box").ExecArgvFor("true"); !slices.Equal(got, []string{"docker", "exec", "-i", "box", "sh", "-c", "true"}) {
		t.Errorf("exec should drive background commands, got %q", got)
	}
	if got := find(t, targets, "plain").ExecArgvFor("true"); !slices.Equal(got, []string{"ssh", "-t", "plain", "true"}) {
		t.Errorf("without exec, via drives background commands, got %q", got)
	}
}

func TestHooksInterpolateTheName(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: forge
    via: ssh lima-{name}
    status: limactl list {name}
    up: limactl start {name}
    down: limactl stop {name}
    modes: [{ name: shell }]
`)
	forge := find(t, targets, "forge")
	if forge.Status != "limactl list forge" || forge.Up != "limactl start forge" || forge.Down != "limactl stop forge" {
		t.Errorf("hooks should interpolate the name, got %+v", forge)
	}
}

func TestAnchorsShareValuesAcrossSections(t *testing.T) {
	targets := mustLoad(t, `
x-ssh: &ssh ssh -t shared
targets:
  - name: one
    via: *ssh
    modes: [{ name: shell }]
`)
	if got := find(t, targets, "one").Via; !slices.Equal(got, []string{"ssh", "-t", "shared"}) {
		t.Errorf("an alias defined in a scratch section should resolve, got %q", got)
	}
}

func TestModesDefaultWhenLeftOut(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: omitted
  - { name: empty, modes: [] }
  - { name: declared, modes: [{ name: tmux, run: tmux }] }
`)
	for _, name := range []string{"omitted", "empty"} {
		target := find(t, targets, name)
		if got := modeNames(target); !slices.Equal(got, []string{"tmux", "zellij", "shell"}) {
			t.Errorf("%s: want tmux, zellij, shell, got %v", name, got)
		}
		if got := modeNamed(t, target, "tmux").Requires; got != "tmux" {
			t.Errorf("%s: the default tmux mode needs tmux, got %q", name, got)
		}
		if got := modeNamed(t, target, "shell").Requires; got != "" {
			t.Errorf("%s: a shell needs nothing, got %q", name, got)
		}
	}

	declared := find(t, targets, "declared")
	if got := modeNames(declared); !slices.Equal(got, []string{"tmux"}) {
		t.Errorf("declared modes replace the defaults, got %v", got)
	}
	if got := modeNamed(t, declared, "tmux").Requires; got != "" {
		t.Errorf("a declared mode is never filtered, got %q", got)
	}
}

func TestRequiresCannotBeConfigured(t *testing.T) {
	_, err := load(t, "targets:\n  - { name: box, modes: [{ name: a, requires: tmux }] }\n")
	if err == nil || !strings.Contains(err.Error(), "requires") {
		t.Errorf("requires belongs to the core, got %v", err)
	}
}

func TestDefaultTmuxModeFallsBackToTheShell(t *testing.T) {
	const (
		tmux   = `echo "tmux $*"`
		zellij = `echo "zellij $*"`
	)
	for name, c := range map[string]struct {
		mode  string
		tools map[string]string
		want  string
	}{
		"with tmux":           {"tmux", map[string]string{"tmux": tmux}, "tmux new-session -A -s 0"},
		"without tmux":        {"tmux", nil, "user shell"},
		"with zellij":         {"zellij", map[string]string{"zellij": zellij}, "zellij attach -c 0"},
		"without zellij":      {"zellij", nil, "user shell"},
		"tmux ignores zellij": {"tmux", map[string]string{"zellij": zellij}, "user shell"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			sh, err := exec.LookPath("sh")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(sh, filepath.Join(dir, "sh")); err != nil {
				t.Fatal(err)
			}
			stubs := map[string]string{"user-shell": `[ "$1" = -lc ] && exec sh -c "$2"
echo "user shell"`}
			maps.Copy(stubs, c.tools)
			for stub, body := range stubs {
				if err := os.WriteFile(filepath.Join(dir, stub), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir)
			t.Setenv("SHELL", filepath.Join(dir, "user-shell"))

			targets := mustLoad(t, "targets:\n  - name: box\n")
			argv := modeNamed(t, find(t, targets, "box"), c.mode).Argv
			out, err := exec.Command(argv[0], argv[1:]...).Output()
			if err != nil {
				t.Fatalf("%q: %v", argv, err)
			}
			if got := strings.TrimSpace(string(out)); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestDefaultTmuxModeFindsToolsOnTheLoginPath(t *testing.T) {
	loginDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(loginDir, "tmux"), []byte("#!/bin/sh\necho \"tmux $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sh, filepath.Join(dir, "sh")); err != nil {
		t.Fatal(err)
	}
	userShell := filepath.Join(dir, "user-shell")
	body := "#!/bin/sh\nPATH=\"" + loginDir + ":$PATH\"\nexport PATH\n[ \"$1\" = -lc ] && exec sh -c \"$2\"\necho \"user shell\"\n"
	if err := os.WriteFile(userShell, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SHELL", userShell)

	argv := modeNamed(t, find(t, mustLoad(t, "targets:\n  - name: box\n"), "box"), "tmux").Argv
	out, err := exec.Command(argv[0], argv[1:]...).Output()
	if err != nil {
		t.Fatalf("%q: %v", argv, err)
	}
	if got, want := strings.TrimSpace(string(out)), "tmux new-session -A -s 0"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestShellModeStartsWithoutSHELLSet(t *testing.T) {
	t.Setenv("SHELL", "")
	t.Setenv("HOME", t.TempDir())

	argv := modeNamed(t, find(t, mustLoad(t, "targets:\n  - name: box\n"), "box"), "shell").Argv
	command := exec.Command(argv[0], argv[1:]...)
	command.Stdin = strings.NewReader("exit 0\n")
	if out, err := command.CombinedOutput(); err != nil {
		t.Errorf("%q: %v\n%s", argv, err, out)
	}
}

func TestModeCommandTravelsAsOneArgument(t *testing.T) {
	targets := mustLoad(t, `
targets:`+host+`
  - name: dojo
    via: ssh -t dojo
    modes:
      - { name: tmux, run: "bash -lc 'lc && tmux'" }
`)
	argv := find(t, targets, "dojo").Modes[0].Argv
	if want := []string{"ssh", "-t", "dojo", "bash -lc 'lc && tmux'"}; !slices.Equal(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
	if got := Quote(argv); got != `ssh -t dojo 'bash -lc '\''lc && tmux'\'''` {
		t.Errorf("the printed command must paste back into a shell, got %s", got)
	}
}

func TestInvalidTargetsAreSkippedAndReported(t *testing.T) {
	targets, err := load(t, `
targets:`+host+`
  - name: good
    via: ssh good
    modes: [{ name: shell }]
  - name: dupmodes
    via: ssh x
    modes: [{ name: a }, { name: a }]
  - name: badquote
    via: ssh 'x
    modes: [{ name: shell }]
  - name: badexec
    via: ssh x
    exec: ssh 'x
    modes: [{ name: shell }]
  - name: good
    via: ssh other
    modes: [{ name: shell }]
  - via: ssh x
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("every broken target must be reported")
	}
	for _, want := range []string{`duplicate mode "a"`,
		"badquote", `"badexec": exec`, `duplicate target "good"`, "#7"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q:\n%v", want, err)
		}
	}
	var names []string
	for _, target := range targets {
		names = append(names, target.Name)
	}
	if !slices.Equal(names, []string{"host", "good"}) {
		t.Errorf("valid targets must survive, got %v", names)
	}
	if find(t, targets, "good").Via[1] != "good" {
		t.Errorf("the first claim on a name wins")
	}
}

func TestMalformedNamesAndViaAreRejected(t *testing.T) {
	_, err := load(t, `
targets:`+host+`
  - name: "db:prod"
    via: ssh db
    modes: [{ name: shell }]
  - name: "my box"
    via: ssh box
    modes: [{ name: shell }]
  - name: blank
    via: "   "
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("malformed targets must be reported")
	}
	for _, want := range []string{`"db:prod"`, `"my box"`, `"blank": via is blank`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s:\n%v", want, err)
		}
	}
}

func TestLogsDropEscapesAndKeepTheTail(t *testing.T) {
	out := System{}.Logs(Target{Logs: `printf 'one\n\033[31mtwo\033[0m\tx\nthree\n'`}, 2)
	if out != "two\tx\nthree" {
		t.Errorf("logs = %q", out)
	}
	if got := (System{}).Logs(Target{}, 5); got != "" {
		t.Errorf("a target without logs yields nothing, got %q", got)
	}
}

func TestStatusPrefersTheHook(t *testing.T) {
	if err := (System{}).Status(Target{Status: "true", Via: []string{"false"}}); err != nil {
		t.Errorf("a passing hook means up, regardless of via: %v", err)
	}
	if err := (System{}).Status(Target{Status: "false"}); err == nil {
		t.Errorf("a failing hook means down")
	}
	if err := (System{}).Status(Target{}); err != nil {
		t.Errorf("a local target without a hook is up: %v", err)
	}
	if err := (System{}).Status(Target{Via: []string{"false"}}); err == nil {
		t.Errorf("without a hook, a failing transport means down")
	}
	if err := (System{}).Status(Target{Via: []string{"false"}, Exec: []string{"sh", "-c"}}); err != nil {
		t.Errorf("without a hook, exec carries the check: %v", err)
	}
}

func TestHookFailureCarriesItsStderr(t *testing.T) {
	err := (System{}).Up(Target{Up: "echo progress; echo 'no such instance' >&2; exit 1"})
	if err == nil || err.Error() != "no such instance" {
		t.Errorf("a failed hook should say why and nothing else, got %v", err)
	}
	if err := (System{}).Up(Target{Up: "exit 3"}); err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("a silent failure still reports its exit, got %v", err)
	}
	if err := (System{}).Down(Target{}); err == nil {
		t.Errorf("a missing hook cannot succeed")
	}
}

func TestStatsFlowThroughExec(t *testing.T) {
	metrics := []stats.Metric{{Type: stats.TypeGauge, Label: "where", Value: "echo 1", Max: "2"}}
	for name, target := range map[string]Target{
		"local": {Stats: metrics},
		"via":   {Stats: metrics, Via: []string{"false"}, Exec: []string{"sh", "-c", "exec sh"}},
	} {
		snapshot, err := System{}.Inspect(target)
		if err != nil {
			t.Fatalf("%s: stats through a local shell: %v", name, err)
		}
		readings := snapshot.Readings
		if len(readings) != 1 || !readings[0].OK || readings[0].Value != 1 {
			t.Errorf("%s: the declared metrics should come back, got %+v", name, readings)
		}
	}
}

func TestStatsAreValidated(t *testing.T) {
	targets, err := load(t, `
targets:`+host+`
  - name: broken
    stats: [{ type: gauge, label: gpu, value: nvidia-smi }]
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("broken stats must be reported")
	}
	for _, want := range []string{`"broken": stats #1: gauge needs a max`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q:\n%v", want, err)
		}
	}
	if len(targets) == 0 || targets[0].Name != "host" {
		t.Errorf("the host should survive, got %+v", targets)
	}
}

func TestLogsDefaultToTheGuestJournal(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: omitted
  - { name: off, logs: "" }
  - { name: declared, logs: "tail {name}.log" }
`)
	if omitted := find(t, targets, "omitted"); !omitted.GuestLogs || omitted.Logs != "" {
		t.Errorf("left out means the guest journal, got %+v", omitted)
	}
	if off := find(t, targets, "off"); off.GuestLogs || off.Logs != "" {
		t.Errorf(`logs: "" turns logs off, got %+v`, off)
	}
	if declared := find(t, targets, "declared"); declared.GuestLogs || declared.Logs != "tail declared.log" {
		t.Errorf("declared logs stay host-side and interpolate, got %+v", declared)
	}
}

func TestGuestLogsFlowThroughExec(t *testing.T) {
	guest := Target{GuestLogs: true, Exec: []string{"sh", "-c", `printf 'old\n\033[31m%s\033[0m\n' "$0"`}}
	if got, want := (System{}).Logs(guest, 1), journalCommand; got != want {
		t.Errorf("want the journal command through exec, stripped and limited, got %q", got)
	}
	if got := (System{}).Logs(Target{}, 1); got != "" {
		t.Errorf("no logs at all should run nothing, got %q", got)
	}
}

func TestStatsDefaultToBuiltIn(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: omitted
  - { name: off, stats: [] }
  - { name: declared, stats: [{ type: text, label: a, value: x }] }
`)
	if omitted := find(t, targets, "omitted"); !omitted.BuiltinStats || len(omitted.Stats) != 0 {
		t.Errorf("left out means built-in, got %+v", omitted)
	}
	if off := find(t, targets, "off"); off.BuiltinStats || len(off.Stats) != 0 {
		t.Errorf("an empty list turns stats off, got %+v", off)
	}
	if declared := find(t, targets, "declared"); declared.BuiltinStats || len(declared.Stats) != 1 {
		t.Errorf("declared stats are used as written, got %+v", declared)
	}
}

func TestContainersGetTheContainerPreset(t *testing.T) {
	targets, err := Merge(fakeSource{name: "docker", specs: []Spec{{Name: "web", Kind: KindContainer}}})
	if err != nil {
		t.Fatal(err)
	}
	preset, _ := stats.Preset(stats.PresetContainer)
	if got := targets[0]; got.BuiltinStats || !slices.Equal(got.Stats, preset) {
		t.Errorf("uname cannot tell a container apart, got %+v", got)
	}
}

func TestKindCannotBeConfigured(t *testing.T) {
	if _, err := load(t, "targets:\n  - { name: box, kind: container }\n"); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Errorf("kind belongs to providers, got %v", err)
	}
}

func TestAnchoredStatsAreShared(t *testing.T) {
	targets := mustLoad(t, `
x-linux: &linux [{ type: text, label: a, value: x }, { type: text, label: b, value: y }]
targets:
  - { name: one, stats: *linux, modes: [{ name: shell }] }
  - { name: two, stats: *linux, modes: [{ name: shell }] }
`)
	if len(find(t, targets, "one").Stats) != 2 || len(find(t, targets, "two").Stats) != 2 {
		t.Errorf("an anchored list should reach every target, got %+v", targets)
	}
}

func noTargets(t *testing.T, targets []Target) {
	t.Helper()
	if len(targets) != 0 {
		t.Errorf("a refused config should give no targets, got %v", targets)
	}
}

func TestUnknownFieldsRefuseTheConfig(t *testing.T) {
	for where, body := range map[string]string{
		"target":  "targets:\n  - { name: prod, vai: ssh prod, modes: [{ name: shell }] }\n",
		"mode":    "targets:\n  - { name: prod, modes: [{ name: shell, rnu: bash }] }\n",
		"stat":    "targets:\n  - { name: prod, stats: [{ type: text, label: a, value: b, lable: c }], modes: [{ name: shell }] }\n",
		"anchor":  "x-vm: &vm { vai: ssh vm }\ntargets:\n  - { <<: *vm, name: prod, modes: [{ name: shell }] }\n",
		"display": "targets:\n  - { name: prod, display: db, modes: [{ name: shell }] }\n",
	} {
		targets, err := load(t, body)
		if err == nil || !strings.Contains(err.Error(), "line ") || !strings.Contains(err.Error(), "not found") {
			t.Errorf("%s: an unknown field should be reported with its line, got %v", where, err)
		}
		noTargets(t, targets)
	}
}

func TestUnknownSectionsRefuseTheConfig(t *testing.T) {
	for _, section := range []string{"lima", "profiles"} {
		targets, err := load(t, section+":\n  vm: { via: ssh vm }\ntargets:"+host)
		if err == nil || !strings.Contains(err.Error(), `unknown section "`+section+`"`) {
			t.Errorf("an unknown top-level key should be reported, got %v", err)
		}
		noTargets(t, targets)
	}

	if _, err := load(t, "x-anything: [1, 2]\ntargets:"+host); err != nil {
		t.Errorf("x- keys hold anchors and are allowed, got %v", err)
	}
}

func TestStatsMustBeAList(t *testing.T) {
	for _, value := range []string{"false", "linux"} {
		targets, err := load(t, "targets:\n  - { name: prod, stats: "+value+", modes: [{ name: shell }] }\n")
		if err == nil {
			t.Errorf("stats: %s should be refused", value)
		}
		noTargets(t, targets)
	}
}

func TestStatsNamePresets(t *testing.T) {
	targets, err := load(t, `
targets:`+host+`
  - name: box
    stats: [linux, { type: gauge, label: gpu, value: gpu, max: 100 }]
    modes: [{ name: shell }]
  - { name: typo, stats: [linx], modes: [{ name: shell }] }
`)
	if err == nil || !strings.Contains(err.Error(), `"typo": stats: unknown preset "linx"`) {
		t.Errorf("an unknown preset should be reported on its target, got %v", err)
	}
	linux, _ := stats.Preset(stats.PresetLinux)
	box := find(t, targets, "box").Stats
	if len(box) != len(linux)+1 || box[0] != linux[0] || box[len(box)-1].Label != "gpu" {
		t.Errorf("want the linux preset followed by gpu, got %+v", box)
	}
	if len(targets) != 2 {
		t.Errorf("only the typo should be skipped, got %d targets", len(targets))
	}
}

func TestMalformedYAMLNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("targets: [ { name: host\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("malformed yaml should name the file, got %v", err)
	}
	noTargets(t, targets)
}

type fakeMultiplexer struct{}

func (fakeMultiplexer) Tool() string                         { return "fake" }
func (fakeMultiplexer) Attach() string                       { return "fake attach" }
func (fakeMultiplexer) Script() string                       { return "" }
func (fakeMultiplexer) Parse([]string) []multiplexer.Session { return nil }

func withFakeMultiplexer(t *testing.T) {
	t.Helper()
	previous := multiplexer.All
	t.Cleanup(func() { multiplexer.All = previous })
	multiplexer.All = []multiplexer.Multiplexer{multiplexer.Tmux{}, fakeMultiplexer{}}
}

func TestDefaultModeOrdersTheAutomaticModes(t *testing.T) {
	withFakeMultiplexer(t)
	source := fakeSource{name: "test", specs: []Spec{{Name: "box"}}}

	for defaultMode, want := range map[string][]string{
		"":      {"tmux", "fake", "shell"},
		"tmux":  {"tmux", "fake", "shell"},
		"fake":  {"fake", "tmux", "shell"},
		"shell": {"shell", "tmux", "fake"},
	} {
		targets, err := MergeWithDefault(defaultMode, source)
		if err != nil {
			t.Fatalf("%q: %v", defaultMode, err)
		}
		if got := modeNames(targets[0]); !slices.Equal(got, want) {
			t.Errorf("%q: want %v, got %v", defaultMode, want, got)
		}
	}
}

func TestZellijCanBeTheDefaultMode(t *testing.T) {
	targets, err := MergeWithDefault("zellij", fakeSource{name: "test", specs: []Spec{{Name: "box"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := modeNames(targets[0]); !slices.Equal(got, []string{"zellij", "tmux", "shell"}) {
		t.Errorf("want zellij, tmux, shell, got %v", got)
	}
}

func TestDefaultModeLeavesExplicitModesAlone(t *testing.T) {
	withFakeMultiplexer(t)
	modes := []ModeSpec{{Name: "root"}, {Name: "tmux"}}
	source := fakeSource{name: "test", specs: []Spec{
		{Name: "declared", Modes: modes},
		{Name: "inherited", Defaults: &Spec{Modes: modes}},
	}}

	targets, err := MergeWithDefault("tmux", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if got := modeNames(target); !slices.Equal(got, []string{"root", "tmux"}) {
			t.Errorf("%s: explicit modes keep their order, got %v", target.Name, got)
		}
	}
}

func TestUnknownDefaultModeIsReportedAndTargetsStillLoad(t *testing.T) {
	targets, err := MergeWithDefault("zelij", fakeSource{name: "test", specs: []Spec{{Name: "box"}}})
	if err == nil || !strings.Contains(err.Error(), `unknown mode "zelij"`) || !strings.Contains(err.Error(), "tmux, zellij, shell") {
		t.Errorf("want the value and the choices reported, got %v", err)
	}
	if len(targets) != 1 || modeNames(targets[0])[0] != "tmux" {
		t.Errorf("want the target with the implicit default, got %+v", targets)
	}
}

func TestDefaultModeIsReadFromTheConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("default_mode: shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Read(path)
	if err != nil || doc.DefaultMode != "shell" {
		t.Errorf("want shell, got %q and %v", doc.DefaultMode, err)
	}

	if err := os.WriteFile(path, []byte("default_mode: [shell]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Error("a list is not a mode name")
	}
}
