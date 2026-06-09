package cmd

import (
	"fmt"

	"github.com/jazho76/vmm/internal/templates"
	"github.com/jazho76/vmm/internal/ui"
	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage VM templates",
}

var templateAddCmd = &cobra.Command{
	Use:   "add <git-url> <name>",
	Short: "Clone a template repo into the templates directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := templates.Root()
		if err != nil {
			return err
		}
		ui.Step("Adding template " + args[1])
		tmpl, err := templates.Add(root, args[0], args[1])
		if err != nil {
			return err
		}
		ui.Info("added %s at %s", tmpl.Name, tmpl.Dir)
		return nil
	},
}

var templateUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Pull the latest version of a template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpl, err := findTemplate(args[0])
		if err != nil {
			return err
		}
		ui.Step("Updating template " + tmpl.Name)
		return tmpl.Update()
	},
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed templates",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := templates.Root()
		if err != nil {
			return err
		}
		tmpls, err := templates.All(root)
		if err != nil {
			return err
		}
		if len(tmpls) == 0 {
			ui.Info("no templates installed")
			return nil
		}
		for _, t := range tmpls {
			line := fmt.Sprintf("%-16s %s", t.Name, t.Origin())
			if t.Dirty() {
				line += " (dirty)"
			}
			ui.Info("%s", line)
		}
		return nil
	},
}

var templateRemoveForce bool

var templateRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a template from the templates directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpl, err := findTemplate(args[0])
		if err != nil {
			return err
		}
		if !templateRemoveForce && !ui.Confirm("Delete template "+tmpl.Name+" at "+tmpl.Dir+"?") {
			return nil
		}
		ui.Step("Removing template " + tmpl.Name)
		return tmpl.Remove()
	},
}

func init() {
	templateRemoveCmd.Flags().BoolVar(&templateRemoveForce, "force", false, "skip confirmation")
	templateCmd.AddCommand(templateAddCmd, templateListCmd, templateUpdateCmd, templateRemoveCmd)
	rootCmd.AddCommand(templateCmd)
}
