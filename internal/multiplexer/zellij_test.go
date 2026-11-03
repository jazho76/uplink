package multiplexer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func fixtureRows(t *testing.T, name, tag string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		rows = append(rows, tag+" "+line)
	}
	return rows
}

func TestZellijReadsRealOutput(t *testing.T) {
	rows := append([]string{"session 0"}, fixtureRows(t, "zellij-clients.txt", zellijClientTag)...)
	rows = append(rows, fixtureRows(t, "zellij-panes.json", zellijPaneTag)...)

	want := []target.Session{{Name: "0", Attached: true, Windows: []target.Window{
		{Name: "Tab #1", Panes: 1},
		{Name: "a  terminal_9  terminal  x", Panes: 2},
		{Name: "notes", Panes: 1},
	}}}
	if got := (Zellij{}).Parse(rows); !reflect.DeepEqual(got, want) {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestZellijAttachedNeedsAClient(t *testing.T) {
	for name, rows := range map[string][]string{
		"header only": {"session main", "client CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND"},
		"no rows":     {"session main"},
		"blank row":   {"session main", "client "},
	} {
		got := Zellij{}.Parse(rows)
		if len(got) != 1 || got[0].Attached {
			t.Errorf("%s: want one unattached session, got %+v", name, got)
		}
	}

	got := Zellij{}.Parse([]string{"session main", "client 1         terminal_2     N/A"})
	if len(got) != 1 || !got[0].Attached {
		t.Errorf("a client row means attached, got %+v", got)
	}
}

func TestZellijClientsBelongToTheirSession(t *testing.T) {
	got := Zellij{}.Parse([]string{"session a", "client 1 terminal_0 N/A", "session b"})
	if len(got) != 2 || !got[0].Attached || got[1].Attached {
		t.Errorf("want only a attached, got %+v", got)
	}
}

func TestZellijOrdersTabsByPosition(t *testing.T) {
	got := Zellij{}.Parse([]string{"session main",
		`pane [{"tab_id":7,"tab_position":1,"tab_name":"second"},{"tab_id":3,"tab_position":0,"tab_name":"first"},{"tab_id":7,"tab_position":1,"tab_name":"second"}]`})
	want := []target.Window{{Name: "first", Panes: 1}, {Name: "second", Panes: 2}}
	if len(got) != 1 || !reflect.DeepEqual(got[0].Windows, want) {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestZellijTabWithOnlyPluginsIsNotListed(t *testing.T) {
	got := Zellij{}.Parse([]string{"session main", `pane [{"is_plugin":true,"tab_id":0,"tab_position":0,"tab_name":"bar"}]`})
	if len(got) != 1 || got[0].Windows != nil {
		t.Errorf("plugins are not windows, got %+v", got)
	}
}

func TestZellijSessionSurvivesUnreadablePanes(t *testing.T) {
	for name, rows := range map[string][]string{
		"broken json": {"session main", "pane [{not json"},
		"no panes":    {"session main"},
	} {
		got := Zellij{}.Parse(rows)
		want := []target.Session{{Name: "main"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: an older zellij should still show the name, got %+v", name, got)
		}
	}
}

func TestZellijSessionsAreSortedByNameAndKeepSpaces(t *testing.T) {
	got := Zellij{}.Parse([]string{"session my beta", `pane [{"tab_id":0,"tab_position":0,"tab_name":"b"}]`, "session alpha", `pane [{"tab_id":0,"tab_position":0,"tab_name":"a"}]`})
	want := []target.Session{
		{Name: "alpha", Windows: []target.Window{{Name: "a", Panes: 1}}},
		{Name: "my beta", Windows: []target.Window{{Name: "b", Panes: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestZellijIgnoresRowsBeforeAnySession(t *testing.T) {
	if got := (Zellij{}).Parse([]string{"client 1 terminal_0 N/A", "pane []", "other"}); len(got) != 0 {
		t.Errorf("want no sessions, got %+v", got)
	}
}
