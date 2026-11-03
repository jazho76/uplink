package config

const template = `# uplink config. Each target is a machine you can open a shell on.
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
# check   host-side check: exit 0 means up, any other exit means down, and a
#         timeout or a missing command means unknown. Left out, uplink tries to
#         reach the target through exec, and failing to means unknown.
# up      host-side start, run by ctrl+s and before entering a down target.
#         Needs a check.
# down    host-side stop, run by ctrl+x.
# logs    host-side command whose output fills the log view (ctrl+l). Left
#         out means the target's journal, when it has journalctl; "" means none.
# watch   true checks status in the background, not only while selected.
# stats   metrics shown while selected, measured in one probe through exec:
#         gauge (label, value, max, unit) and text (label, value, unit).
#         unit is bytes or seconds. A name instead of a metric adds a
#         preset: linux, macos, or container. Left out means the built-in
#         metrics for the target's OS; [] means none.
# modes   ways to enter; the first is the default. A mode without run opens
#         the user's login shell. multiplexer names the tool whose sessions the
#         preview shows; without it a mode shows none. Left out means
#         default_mode, then any other multiplexer, then shell, each only when
#         the target has it. Every connection returns to the dashboard.
#
# {name} in a command becomes the target's name. Top-level keys starting with
# x- hold YAML anchors for values targets share.

# Providers list machines that other tools already know about, and report
# whether each is alive. Without a discover section every provider runs: local
# (this machine), lima (its VMs), docker (its containers) and tailscale (its
# peers). One that isn't available stays quiet until it is. A discover list runs
# exactly the providers it names and reports the ones that aren't available;
# "discover: []" turns discovery off.
# discover: [local, lima]

# default_mode is what targets without modes open by default: tmux, zellij, or
# shell.
# Left out means tmux. If a target lacks the tool, you get its shell instead.
# default_mode: tmux

# targets:
#   - name: box
#     group: machines
#     via: ssh -t me@box.example
#     stats: [linux]
#     modes:
#       - { name: tmux, multiplexer: tmux, run: tmux new-session -A -s 0 }
#       - { name: shell }
`
