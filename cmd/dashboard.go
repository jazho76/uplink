package cmd

import (
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
	reg, warn := registry()
	return tui.Run(reg, warn)
}
