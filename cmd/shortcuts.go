package cmd

import (
	"github.com/jazho76/uplink/internal/shortcuts"
	"github.com/jazho76/uplink/internal/ui"
	"github.com/spf13/cobra"
)

var installShortcutCmd = &cobra.Command{
	Use:   "install-shortcut",
	Short: "Bind Ctrl+Alt+T to open the dashboard in Alacritty (GNOME)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := shortcuts.Install(); err != nil {
			return err
		}
		ui.Info("installed: Ctrl+Alt+T (dashboard)")
		return nil
	},
}

var uninstallShortcutCmd = &cobra.Command{
	Use:   "uninstall-shortcut",
	Short: "Remove the uplink GNOME keybinding",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := shortcuts.Uninstall(); err != nil {
			return err
		}
		ui.Info("removed the uplink keybinding")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installShortcutCmd, uninstallShortcutCmd)
}
