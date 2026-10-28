package cmd

import (
	"fmt"
	"strings"

	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
	"github.com/spf13/cobra"
)

var connectMode string

var connectCmd = &cobra.Command{
	Use:   "connect <target>[:<mode>]",
	Short: "Open a shell on a target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return connect(args[0], connectMode)
	},
}

func init() {
	connectCmd.Flags().StringVar(&connectMode, "mode", "", "launch mode (defaults to the target's first)")
	rootCmd.AddCommand(connectCmd)
}

func connect(spec, modeName string) error {
	name, inline := splitTargetAndMode(spec)
	if modeName == "" {
		modeName = inline
	}

	resolved, warn := targets()
	if warn != nil {
		ui.Error("%s", warn)
	}

	t, err := target.Resolve(resolved, name)
	if err != nil {
		return err
	}

	mode, err := pickMode(t, modeName)
	if err != nil {
		return err
	}

	return run.Exec(mode.Argv[0], mode.Argv[1:]...)
}

func splitTargetAndMode(spec string) (name, mode string) {
	name, mode, _ = strings.Cut(spec, ":")
	return name, mode
}

func pickMode(t target.Target, name string) (target.Mode, error) {
	if name == "" {
		return t.DefaultMode(), nil
	}
	mode, ok := t.Mode(name)
	if !ok {
		return target.Mode{}, fmt.Errorf("target %q has no mode %q; available: %s",
			t.Name, name, strings.Join(t.ModeNames(), ", "))
	}
	return mode, nil
}
