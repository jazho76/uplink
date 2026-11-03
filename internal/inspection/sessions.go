package inspection

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/jazho76/uplink/internal/inspection/multiplexer"
)

const multiplexerTag = "mux"

var sessionsSection = section{
	script: func(Request) string {
		blocks := make([]string, len(multiplexer.All))
		for i, m := range multiplexer.All {
			blocks[i] = fmt.Sprintf(`if command -v %[1]s >/dev/null 2>&1; then
{ %[2]s
} </dev/null 2>/dev/null | while IFS= read -r row; do printf '%%s\n' "%[3]s $row"; done
fi`, m.Tool(), m.Script(), rowTag(m))
		}
		return strings.Join(blocks, "\n")
	},
	parse: func(_ Request, out string, snapshot *Snapshot) {
		for _, m := range multiplexer.All {
			snapshot.Sessions = append(snapshot.Sessions, clean(m.Tool(), m.Parse(tagged(out, rowTag(m))))...)
		}
	},
}

func rowTag(m multiplexer.Multiplexer) string {
	return multiplexerTag + " " + m.Tool()
}

func clean(tool string, sessions []multiplexer.Session) []multiplexer.Session {
	var kept []multiplexer.Session
	for _, s := range sessions {
		s.Tool, s.Name = tool, ansi.Strip(s.Name)
		if s.Name == "" {
			continue
		}
		var windows []multiplexer.Window
		for _, w := range s.Windows {
			if w.Name = ansi.Strip(w.Name); w.Name != "" {
				windows = append(windows, w)
			}
		}
		s.Windows = windows
		kept = append(kept, s)
	}
	return kept
}
