package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jazho76/uplink/internal/lima"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/templates"
	"github.com/spf13/cobra"
)

var refreshExternalsCmd = &cobra.Command{
	Use:   "refresh-externals <instance>",
	Short: "Re-fetch an instance's externals and re-apply them into the running instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		inst, exists := lima.Get(name)
		if !exists {
			return fmt.Errorf("no such instance %q", name)
		}
		if inst.TemplateDir == "" {
			return fmt.Errorf("instance %q was not created by uplink", name)
		}
		tmpl := templates.Template{Name: filepath.Base(inst.TemplateDir), Dir: inst.TemplateDir}

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

		shell := exec.Command("limactl", "shell", name, "--", "bash")
		shell.Stdin = script
		shell.Stdout = os.Stdout
		shell.Stderr = os.Stderr
		return shell.Run()
	},
}

func init() {
	rootCmd.AddCommand(refreshExternalsCmd)
}
