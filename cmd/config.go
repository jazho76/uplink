package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/probe"
	"github.com/jazho76/uplink/internal/run"
	"github.com/jazho76/uplink/internal/target"
	"github.com/jazho76/uplink/internal/ui"
	"github.com/spf13/cobra"
)

const configTemplate = `# uplink config. Each target is a machine you can open a shell on.
#
# Host-side fields run on this machine through sh -c; target-side ones run
# inside the target through via (or exec).
#
# group   pane the target is listed under; "ungrouped" when left out.
# via     command that reaches the target. uplink appends one argument, the
#         mode's shell command, so argv-style tools end in "sh -c". Leave it
#         out to run on this machine.
# exec    via for background probes, where no terminal is attached; defaults
#         to via. Needed by tools that refuse -t without a terminal (docker).
# status  host-side check, exit 0 means up; defaults to running true via exec.
# up      host-side start, run by ctrl+s and before entering a down target.
# down    host-side stop, run by ctrl+x.
# logs    host-side command whose output fills the log view (ctrl+l).
# watch   true polls status in the background, not only while selected.
# stats   metrics shown while selected, measured in one probe through exec:
#         gauge (label, value, max, unit) and text (label, value, unit).
#         unit is bytes or seconds. Left out means none. The README has a
#         macOS list.
# modes   ways to enter; the first is the default. A mode without run opens
#         the user's shell. Every session returns to the dashboard.
#
# A profile holds shared fields; {name} becomes the target's name.

# x-linux: &linux
#   - { type: text, label: up, unit: seconds, value: cut -d ' ' -f 1 /proc/uptime }
#   - { type: gauge, label: load, value: cut -d ' ' -f 1 /proc/loadavg, max: nproc }
#   - type: gauge
#     label: memory
#     unit: bytes
#     value: awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print (t-a)*1024}' /proc/meminfo
#     max: awk '/^MemTotal:/{print $2*1024}' /proc/meminfo
#   - type: gauge
#     label: disk
#     unit: bytes
#     value: df -P -k / | awk 'NR==2{print $3*1024}'
#     max: df -P -k / | awk 'NR==2{print $2*1024}'

# profiles:
#   lima:
#     group: vms
#     via: ssh -t -F ~/.lima/{name}/ssh.config lima-{name}
#     status: limactl list --format '{{.Status}}' {name} | grep -qx Running
#     up: limactl start {name}
#     down: limactl stop {name}
#     logs: tail -n 500 ~/.lima/{name}/serial.log
#     watch: true
#     stats: *linux
#     modes:
#       - { name: tmux, run: tmux new-session -A -s 0 }
#       - { name: shell }

# targets:
#   - name: host
#     group: local
#     stats: *linux
#     modes:
#       - { name: tmux, run: tmux new-session -A -s host }
#       - { name: shell }
#       - { name: top, run: htop }
#   - { name: devbox, profile: lima }
#   - name: box
#     group: remotes
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
		header := t.Name + "  " + t.Group
		if t.Watch {
			header += "  watch"
		}
		ui.Info("%s", header)
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
				ui.Info("  %-7s %s", f.key, f.value)
			}
		}
		for i, mode := range t.Modes {
			marker := " "
			if i == 0 {
				marker = "*"
			}
			ui.Info("  %s %-10s %s", marker, mode.Name, mode.Command())
		}
	}

	problems = errors.Join(loadErr, problems)
	if problems != nil {
		fmt.Println()
	}
	return problems
}

func statNames(metrics probe.Metrics) string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.Label
	}
	return strings.Join(names, ", ")
}
