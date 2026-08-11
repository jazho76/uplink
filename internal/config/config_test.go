package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type modes struct {
	Modes []struct {
		Name string `yaml:"name"`
		Run  string `yaml:"run"`
	} `yaml:"modes"`
}

func write(t *testing.T, body string) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return f
}

func TestMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), fileName)
	f, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("a missing config must load cleanly, got %v", err)
	}
	if f.Exists() {
		t.Error("Exists should be false")
	}

	var into modes
	found, err := f.Section("local", &into)
	if found || err != nil {
		t.Errorf("found=%v err=%v, want false and nil", found, err)
	}
}

func TestEmptyFileHasNoSections(t *testing.T) {
	f := write(t, "")
	var into modes
	if found, err := f.Section("local", &into); found || err != nil {
		t.Errorf("found=%v err=%v", found, err)
	}
}

func TestSectionsDecodeIndependently(t *testing.T) {
	f := write(t, `
local:
  modes:
    - { name: tmux, run: tmux new-session }
lima:
  modes:
    - { name: shell }
remotes:
  - name: dojo
    ssh: me@dojo
`)

	var localCfg modes
	found, err := f.Section("local", &localCfg)
	if !found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(localCfg.Modes) != 1 || localCfg.Modes[0].Run != "tmux new-session" {
		t.Errorf("local decoded as %+v", localCfg.Modes)
	}

	var limaCfg modes
	if _, err := f.Section("lima", &limaCfg); err != nil {
		t.Fatalf("lima: %v", err)
	}
	if len(limaCfg.Modes) != 1 || limaCfg.Modes[0].Name != "shell" {
		t.Errorf("lima decoded as %+v", limaCfg.Modes)
	}

	var remotes []struct {
		Name string `yaml:"name"`
		SSH  string `yaml:"ssh"`
	}
	if _, err := f.Section("remotes", &remotes); err != nil {
		t.Fatalf("remotes: %v", err)
	}
	if len(remotes) != 1 || remotes[0].SSH != "me@dojo" {
		t.Errorf("remotes decoded as %+v", remotes)
	}

	if _, err := f.Section("absent", &limaCfg); err != nil {
		t.Errorf("an absent section is not an error: %v", err)
	}
}

func TestMalformedYAMLNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, []byte("local:\n  modes: [ { name: tmux\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFrom(path)
	if err == nil {
		t.Fatal("malformed yaml must report an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should name the file, got %v", err)
	}
	if f == nil {
		t.Fatal("a usable File must be returned even on error")
	}
}

func TestWrongSectionShapeNamesTheSection(t *testing.T) {
	f := write(t, "local: not-a-mapping\n")

	var into modes
	found, err := f.Section("local", &into)
	if !found {
		t.Error("the section is present, however malformed")
	}
	if err == nil {
		t.Fatal("decoding a scalar into a struct must fail")
	}
	if !strings.Contains(err.Error(), "local") {
		t.Errorf("error should name the section, got %v", err)
	}
}

func TestDirIsWhereRelativePathsResolve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", f.Dir(), dir)
	}
}

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
