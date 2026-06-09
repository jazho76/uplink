package cmd

import (
	"github.com/jazho76/vmm/internal/lima"
	"github.com/jazho76/vmm/internal/run"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect <vm>",
	Short: "Open a shell in a VM, creating or starting it as needed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		inst, exists := lima.Get(name)
		if !exists {
			prof, err := findProfile(name)
			if err != nil {
				return err
			}
			if err := createVM(prof); err != nil {
				return err
			}
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
