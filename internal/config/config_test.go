package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jazho76/uplink/internal/target"
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

func TestTemplateLoadsCleanly(t *testing.T) {
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := target.Read(path)
	if err != nil {
		t.Fatalf("the template should read cleanly: %v", err)
	}
	if _, err := target.Merge(doc.Profiles, doc.Config()); err != nil {
		t.Errorf("the template's targets should resolve: %v", err)
	}
}
