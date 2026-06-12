package cmd

import (
	"fmt"
	"os"

	"github.com/jazho76/vmm/internal/lima"
	"github.com/jazho76/vmm/internal/run"
	"github.com/jazho76/vmm/internal/templates"
	"github.com/jazho76/vmm/internal/ui"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <template> [instance]",
	Short: "Create and provision a VM from a template",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpl, err := findTemplate(args[0])
		if err != nil {
			return err
		}
		instance := tmpl.Name
		if len(args) == 2 {
			instance = args[1]
		}
		if err := templates.ValidName(instance); err != nil {
			return err
		}
		if _, exists := lima.Get(instance); exists {
			return fmt.Errorf("instance %q already exists", instance)
		}
		return createVM(tmpl, instance)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}

func findTemplate(name string) (templates.Template, error) {
	root, err := templates.Root()
	if err != nil {
		return templates.Template{}, err
	}
	return templates.Find(root, name)
}

func createVM(tmpl templates.Template, instance string) error {
	if info, err := os.Stat(tmpl.FetchExternals()); err == nil && info.Mode()&0o111 != 0 {
		ui.Step("Fetching externals for " + instance)
		if err := run.Stream(tmpl.FetchExternals()); err != nil {
			return err
		}
	}
	ui.Step("Creating + provisioning " + instance + " from " + tmpl.Name + " (several minutes)")
	return lima.Create(instance, tmpl.File(), "TemplateDir="+tmpl.Dir)
}
