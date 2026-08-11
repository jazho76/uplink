package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/ui"
	"github.com/spf13/cobra"
)

const configTemplate = `# uplink config. Every section is optional; drop one and its
# provider falls back to built-in defaults.
#
# A mode is a name plus a shell payload. The provider supplies the transport,
# and an empty run lands you in that transport's plain interactive shell.

# local:
#   modes:
#     - { name: tmux, run: tmux new-session -A -s host }
#     - { name: shell }
#     - { name: top, run: htop, back: true }

# lima:
#   modes:
#     - { name: tmux, run: tmux new-session -A -s 0 }
#     - { name: shell }

# Remotes are the one section that declares what exists. Everything but name is
# optional: with no ssh the name is used as the destination, so a Host block in
# ~/.ssh/config can carry the details. A relative identity resolves against this
# directory, never the working directory.
# remotes:
#   - name: box
#     ssh: me@box.example
#     identity: ~/.ssh/box
#     init: <runs first in the login shell, joined to each mode with &&>
#     modes:
#       - { name: tmux, run: tmux }
#       - { name: shell }
`

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and edit the launcher config",
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config in $VISUAL or $EDITOR, then check it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := config.Path()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(configTemplate), 0o644); err != nil {
				return err
			}
		}
		if err := run.Stream(editor(), path); err != nil {
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

func editor() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "vi"
}

func checkConfig() error {
	cfg, loadErr := config.Load()
	state := "not found, using defaults"
	if cfg.Exists() {
		state = "loaded"
	}
	if loadErr != nil {
		state = "unreadable, using defaults"
	}
	ui.Info("config %s (%s)", cfg.Path, state)

	reg, warn := registryFrom(cfg)
	targets, listErr := reg.All()
	for _, t := range targets {
		fmt.Println()
		ui.Info("%s  %s  %s", t.Name, t.Provider, t.Status)
		for i, mode := range t.Modes {
			marker := " "
			if i == 0 {
				marker = "*"
			}
			back := ""
			if mode.Back {
				back = "  (returns to the dashboard)"
			}
			ui.Info("  %s %-10s %s%s", marker, mode.Name, argvForDisplay(mode.Argv), back)
		}
	}

	problems := errors.Join(loadErr, warn, listErr)
	if problems != nil {
		fmt.Println()
	}
	return problems
}

func argvForDisplay(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == "" || strings.ContainsAny(a, " \t'\"") {
			parts = append(parts, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}
