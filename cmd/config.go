package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jazho76/uplink/internal/catalog"
	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/process"
	"github.com/jazho76/uplink/internal/shell"
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
		if err := process.Stream(config.Editor(), path); err != nil {
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

	resolved, problems := catalog.Load(context.Background(), catalog.Catalog{Path: path, Providers: providers})
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
			{"via", shell.Quote(t.Reach.Interactive)},
			{"exec", shell.Quote(t.Reach.Background)},
			{"check", t.Check},
			{"up", t.StartCommand()},
			{"down", t.StopCommand()},
			{"logs", logsSummary(t)},
			{"stats", statsSummary(t)},
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
			fmt.Printf("  %s %-10s %s\n", marker, mode.Name, shell.Quote(mode.Argv))
		}
	}

	if problems != nil {
		fmt.Println()
	}
	return problems
}

func logsSummary(t target.Target) string {
	if t.Logs.Detected() {
		return "detected in the target"
	}
	return t.Logs.Command
}

func statsSummary(t target.Target) string {
	if t.DetectsMetrics() {
		return "built-in (by OS)"
	}
	names := make([]string, len(t.Metrics))
	for i, m := range t.Metrics {
		names[i] = m.Label
	}
	return strings.Join(names, ", ")
}
