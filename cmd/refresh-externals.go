package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/jazho76/vmm/internal/run"
	"github.com/spf13/cobra"
)

var refreshExternalsCmd = &cobra.Command{
	Use:   "refresh-externals <vm>",
	Short: "Re-fetch a VM's externals and re-apply them into the running VM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpl, err := findTemplate(args[0])
		if err != nil {
			return err
		}
		if info, err := os.Stat(tmpl.FetchExternals()); err != nil || info.Mode()&0o111 == 0 {
			return fmt.Errorf("%s has no executable fetch-externals.sh", tmpl.Name)
		}

		if err := run.Stream(tmpl.FetchExternals()); err != nil {
			return err
		}

		script, err := os.Open(tmpl.ApplyExternals())
		if err != nil {
			return fmt.Errorf("missing provision/apply-externals.sh: %w", err)
		}
		defer script.Close()

		shell := exec.Command("limactl", "shell", tmpl.Name, "--", "bash")
		shell.Stdin = script
		shell.Stdout = os.Stdout
		shell.Stderr = os.Stderr
		return shell.Run()
	},
}

func init() {
	rootCmd.AddCommand(refreshExternalsCmd)
}
