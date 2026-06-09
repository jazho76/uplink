package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/jazho76/vms/internal/run"
	"github.com/spf13/cobra"
)

var refreshExternalsCmd = &cobra.Command{
	Use:   "refresh-externals <vm>",
	Short: "Re-fetch a VM's externals and re-apply them into the running VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prof, err := findProfile(args[0])
		if err != nil {
			return err
		}
		if info, err := os.Stat(prof.FetchExternals()); err != nil || info.Mode()&0o111 == 0 {
			return fmt.Errorf("%s has no executable fetch-externals.sh", prof.Name)
		}

		if err := run.Stream(prof.FetchExternals()); err != nil {
			return err
		}

		script, err := os.Open(prof.ApplyExternals())
		if err != nil {
			return fmt.Errorf("missing provision/apply-externals.sh: %w", err)
		}
		defer script.Close()

		shell := exec.Command("limactl", "shell", prof.Name, "--", "bash")
		shell.Stdin = script
		shell.Stdout = os.Stdout
		shell.Stderr = os.Stderr
		return shell.Run()
	},
}

func init() {
	rootCmd.AddCommand(refreshExternalsCmd)
}
