package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jazho76/vms/internal/run"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs <vm>",
	Short: "Tail a VM's serial console log",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		pattern := filepath.Join(home, ".lima", args[0], "serial*.log")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no serial logs for %s", args[0])
		}
		return run.Exec("tail", append([]string{"-f"}, matches...)...)
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
