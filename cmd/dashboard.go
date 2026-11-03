package cmd

import (
	"os/exec"

	"github.com/jazho76/uplink/internal/catalog"
	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/dashboard"
	"github.com/jazho76/uplink/internal/system"
)

func openDashboard() error {
	catalog := catalog.Catalog{Path: config.Path(), Providers: providers}
	edit := func() (*exec.Cmd, error) {
		path, err := config.Ensure()
		if err != nil {
			return nil, err
		}
		return exec.Command(config.Editor(), path), nil
	}
	return dashboard.Run(system.NewRunner(system.Commands{}, catalog), edit)
}
