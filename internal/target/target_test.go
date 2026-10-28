package target

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/config"
)

const host = `
  - name: host
    kind: local
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
	if len(targets) != 1 || !targets[0].Local() || targets[0].Name != "host" {
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
    kind: vm
    via: ssh -F ~/.lima/{name}/ssh.config lima-{name}
    logs: tail ~/.lima/{name}/serial.log
    modes:
      - { name: tmux, run: "tmux new-session -A -s {name}" }
      - { name: shell }
targets:`+host+`
  - { name: forge, profile: lima }
  - name: tokyo
    profile: lima
    logs: echo own
    stats: false
    modes:
      - { name: top, run: htop, back: true }
`)
	home, _ := os.UserHomeDir()

	forge := find(t, targets, "forge")
	if forge.Kind != KindVM || !forge.Stats {
		t.Errorf("forge should inherit kind and default to stats, got %+v", forge)
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
	if shell, _ := forge.Mode("shell"); !slices.Equal(shell.Argv, wantVia) {
		t.Errorf("a mode without run opens via alone, got %q", shell.Argv)
	}

	tokyo := find(t, targets, "tokyo")
	if tokyo.Logs != "echo own" || tokyo.Stats {
		t.Errorf("target fields override the profile, got logs %q stats %v", tokyo.Logs, tokyo.Stats)
	}
	if got := tokyo.ModeNames(); !slices.Equal(got, []string{"top"}) {
		t.Errorf("target modes replace the profile's wholesale, got %v", got)
	}
	if top, _ := tokyo.Mode("top"); !top.Back {
		t.Errorf("back should survive the merge")
	}
}

func TestLocalModesRunThroughTheLoginShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	targets := mustLoad(t, `
targets:
  - name: host
    kind: local
    modes:
      - { name: tmux, run: tmux new-session -A -s host }
      - { name: shell }
`)
	local := find(t, targets, "host")
	if got := local.DefaultMode().Argv; !slices.Equal(got, []string{"/bin/zsh", "-c", "tmux new-session -A -s host"}) {
		t.Errorf("tmux argv = %q", got)
	}
	if shell, _ := local.Mode("shell"); !slices.Equal(shell.Argv, []string{"/bin/zsh"}) {
		t.Errorf("shell argv = %q", shell.Argv)
	}
}

func TestModeCommandTravelsAsOneArgument(t *testing.T) {
	targets := mustLoad(t, `
targets:`+host+`
  - name: dojo
    kind: remote
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
    kind: remote
    via: ssh good
    modes: [{ name: shell }]
  - name: nokind
    via: ssh x
    modes: [{ name: shell }]
  - name: novia
    kind: vm
    modes: [{ name: shell }]
  - name: noprofile
    profile: missing
  - name: nomodes
    kind: remote
    via: ssh x
  - name: dupmodes
    kind: remote
    via: ssh x
    modes: [{ name: a }, { name: a }]
  - name: badquote
    kind: remote
    via: ssh 'x
    modes: [{ name: shell }]
  - name: good
    kind: remote
    via: ssh other
    modes: [{ name: shell }]
  - kind: remote
    via: ssh x
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("every broken target must be reported")
	}
	for _, want := range []string{"nokind", "novia", "unknown profile", "nomodes", `duplicate mode "a"`,
		"badquote", `duplicate target "good"`, "#10"} {
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
    kind: remote
    via: ssh db
    modes: [{ name: shell }]
  - name: "my box"
    kind: remote
    via: ssh box
    modes: [{ name: shell }]
  - name: blank
    kind: remote
    via: "   "
    modes: [{ name: shell }]
`)
	if err == nil {
		t.Fatal("malformed targets must be reported")
	}
	for _, want := range []string{`"db:prod"`, `"my box"`, `"blank": via is required`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s:\n%v", want, err)
		}
	}
}

func TestBrokenHostIsNotReportedAsMissing(t *testing.T) {
	_, err := load(t, `
targets:
  - name: host
    kind: local
    modes: [{ name: a }, { name: a }]
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate mode") {
		t.Fatalf("the real problem must be reported, got %v", err)
	}
	if strings.Contains(err.Error(), "declare the host") {
		t.Errorf("a declared but broken host is not a missing one:\n%v", err)
	}
}

func TestExactlyOneLocalTarget(t *testing.T) {
	if _, err := load(t, `
targets:
  - name: box
    kind: remote
    via: ssh box
    modes: [{ name: shell }]
`); err == nil || !strings.Contains(err.Error(), "no target is local") {
		t.Errorf("a config without the host must be reported, got %v", err)
	}

	targets, err := load(t, `
targets:`+host+`
  - name: other
    kind: local
    modes: [{ name: shell }]
`)
	if err == nil || !strings.Contains(err.Error(), "only one target can be local") {
		t.Errorf("a second local target must be reported, got %v", err)
	}
	if len(targets) != 1 {
		t.Errorf("the first local target should survive, got %d", len(targets))
	}

	if _, err := load(t, `
targets:
  - name: host
    kind: local
    via: ssh host
    modes: [{ name: shell }]
`); err == nil || !strings.Contains(err.Error(), "takes no via") {
		t.Errorf("a local target with via must be reported, got %v", err)
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

func TestReachableUsesVia(t *testing.T) {
	up := Target{Kind: KindRemote, Via: []string{"sh", "-c"}}
	if err := (System{}).Reachable(up); err != nil {
		t.Errorf("sh -c true should be reachable: %v", err)
	}
	down := Target{Kind: KindRemote, Via: []string{"false"}}
	if err := (System{}).Reachable(down); err == nil {
		t.Errorf("a failing transport is unreachable")
	}
}

func TestStatsFlowThroughVia(t *testing.T) {
	stats, err := System{}.Stats(Target{Kind: KindRemote, Via: []string{"sh", "-c", "exec sh"}})
	if err != nil {
		t.Fatalf("stats through a local shell: %v", err)
	}
	if stats.Cores < 1 || stats.MemTotal == 0 {
		t.Errorf("the probe script should report this machine, got %+v", stats)
	}
}
