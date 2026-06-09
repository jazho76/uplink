package cmd

import (
	"os"

	"github.com/jazho76/vmm/internal/tui"
	"github.com/jazho76/vmm/internal/ui"
	"github.com/jazho76/vmm/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "vmm",
	Short:         "Lima VM control plane",
	Version:       version.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
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
