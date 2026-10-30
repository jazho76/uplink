package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathHonorsXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, want := Path(), filepath.Join("/xdg", "uplink", fileName); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got, want := Path(), filepath.Join(home, ".config", "uplink", fileName); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
