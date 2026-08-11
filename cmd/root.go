package cmd

import (
	"os"

	"github.com/jazho76/uplink/internal/ui"
	"github.com/jazho76/uplink/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "uplink [target][:mode]",
	Short:         "Launcher for local, virtual, and remote shells",
	Long:          "Run bare to open the dashboard, or with a target to connect to it directly.",
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return dashboard()
		}
		return connect(args[0], "")
	},
}

func init() {
	rootCmd.SetVersionTemplate(version.String() + "\n")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		ui.Error("%s", err)
		os.Exit(1)
	}
}
