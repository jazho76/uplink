package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
	"github.com/spf13/cobra"
)

var connectMode string

var connectCmd = &cobra.Command{
	Use:   "connect <target>[:<mode>]",
	Short: "Open a shell on a target, starting it if stopped",
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

	reg, warn := registry()
	if warn != nil {
		ui.Error("%s", warn)
	}

	t, provider, err := reg.Resolve(name)
	if err != nil {
		return err
	}

	mode, err := pickMode(t, modeName)
	if err != nil {
		return err
	}

	if !t.Running() {
		if lifecycle, ok := provider.(target.Lifecycle); ok {
			if err := lifecycle.Start(name, os.Stdout); err != nil {
				return err
			}
		}
	}

	return run.Exec(mode.Argv[0], mode.Argv[1:]...)
}

func splitTargetAndMode(spec string) (name, mode string) {
	name, mode, _ = strings.Cut(spec, ":")
	return name, mode
}

func pickMode(t target.Target, name string) (target.Mode, error) {
	if len(t.Modes) == 0 {
		return target.Mode{}, fmt.Errorf("target %q has no launch mode", t.Name)
	}
	if name == "" {
		return t.DefaultMode(), nil
	}
	mode, ok := t.Mode(name)
	if !ok {
		return target.Mode{}, fmt.Errorf("target %q has no mode %q; available: %s",
			t.Name, name, strings.Join(t.ModeNames(), ", "))
	}
	if len(mode.Argv) == 0 {
		return target.Mode{}, fmt.Errorf("mode %q of %q resolves to nothing", name, t.Name)
	}
	return mode, nil
}
