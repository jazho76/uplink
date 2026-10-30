package cmd

import (
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
	return tui.Run(func() ([]target.Target, error) { return target.Load(config.Path()) }, target.System{})
}
