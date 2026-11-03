package shell

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

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
		got, err := Words(c.in)
		if err != nil {
			t.Errorf("Words(%q): %v", c.in, err)
			continue
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("Words(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, broken := range []string{`ssh 'box`, `ssh "box`, `ssh box\`} {
		if _, err := Words(broken); err == nil {
			t.Errorf("Words(%q) should reject the unterminated input", broken)
		}
	}
}

func TestQuotePastesBackIntoAShell(t *testing.T) {
	for _, word := range []string{"plain", "a b", "it's", "$HOME", "a;b", "(x)", `back\\slash`, "", "~/x", "a|b"} {
		out, err := exec.Command("sh", "-c", "printf %s "+Quote([]string{word})).Output()
		if err != nil {
			t.Errorf("%q: %v", word, err)
		}
		if string(out) != word {
			t.Errorf("%q came back as %q", word, out)
		}
		if words, err := Words(Quote([]string{word})); err != nil || !slices.Equal(words, []string{word}) {
			t.Errorf("%q: words %q, %v", word, words, err)
		}
	}
}

func TestPOSIXWrapsOneArgumentForSh(t *testing.T) {
	out, err := exec.Command("sh", "-c", POSIX(`printf '%s' "it's"`)).Output()
	if err != nil || strings.TrimSpace(string(out)) != "it's" {
		t.Errorf("got %q, %v", out, err)
	}
}
