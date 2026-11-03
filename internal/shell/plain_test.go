package shell

import "testing"

func TestPlainLeavesNothingATerminalWouldObey(t *testing.T) {
	for in, want := range map[string]string{
		"plain":              "plain",
		"\x1b[31mred\x1b[0m": "red",
		"\x1b]0;pwned\x07hi": "hi",
		"a\tb":               "a b",
		"be\x07ll\x7f":       "bell",
		"csi\u009b31m":       "csi31m",
		"one\ntwo":           "onetwo",
		"naïve ✓":            "naïve ✓",
	} {
		if got := Plain(in); got != want {
			t.Errorf("Plain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlainLinesKeepsLinesApart(t *testing.T) {
	if got := PlainLines("\x1b]0;unterminated\nsecond\n\x1b[31mthird\x1b[0m"); got != "\nsecond\nthird" {
		t.Errorf("an unterminated sequence must not swallow later lines, got %q", got)
	}
}
