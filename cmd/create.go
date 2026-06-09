package cmd

import (
	"os"

	"github.com/jazho76/vms/internal/lima"
	"github.com/jazho76/vms/internal/profiles"
	"github.com/jazho76/vms/internal/run"
	"github.com/jazho76/vms/internal/ui"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <vm>",
	Short: "Create and provision a VM from its profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prof, err := findProfile(args[0])
		if err != nil {
			return err
		}
		return createVM(prof)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}

func findProfile(name string) (profiles.Profile, error) {
	root, err := profiles.Root()
	if err != nil {
		return profiles.Profile{}, err
	}
	return profiles.Find(root, name)
}

func createVM(prof profiles.Profile) error {
	if info, err := os.Stat(prof.FetchExternals()); err == nil && info.Mode()&0o111 != 0 {
		ui.Step("Fetching externals for " + prof.Name)
		if err := run.Stream(prof.FetchExternals()); err != nil {
			return err
		}
	}
	ui.Step("Creating + provisioning " + prof.Name + " (several minutes)")
	return lima.Create(prof.Name, prof.File(), "ProfileDir="+prof.Dir)
}
