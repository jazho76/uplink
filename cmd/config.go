package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/stats"
	"github.com/jazho76/uplink/internal/target"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and edit the launcher config",
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config in $VISUAL or $EDITOR, then check it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Ensure()
		if err != nil {
			return err
		}
		if err := run.Stream(config.Editor(), path); err != nil {
			return err
		}
		return checkConfig()
	},
}

var configCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Resolve every target and show the command each mode runs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return checkConfig()
	},
}

func init() {
	configCmd.AddCommand(configEditCmd, configCheckCmd)
	rootCmd.AddCommand(configCmd)
}

func checkConfig() error {
	path := config.Path()
	state := "not found"
	if _, err := os.Stat(path); err == nil {
		state = "found"
	}
	fmt.Printf("config %s (%s)\n", path, state)

	resolved, problems := inventory.Load(path, providers)
	for _, t := range resolved {
		fmt.Println()
		header := t.ID.String()
		if t.Name != t.ID.Key {
			header += " (" + t.Name + ")"
		}
		header += "  " + t.Group
		if t.Watch {
			header += "  watch"
		}
		fmt.Printf("%s\n", header)
		for _, f := range []struct{ key, value string }{
			{"via", target.Quote(t.Via)},
			{"exec", target.Quote(t.Exec)},
			{"status", t.Status},
			{"up", t.Up},
			{"down", t.Down},
			{"logs", t.Logs},
			{"stats", statNames(t.Stats)},
		} {
			if f.value != "" {
				fmt.Printf("  %-7s %s\n", f.key, f.value)
			}
		}
		for i, mode := range t.Modes {
			marker := " "
			if i == 0 {
				marker = "*"
			}
			fmt.Printf("  %s %-10s %s\n", marker, mode.Name, mode.Command())
		}
	}

	if problems != nil {
		fmt.Println()
	}
	return problems
}

func statNames(metrics []stats.Metric) string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.Label
	}
	return strings.Join(names, ", ")
}
