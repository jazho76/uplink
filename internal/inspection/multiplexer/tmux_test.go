package multiplexer

import (
	"reflect"
	"testing"

	"github.com/jazho76/uplink/internal/target"
)

func TestTmuxJoinsWindowsToTheirSessions(t *testing.T) {
	got := Tmux{}.Parse([]string{
		"session $1 1 main",
		"session $0 0 my long: name",
		"window $0 1 notes",
		"window $1 2 my editor",
		"window $1 1 logs",
	})
	want := []target.Session{
		{Name: "main", Attached: true, Windows: []target.Window{{Name: "my editor", Panes: 2}, {Name: "logs", Panes: 1}}},
		{Name: "my long: name", Windows: []target.Window{{Name: "notes", Panes: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestTmuxDropsSessionRowsItCannotRead(t *testing.T) {
	got := Tmux{}.Parse([]string{"session", "session x 0 name", "session $0 y name", "session $0 0", "window $0 1 orphan"})
	if len(got) != 0 {
		t.Errorf("malformed rows should be dropped, got %+v", got)
	}
}

func TestTmuxDropsWindowRowsItCannotRead(t *testing.T) {
	got := Tmux{}.Parse([]string{"session $0 0 main", "window $0 x bad", "window $0 1", "window $9 1 stranger", "window $0 1 good"})
	want := []target.Session{{Name: "main", Windows: []target.Window{{Name: "good", Panes: 1}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want only the well-formed window, got %+v", got)
	}
}
