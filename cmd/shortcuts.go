package cmd

import (
	"github.com/jazho76/vms/internal/shortcuts"
	"github.com/jazho76/vms/internal/ui"
	"github.com/spf13/cobra"
)

var installShortcutsCmd = &cobra.Command{
	Use:   "install-shortcuts",
	Short: "Register GNOME global keybindings (Ctrl+Alt+T launcher, Ctrl+Alt+P clipboard)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := shortcuts.Install(); err != nil {
			return err
		}
		ui.Info("installed: Ctrl+Alt+T (launcher), Ctrl+Alt+P (push clipboard)")
		return nil
	},
}

var uninstallShortcutsCmd = &cobra.Command{
	Use:   "uninstall-shortcuts",
	Short: "Remove the VMs GNOME global keybindings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := shortcuts.Uninstall(); err != nil {
			return err
		}
		ui.Info("removed VMs keybindings")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installShortcutsCmd, uninstallShortcutsCmd)
}
