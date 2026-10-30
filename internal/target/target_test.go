package target

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/probe"
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
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return Load(cfg)
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
	found, err := Resolve(targets, name)
	if err != nil {
		t.Fatal(err)
	}
	return found
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

func TestMissingTargetsFallBackToTheHost(t *testing.T) {
	targets := mustLoad(t, "")
	if len(targets) != 1 || targets[0].Name != "host" || targets[0].Group != "local" || targets[0].Stats != nil {
		t.Fatalf("an empty config should still offer the host, got %+v", targets)
	}
	if got := targets[0].ModeNames(); !slices.Equal(got, []string{"tmux", "shell"}) {
		t.Errorf("default host modes = %v", got)
	}
}

func TestProfileFillsWhatTheTargetLeavesOut(t *testing.T) {
	targets := mustLoad(t, `
profiles:
  lima:
    group: vms
    watch: true
    via: ssh -F ~/.lima/{name}/ssh.config lima-{name}
    logs: tail ~/.lima/{name}/serial.log
    stats: [{ type: text, label: up, unit: seconds, value: "uptime {name}" }, { type: gauge, label: jobs, value: "jobs {name}", max: 8 }]
    modes:
      - { name: tmux, run: "tmux new-session -A -s {name}" }
      - { name: shell }
targets:`+host+`
  - { name: forge, profile: lima }
  - name: tokyo
    profile: lima
    logs: echo own
    stats: []
    modes:
      - { name: top, run: htop, back: true }
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
	if got := forge.DefaultMode().Argv; !slices.Equal(got, append(wantVia, "tmux new-session -A -s forge")) {
		t.Errorf("tmux argv = %q", got)
	}
	if shell, _ := forge.Mode("shell"); !slices.Equal(shell.Argv, append(wantVia, loginShell)) {
		t.Errorf("a mode without run opens the user's shell, got %q", shell.Argv)
	}

	tokyo := find(t, targets, "tokyo")
	if tokyo.Logs != "echo own" || tokyo.Stats == nil || len(tokyo.Stats) != 0 {
		t.Errorf("target fields override the profile, got logs %q stats %v", tokyo.Logs, tokyo.Stats)
	}
	if got := tokyo.ModeNames(); !slices.Equal(got, []string{"top"}) {
		t.Errorf("target modes replace the profile's wholesale, got %v", got)
	}
	if top, _ := tokyo.Mode("top"); top.Argv[len(top.Argv)-1] != "htop" {
		t.Errorf("a mode still carrying the retired back key should load, got %q", top.Argv)
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
	if got := local.DefaultMode().Argv; !slices.Equal(got, []string{"sh", "-c", "tmux new-session -A -s host"}) {
		t.Errorf("tmux argv = %q", got)
	}
	if shell, _ := local.Mode("shell"); !slices.Equal(shell.Argv, []string{"sh", "-c", loginShell}) {
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
profiles:
  lima:
    via: ssh lima-{name}
    status: limactl list {name}
    up: limactl start {name}
    down: limactl stop {name}
    modes: [{ name: shell }]
targets:
  - { name: forge, profile: lima }
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

func TestModeCommandTravelsAsOneArgument(t *testing.T) {
	targets := mustLoad(t, `
targets:`+host+`
  - name: dojo
    via: ssh -t dojo
    modes:
      - { name: tmux, run: "bash -lc 'lc && tmux'" }
`)
	argv := find(t, targets, "dojo").DefaultMode().Argv
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
  - name: noprofile
    profile: missing
  - name: nomodes
    via: ssh x
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
	for _, want := range []string{"unknown profile", "nomodes", `duplicate mode "a"`,
		"badquote", `"badexec": exec`, `duplicate target "good"`, "#9"} {
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

func TestExpandIsReportedButHarmless(t *testing.T) {
	targets, err := load(t, `
profiles:
  old:
    expand: true
targets:
  - name: host
    expand: true
    modes: [{ name: shell }]
  - { name: box, profile: old, modes: [{ name: shell }] }
`)
	if err == nil {
		t.Fatal("expand must be reported")
	}
	for _, want := range []string{`profile "old"`, `target "host"`, "modes are picked with tab"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s:\n%v", want, err)
		}
	}
	if len(targets) != 2 {
		t.Errorf("expand is ignored, so every target survives, got %d", len(targets))
	}
}

func TestKindPointsAtGroup(t *testing.T) {
	targets, err := load(t, `
profiles:
  old:
    kind: vm
targets:`+host+`
  - name: box
    kind: remote
    via: ssh box
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("kind must be reported")
	}
	for _, want := range []string{`profile "old"`, `target "box"`, "use group"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s:\n%v", want, err)
		}
	}
	if len(targets) != 1 {
		t.Errorf("only the host should survive, got %d targets", len(targets))
	}
}

func TestLegacySectionsPointAtTheMigration(t *testing.T) {
	_, err := load(t, `
lima:
  modes: [{ name: shell }]
remotes:
  - name: box
`)
	if err == nil {
		t.Fatal("legacy sections must be reported")
	}
	for _, want := range []string{`"lima"`, `"remotes"`, "README"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s:\n%v", want, err)
		}
	}
}

func TestResolveListsKnownTargets(t *testing.T) {
	targets := mustLoad(t, "")
	if _, err := Resolve(targets, "nope"); err == nil || !strings.Contains(err.Error(), "host") {
		t.Errorf("an unknown name should list the known targets, got %v", err)
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
	metrics := probe.Metrics{{Type: probe.TypeGauge, Label: "where", Value: "echo 1", Max: "2"}}
	for name, target := range map[string]Target{
		"local": {Stats: metrics},
		"via":   {Stats: metrics, Via: []string{"false"}, Exec: []string{"sh", "-c", "exec sh"}},
	} {
		readings, err := System{}.Stats(target)
		if err != nil {
			t.Fatalf("%s: stats through a local shell: %v", name, err)
		}
		if len(readings) != 1 || !readings[0].OK || readings[0].Value != 1 {
			t.Errorf("%s: the declared metrics should come back, got %+v", name, readings)
		}
	}
}

func TestStatsAreValidated(t *testing.T) {
	targets, err := load(t, `
targets:`+host+`
  - name: broken
    stats: [{ type: load }, { type: gauge, label: gpu }]
    modes: [{ name: shell }]
  - name: legacy
    stats: false
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("broken stats must be reported")
	}
	for _, want := range []string{`"broken": stats #1: load is no longer built in`, "use [] for none"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q:\n%v", want, err)
		}
	}
	if len(targets) == 0 || targets[0].Name != "host" {
		t.Errorf("the host should survive, got %+v", targets)
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
