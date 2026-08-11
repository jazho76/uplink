package cmd

import (
	"github.com/jazho76/uplink/internal/selfupdate"
	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the uplink binary to the latest release",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return selfupdate.Run()
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}
