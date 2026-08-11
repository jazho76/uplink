package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const fileName = "config.yaml"

type File struct {
	Path     string
	sections map[string]yaml.Node
}

func Dir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "uplink")
}

func Path() string { return filepath.Join(Dir(), fileName) }

func Load() (*File, error) { return LoadFrom(Path()) }

func LoadFrom(path string) (*File, error) {
	f := &File{Path: path, sections: map[string]yaml.Node{}}

	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return f, nil
	case err != nil:
		return f, err
	}

	if err := yaml.Unmarshal(data, &f.sections); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func (f *File) Section(name string, into any) (bool, error) {
	node, ok := f.sections[name]
	if !ok {
		return false, nil
	}
	if err := node.Decode(into); err != nil {
		return true, fmt.Errorf("%s: section %q: %w", f.Path, name, err)
	}
	return true, nil
}

func (f *File) Dir() string { return filepath.Dir(f.Path) }

func (f *File) Exists() bool {
	_, err := os.Stat(f.Path)
	return err == nil
}
