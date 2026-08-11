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
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
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
