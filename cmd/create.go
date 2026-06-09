package cmd

import (
	"os"

	"github.com/jazho76/vmm/internal/lima"
	"github.com/jazho76/vmm/internal/templates"
	"github.com/jazho76/vmm/internal/run"
	"github.com/jazho76/vmm/internal/ui"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <vm>",
	Short: "Create and provision a VM from its template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpl, err := findTemplate(args[0])
		if err != nil {
			return err
		}
		return createVM(tmpl)
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

func createVM(tmpl templates.Template) error {
	if info, err := os.Stat(tmpl.FetchExternals()); err == nil && info.Mode()&0o111 != 0 {
		ui.Step("Fetching externals for " + tmpl.Name)
		if err := run.Stream(tmpl.FetchExternals()); err != nil {
			return err
		}
	}
	ui.Step("Creating + provisioning " + tmpl.Name + " (several minutes)")
	return lima.Create(tmpl.Name, tmpl.File(), "TemplateDir="+tmpl.Dir)
}
