package shell

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

func Plain(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, ansi.Strip(s))
}

func PlainLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = Plain(line)
	}
	return strings.Join(lines, "\n")
}
