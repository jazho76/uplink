package cmd

import (
	"fmt"

	"github.com/jazho76/vmm/internal/lima"
	"github.com/jazho76/vmm/internal/run"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect <instance>",
	Short: "Open a shell in an instance, starting it if stopped",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		inst, exists := lima.Get(name)
		if !exists {
			return fmt.Errorf("no such instance %q; create it with: vmm create <template> [instance]", name)
		}

		if !inst.Running() {
			if err := lima.Start(name); err != nil {
				return err
			}
		}

		argv := lima.ShellArgv(name)
		return run.Exec(argv[0], argv[1:]...)
	},
}

func init() {
	rootCmd.AddCommand(connectCmd)
}
