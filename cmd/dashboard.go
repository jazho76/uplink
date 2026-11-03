package cmd

import (
	"os/exec"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/dashboard"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/launcher"
	"github.com/jazho76/uplink/internal/target"
)

func openDashboard() error {
	load := func() ([]target.Target, error) { return inventory.Load(config.Path(), providers) }
	edit := func() (*exec.Cmd, error) {
		path, err := config.Ensure()
		if err != nil {
			return nil, err
		}
		return exec.Command(config.Editor(), path), nil
	}
	return dashboard.Run(load, edit, launcher.System{})
}
