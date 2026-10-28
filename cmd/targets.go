package cmd

import (
	"errors"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/target"
)

func targets() ([]target.Target, error) {
	cfg, loadErr := config.Load()
	resolved, err := target.Load(cfg)
	return resolved, errors.Join(loadErr, err)
}
