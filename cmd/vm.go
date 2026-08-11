package cmd

import "github.com/spf13/cobra"

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Manage Lima VMs and their templates",
}

func init() {
	rootCmd.AddCommand(vmCmd)
}
