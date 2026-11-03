package config_test

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/inspection/stats"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/shell"
	"github.com/jazho76/uplink/internal/target"
)

const host = `
  - name: host
    modes:
      - { name: shell }
`

func load(t *testing.T, body string) ([]target.Target, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return loadConfig(path)
}

func loadConfig(path string) ([]target.Target, error) {
	return inventory.Load(context.Background(), inventory.Catalog{Path: path})
}

func mustLoad(t *testing.T, body string) []target.Target {
	t.Helper()
	targets, err := load(t, body)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return targets
}

func find(t *testing.T, targets []target.Target, name string) target.Target {
	t.Helper()
	i := slices.IndexFunc(targets, func(found target.Target) bool { return found.Name == name })
	if i < 0 {
		t.Fatalf("no target %q", name)
	}
	return targets[i]
}

func modeNames(found target.Target) []string {
	names := make([]string, 0, len(found.Modes))
	for _, m := range found.Modes {
		names = append(names, m.Name)
	}
	return names
}

func modeNamed(t *testing.T, found target.Target, name string) target.Mode {
	t.Helper()
	i := slices.IndexFunc(found.Modes, func(m target.Mode) bool { return m.Name == name })
	if i < 0 {
		t.Fatalf("%s has no mode %q", found.Name, name)
	}
	return found.Modes[i]
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
	if want := (target.ID{Source: target.ConfigSource, Key: "host"}); got != want {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestDiscoverAbsentDiffersFromEmpty(t *testing.T) {
	for body, wantNil := range map[string]bool{"targets: []\n": true, "discover: []\n": false} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		doc, err := config.Read(path)
		if err != nil || (doc.Discover == nil) != wantNil {
			t.Errorf("%q: want nil discover %v, got %#v and %v", body, wantNil, doc.Discover, err)
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
	if forge.Group != "vms" || !forge.Watch || len(forge.Stats.Metrics) != 2 {
		t.Errorf("forge should inherit group, watch and stats, got %+v", forge)
	}
	if got := forge.Stats.Metrics[0].Value + ", " + forge.Stats.Metrics[1].Value; got != "uptime forge, jobs forge" {
		t.Errorf("metric commands should interpolate the name, got %q", got)
	}
	wantVia := []string{"ssh", "-F", filepath.Join(home, ".lima/forge/ssh.config"), "lima-forge"}
	if !slices.Equal(forge.Reach.Interactive, wantVia) {
		t.Errorf("via = %q, want %q", forge.Reach.Interactive, wantVia)
	}
	if forge.Logs.Command != "tail ~/.lima/forge/serial.log" {
		t.Errorf("logs should interpolate the name and leave ~ to the shell, got %q", forge.Logs.Command)
	}
	if got := forge.Modes[0].Argv; !slices.Equal(got, append(wantVia, shell.POSIX("tmux new-session -A -s forge"))) {
		t.Errorf("tmux argv = %q", got)
	}
	if mode := modeNamed(t, forge, "shell"); !slices.Equal(mode.Argv, append(wantVia, shell.POSIX(shell.LoginShell))) {
		t.Errorf("a mode without run opens the user's shell, got %q", mode.Argv)
	}

	tokyo := find(t, targets, "tokyo")
	if tokyo.Logs.Command != "echo own" || tokyo.Stats.Metrics == nil || len(tokyo.Stats.Metrics) != 0 {
		t.Errorf("target fields override the anchor, got logs %q stats %v", tokyo.Logs.Command, tokyo.Stats.Metrics)
	}
	if got := modeNames(tokyo); !slices.Equal(got, []string{"top"}) {
		t.Errorf("target modes replace the anchor's wholesale, got %v", got)
	}
	if top := modeNamed(t, tokyo, "top"); top.Argv[len(top.Argv)-1] != shell.POSIX("htop") {
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
	if got := local.Modes[0].Argv; !slices.Equal(got, []string{"sh", "-c", shell.POSIX("tmux new-session -A -s host")}) {
		t.Errorf("tmux argv = %q", got)
	}
	if mode := modeNamed(t, local, "shell"); !slices.Equal(mode.Argv, []string{"sh", "-c", shell.POSIX(shell.LoginShell)}) {
		t.Errorf("shell argv = %q", mode.Argv)
	}
	if local.Group != target.DefaultGroup || local.Watch || local.Stats.Metrics != nil {
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
	if got := find(t, targets, "box").Reach.Probe("true"); !slices.Equal(got, []string{"docker", "exec", "-i", "box", "sh", "-c", "true"}) {
		t.Errorf("exec should drive background commands, got %q", got)
	}
	if got := find(t, targets, "plain").Reach.Probe("true"); !slices.Equal(got, []string{"ssh", "-t", "plain", "true"}) {
		t.Errorf("without exec, via drives background commands, got %q", got)
	}
}

func TestHooksInterpolateTheName(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: forge
    via: ssh lima-{name}
    check: limactl list {name}
    up: limactl start {name}
    down: limactl stop {name}
    modes: [{ name: shell }]
`)
	forge := find(t, targets, "forge")
	if forge.Check != "limactl list forge" || forge.StartCommand() != "limactl start forge" || forge.StopCommand() != "limactl stop forge" {
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
	if got := find(t, targets, "one").Reach.Interactive; !slices.Equal(got, []string{"ssh", "-t", "shared"}) {
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
		found := find(t, targets, name)
		if got := modeNames(found); !slices.Equal(got, []string{"tmux", "zellij", "shell"}) {
			t.Errorf("%s: want tmux, zellij, shell, got %v", name, got)
		}
		if got := modeNamed(t, found, "tmux").Requires; got != "tmux" {
			t.Errorf("%s: the default tmux mode needs tmux, got %q", name, got)
		}
		if got := modeNamed(t, found, "shell").Requires; got != "" {
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
	if want := []string{"ssh", "-t", "dojo", shell.POSIX("bash -lc 'lc && tmux'")}; !slices.Equal(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
	if got := shell.Quote(argv); got != `ssh -t dojo 'sh -c '\''bash -lc '\''\'\'''\''lc && tmux'\''\'\'''\'''\'''` {
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
	for _, found := range targets {
		names = append(names, found.Name)
	}
	if !slices.Equal(names, []string{"host", "good"}) {
		t.Errorf("valid targets must survive, got %v", names)
	}
	if find(t, targets, "good").Reach.Interactive[1] != "good" {
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
	if omitted := find(t, targets, "omitted"); !omitted.Logs.Journal || omitted.Logs.Command != "" {
		t.Errorf("left out means the guest journal, got %+v", omitted)
	}
	if off := find(t, targets, "off"); off.Logs.Journal || off.Logs.Command != "" {
		t.Errorf(`logs: "" turns logs off, got %+v`, off)
	}
	if declared := find(t, targets, "declared"); declared.Logs.Journal || declared.Logs.Command != "tail declared.log" {
		t.Errorf("declared logs stay host-side and interpolate, got %+v", declared)
	}
}

func TestStatsDefaultToBuiltIn(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: omitted
  - { name: off, stats: [] }
  - { name: declared, stats: [{ type: text, label: a, value: x }] }
`)
	if omitted := find(t, targets, "omitted"); !omitted.Stats.Builtin || len(omitted.Stats.Metrics) != 0 {
		t.Errorf("left out means built-in, got %+v", omitted)
	}
	if off := find(t, targets, "off"); off.Stats.Builtin || len(off.Stats.Metrics) != 0 {
		t.Errorf("an empty list turns stats off, got %+v", off)
	}
	if declared := find(t, targets, "declared"); declared.Stats.Builtin || len(declared.Stats.Metrics) != 1 {
		t.Errorf("declared stats are used as written, got %+v", declared)
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
	if len(find(t, targets, "one").Stats.Metrics) != 2 || len(find(t, targets, "two").Stats.Metrics) != 2 {
		t.Errorf("an anchored list should reach every target, got %+v", targets)
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
	box := find(t, targets, "box").Stats.Metrics
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

func TestDefaultModeIsReadFromTheConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("default_mode: shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := config.Read(path)
	if err != nil || doc.DefaultMode != "shell" {
		t.Errorf("want shell, got %q and %v", doc.DefaultMode, err)
	}

	if err := os.WriteFile(path, []byte("default_mode: [shell]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Read(path); err == nil {
		t.Error("a list is not a mode name")
	}
}

func noTargets(t *testing.T, targets []target.Target) {
	t.Helper()
	if len(targets) != 0 {
		t.Errorf("a refused config should give no targets, got %v", targets)
	}
}

func TestDeclaredModesNameTheirMultiplexer(t *testing.T) {
	targets := mustLoad(t, `
targets:
  - name: box
    modes:
      - { name: work, multiplexer: tmux, run: tmux }
      - { name: root, run: sudo tmux }
`)
	box := find(t, targets, "box")
	if got := modeNamed(t, box, "work"); got.Multiplexer != "tmux" || got.Requires != "" {
		t.Errorf("a named multiplexer shows sessions without filtering the mode, got %+v", got)
	}
	if got := modeNamed(t, box, "root").Multiplexer; got != "" {
		t.Errorf("no multiplexer means no sessions, got %q", got)
	}

	_, err := load(t, "targets:\n  - { name: box, modes: [{ name: a, multiplexer: screen }] }\n")
	if err == nil || !strings.Contains(err.Error(), `unknown multiplexer "screen"`) {
		t.Errorf("want the unknown multiplexer reported, got %v", err)
	}
}

func TestAutomaticModesSurviveANonPOSIXLoginShell(t *testing.T) {
	dir := t.TempDir()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sh, filepath.Join(dir, "sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\necho \"tmux $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	strict := `#!/bin/sh
case "$*" in *'${'*) echo "syntax error" >&2; exit 127 ;; esac
[ "$1" = -lc ] && exec sh -c "$2"
echo "user shell"
`
	if err := os.WriteFile(filepath.Join(dir, "strict-shell"), []byte(strict), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SHELL", filepath.Join(dir, "strict-shell"))

	box := find(t, mustLoad(t, "targets:\n  - name: box\n"), "box")
	for mode, want := range map[string]string{"tmux": "tmux new-session -A -s 0", "shell": "user shell"} {
		argv := modeNamed(t, box, mode).Argv
		payload := argv[len(argv)-1]
		if !strings.HasPrefix(payload, "sh -c ") {
			t.Errorf("%s: a remote login shell must only have to parse sh -c, got %q", mode, payload)
		}
		out, err := exec.Command(argv[0], argv[1:]...).Output()
		if err != nil {
			t.Fatalf("%s: %q: %v", mode, argv, err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s: got %q, want %q", mode, got, want)
		}
	}
}
