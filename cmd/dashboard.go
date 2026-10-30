package cmd

import (
	"os/exec"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/tui"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open the interactive launcher dashboard",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dashboard()
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}

func dashboard() error {
	load := func() ([]target.Target, error) { return target.Load(config.Path()) }
	edit := func() (*exec.Cmd, error) {
		path, err := config.Ensure()
		if err != nil {
			return nil, err
		}
		return exec.Command(config.Editor(), path), nil
	}
	return tui.Run(load, edit, target.System{})
}
