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

# Providers list machines that other tools already know about. Without a
# discover section, local (this machine) runs; "discover: []" turns discovery
# off. An entry's profile overrides the provider's group, stats, logs and modes.
# discover: []

# targets:
  # - { name: devbox, profile: lima }
  # - name: box
  #   group: machines
  #   via: ssh -t me@box.example
  #   modes:
  #     - { name: tmux, run: tmux new-session -A -s 0 }
  #     - { name: shell }
`
