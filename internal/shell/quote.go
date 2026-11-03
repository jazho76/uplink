package shell

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const LoginShell = `exec "${SHELL:-/bin/sh}" -l`

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func Quote(argv []string) string {
	parts := make([]string, len(argv))
	for i, word := range argv {
		parts[i] = word
		if !plainWord.MatchString(word) {
			parts[i] = SingleQuote(word)
		}
	}
	return strings.Join(parts, " ")
}

func SingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func POSIX(command string) string {
	return "sh -c " + SingleQuote(command)
}

func Preamble(limit time.Duration) string {
	return fmt.Sprintf(`if command -v timeout >/dev/null 2>&1; then
  limit='timeout %[1]d'
else
  limited() { sh -c '
    set -m
    "$@" &
    pid=$!
    { sleep %[1]d && kill -KILL -$pid; } >/dev/null 2>&1 </dev/null &
    watcher=$!
    wait $pid
    status=$?
    kill $watcher 2>/dev/null
    exit $status
  ' sh "$@"; }
  limit=limited
fi
login_path=$($limit "${SHELL:-/bin/sh}" -lc 'printenv PATH' </dev/null 2>/dev/null | tail -n 1)
case "$login_path" in /*) PATH="$PATH:$login_path" ;; esac`, int(limit.Seconds()))
}

func Words(s string) ([]string, error) {
	var (
		words   []string
		current strings.Builder
		inWord  bool
		quote   byte
		escaped bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			current.WriteByte(c)
			escaped = false
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				current.WriteByte(c)
			}
		case quote == '"' && c == '"':
			quote = 0
		case quote == '"' && c == '\\' && i+1 < len(s) && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0:
			escaped = true
		case quote == '"':
			current.WriteByte(c)
		case c == '\\':
			escaped, inWord = true, true
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				words = append(words, current.String())
				current.Reset()
				inWord = false
			}
		default:
			current.WriteByte(c)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, errors.New("unterminated quote or escape")
	}
	if inWord {
		words = append(words, current.String())
	}
	return words, nil
}
