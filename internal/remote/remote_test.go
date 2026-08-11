package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func argvOf(t *testing.T, r Remote, mode string) string {
	t.Helper()
	p, err := New(Config{r}, t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	targets, err := p.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	m, ok := targets[0].Mode(mode)
	if !ok {
		t.Fatalf("no mode %q in %v", mode, targets[0].ModeNames())
	}
	return strings.Join(m.Argv, " ")
}

func TestDojoScriptEquivalence(t *testing.T) {
	dojo := Remote{
		Name: "dojo",
		SSH:  "hacker@dojo.pwn.college",
		Init: "lc",
		Modes: []target.ModeSpec{
			{Name: "tmux", Run: "tmux"},
			{Name: "shell"},
			{Name: "root", Run: "sudo env HOME=/home/hacker/ tmux"},
		},
	}

	cases := map[string]string{
		"tmux":  `ssh -t hacker@dojo.pwn.college bash --login -c 'lc && tmux'`,
		"shell": `ssh -t hacker@dojo.pwn.college bash --login -c 'lc && exec bash'`,
		"root":  `ssh -t hacker@dojo.pwn.college bash --login -c 'lc && sudo env HOME=/home/hacker/ tmux'`,
	}
	for mode, want := range cases {
		if got := argvOf(t, dojo, mode); got != want {
			t.Errorf("mode %s:\n got %s\nwant %s", mode, got, want)
		}
	}
}

func TestNoInitNoPayloadIsAPlainLogin(t *testing.T) {
	bare := Remote{Name: "box", Modes: []target.ModeSpec{{Name: "shell"}}}
	if got, want := argvOf(t, bare, "shell"), "ssh -t box"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDestinationFallsBackToName(t *testing.T) {
	r := Remote{Name: "alias", Modes: []target.ModeSpec{{Name: "shell", Run: "true"}}}
	if got := argvOf(t, r, "shell"); !strings.Contains(got, " alias bash") {
		t.Errorf("a bare entry should use its name as the ssh destination, got %q", got)
	}
}

func TestDefaultModeIsAShell(t *testing.T) {
	p, err := New(Config{{Name: "box"}}, t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	targets, _ := p.List()
	if names := targets[0].ModeNames(); len(names) != 1 || names[0] != "shell" {
		t.Errorf("a remote with no modes gets a lone shell, got %v", names)
	}
}

func TestFlagOrderAndIdentity(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "id")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := New(Config{{
		Name:     "box",
		SSH:      "me@box",
		Identity: "id",
		Port:     2222,
		SSHArgs:  []string{"-o", "ServerAliveInterval=30"},
		Modes:    []target.ModeSpec{{Name: "shell", Run: "tmux"}},
	}}, dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	targets, _ := p.List()
	got := strings.Join(targets[0].Modes[0].Argv, " ")

	want := "ssh -i " + key + " -o IdentitiesOnly=yes -p 2222 -o ServerAliveInterval=30 -t me@box bash --login -c 'tmux'"
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestQuotingSurvivesEmbeddedQuotes(t *testing.T) {
	r := Remote{
		Name:  "box",
		Modes: []target.ModeSpec{{Name: "shell", Run: `echo 'hi there'`}},
	}
	got := argvOf(t, r, "shell")
	want := `ssh -t box bash --login -c 'echo '\''hi there'\'''`
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}

	quoted := got[strings.Index(got, "-c ")+3:]
	if unquoted := unquote(quoted); unquoted != `echo 'hi there'` {
		t.Errorf("remote shell would see %q", unquoted)
	}
}

func unquote(s string) string {
	var b strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			inQuotes = !inQuotes
		case s[i] == '\\' && !inQuotes && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func TestIdentityWarnings(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open")
	if err := os.WriteFile(open, []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		identity string
		want     string
	}{
		{"missing", filepath.Join(dir, "nope"), "missing"},
		{"too open", open, "permissions too open"},
		{"directory", dir, "not a file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := New(Config{{Name: "box", Identity: c.identity}}, dir)
			if err != nil {
				t.Fatalf("a bad key is a warning, not an error: %v", err)
			}
			targets, _ := p.List()
			var keyField string
			for _, f := range targets[0].Detail {
				if f.Key == "key" {
					keyField = f.Value
				}
			}
			if !strings.Contains(keyField, c.want) {
				t.Errorf("detail should warn %q, got %q", c.want, keyField)
			}
		})
	}
}

func TestHardConfigErrors(t *testing.T) {
	cases := map[string]Config{
		"no name":        {{SSH: "box"}},
		"duplicate name": {{Name: "box"}, {Name: "box"}},
		"unnamed mode":   {{Name: "box", Modes: []target.ModeSpec{{Run: "tmux"}}}},
		"duplicate mode": {{Name: "box", Modes: []target.ModeSpec{{Name: "a"}, {Name: "a"}}}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg, t.TempDir()); err == nil {
				t.Errorf("expected an error")
			}
		})
	}
}

const reservedUnroutableAddress = "192.0.2.1"

func TestStatusStartsUnknownAndProbeResolvesIt(t *testing.T) {
	p, err := New(Config{{Name: "box", SSH: reservedUnroutableAddress}}, t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	targets, _ := p.List()
	if targets[0].Status != target.StatusUnknown {
		t.Errorf("a remote is unknown until probed, got %q", targets[0].Status)
	}

	if _, err := p.Probe("box"); err == nil {
		t.Skip("the documentation address answered; skipping")
	}
	targets, _ = p.List()
	if targets[0].Status != target.StatusUnreachable {
		t.Errorf("a failed probe should mark the remote unreachable, got %q", targets[0].Status)
	}
}

func TestProbeArgvAvoidsTTYAndPrompts(t *testing.T) {
	p, _ := New(Config{{Name: "box", SSH: "me@box", Port: 22}}, t.TempDir())
	e, _ := p.find("box")
	argv := strings.Join(e.probeArgv(), " ")

	for _, want := range []string{"BatchMode=yes", "ConnectTimeout=2", "me@box", "sh"} {
		if !strings.Contains(argv, want) {
			t.Errorf("probe argv missing %q: %s", want, argv)
		}
	}
	if strings.Contains(argv, " -t") {
		t.Errorf("probe must not request a tty: %s", argv)
	}
}
