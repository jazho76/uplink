package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
	"github.com/spf13/cobra"
)

const configTemplate = `# uplink config. Each target is a machine you can open a shell on.
#
# kind   local, vm or remote; only picks the pane and glyph. Exactly one
#        target is local: this machine, reached without via.
# via    command that reaches the target. uplink appends one argument, the
#        mode's shell command, so argv-style tools end in "sh -c".
# logs   host command whose output fills the log view (ctrl+l).
# stats  false skips the load, memory and disk probe.
# modes  ways to enter; the first is the default. A mode without run opens
#        via alone; back: true returns to the dashboard when it exits.
#
# A profile holds shared fields; {name} becomes the target's name.

# profiles:
#   lima:
#     kind: vm
#     via: ssh -t -F ~/.lima/{name}/ssh.config lima-{name}
#     logs: tail -n 500 ~/.lima/{name}/serial.log
#     modes:
#       - { name: tmux, run: tmux new-session -A -s 0 }
#       - { name: shell }

# targets:
#   - name: host
#     kind: local
#     modes:
#       - { name: tmux, run: tmux new-session -A -s host }
#       - { name: shell }
#       - { name: top, run: htop, back: true }
#   - { name: devbox, profile: lima }
#   - name: box
#     kind: remote
#     via: ssh -t me@box.example
#     modes:
#       - { name: tmux, run: tmux new-session -A -s 0 }
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
	state := "not found, only the default host"
	if cfg.Exists() {
		state = "loaded"
	}
	if loadErr != nil {
		state = "unreadable"
	}
	ui.Info("config %s (%s)", cfg.Path, state)

	resolved, problems := target.Load(cfg)
	for _, t := range resolved {
		fmt.Println()
		ui.Info("%s  %s", t.Name, t.Kind)
		if len(t.Via) > 0 {
			ui.Info("  via   %s", target.Quote(t.Via))
		}
		if t.Logs != "" {
			ui.Info("  logs  %s", t.Logs)
		}
		for i, mode := range t.Modes {
			marker := " "
			if i == 0 {
				marker = "*"
			}
			back := ""
			if mode.Back {
				back = "  (returns to the dashboard)"
			}
			ui.Info("  %s %-10s %s%s", marker, mode.Name, mode.Command(), back)
		}
	}

	problems = errors.Join(loadErr, problems)
	if problems != nil {
		fmt.Println()
	}
	return problems
}
