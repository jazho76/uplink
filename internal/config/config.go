package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const fileName = "config.yaml"

func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fileName
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "uplink", fileName)
}

func Ensure() (string, error) {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		err = os.WriteFile(path, []byte(template), 0o644)
	}
	return path, err
}

func Editor() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "vi"
}
