package cmd

import (
	"os/exec"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/dashboard"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/launcher"
)

func openDashboard() error {
	catalog := inventory.Catalog{Path: config.Path(), Providers: providers}
	edit := func() (*exec.Cmd, error) {
		path, err := config.Ensure()
		if err != nil {
			return nil, err
		}
		return exec.Command(config.Editor(), path), nil
	}
	return dashboard.Run(launcher.NewRunner(launcher.System{}, catalog), edit)
}
