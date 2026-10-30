package config

import (
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
