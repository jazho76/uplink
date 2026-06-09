package cmd

import (
	"github.com/jazho76/vmm/internal/clipboard"
	"github.com/jazho76/vmm/internal/ui"
	"github.com/spf13/cobra"
)

var clipboardCmd = &cobra.Command{
	Use:   "push-clipboard [vm]",
	Short: "Copy the host clipboard into a VM (defaults to the sole running VM)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var name string
		if len(args) == 1 {
			name = args[0]
		}
		summary, err := clipboard.Push(name)
		if err != nil {
			clipboard.Notify(false)
			return err
		}
		clipboard.Notify(true)
		ui.Info("%s", summary)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(clipboardCmd)
}
