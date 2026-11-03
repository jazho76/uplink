package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFieldGutterSizesToTheLongestKey(t *testing.T) {
	block := ansi.Strip(renderFields([]field{
		{"template", "kyoto_vm"},
		{"cpus", "6"},
		{"ssh", "127.0.0.1:38203"},
	}, 60))

	var starts []int
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		starts = append(starts, strings.Index(line, strings.TrimSpace(strings.SplitN(line, "  ", 2)[1])))
	}
	for i, at := range starts {
		if at != starts[0] {
			t.Errorf("line %d starts its value at column %d, want %d:\n%s", i, at, starts[0], block)
		}
	}
	if !strings.HasPrefix(block, "template  kyoto_vm") {
		t.Errorf("the longest key sets the gutter, got %q", block)
	}
}

func TestFieldValuesNeverOverrunTheirWidth(t *testing.T) {
	block := renderFields([]field{{"dir", "/home/jazho/.lima/kyoto/some/deep/path"}}, 20)
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("field line spans %d cells, want at most 20: %q", w, line)
		}
	}
}

func TestTruncateMeasuresDisplayWidth(t *testing.T) {
	if got := truncate("日本語テスト", 4); lipgloss.Width(got) != 4 {
		t.Errorf("truncate should count display cells, got %q spanning %d", got, lipgloss.Width(got))
	}
}
