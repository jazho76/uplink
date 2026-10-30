package cmd

import (
	"os"

	"github.com/jazho76/uplink/internal/ui"
	"github.com/jazho76/uplink/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "uplink",
	Short:         "Launcher for local, virtual, and remote shells",
	Long:          "Run bare to open the dashboard.",
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dashboard()
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
