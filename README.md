# Uplink

**One shortcut. Every machine. The same tmux-native workflow everywhere.**

Uplink is the launcher I use to move between my local host, VMs, and remote
machines. One shortcut, choose where you want to work and land in tmux.

![Uplink dashboard](docs/dashboard.png)

## Why I built it

I live almost entirely in the terminal. My browser is usually fullscreen on one
desktop, Slack is fullscreen on another, and everything else happens in tmux.
I care very little about desktop environments or tiling window managers because
tmux is my tiling system. It is the main interface to my computers.

As agents became a larger part of my workflow, I stopped running them on the
local machine. I began giving them isolated VMs and using remote machines over
SSH much more heavily. The environment changed, but the interface did not: tmux
remained the same language everywhere.

What was missing was a good way to get to the right machine without having to
remember which VMs were running, manage a collection of terminal windows, or
treat local, virtual, and remote environments as separate workflows. I wanted
one starting point.

That is Uplink. It binds to a global shortcut and the dashboard is where every
terminal session begins. From there:

- see the local host, VMs, and remotes together;
- check which machines are up and what they are doing;
- choose a launch mode, usually a persistent tmux session, and enter it.

Uplink is not another layer to work inside, and it is not trying to replace
tmux. It is the small piece of glue that gets you to the right tmux session and
then gets out of the way. 

## Install

The release installer supports Linux on x86-64:

```sh
curl -fsSL https://raw.githubusercontent.com/jazho76/uplink/main/scripts/install.sh | sh
```

It installs `uplink` to `~/.local/bin`. If necessary, add that directory to your
`PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

To build from source, install Go 1.26 or newer and run:

```sh
git clone https://github.com/jazho76/uplink.git
cd uplink
make install
```

## Make Uplink your entry point

Run Uplink without arguments to open the dashboard:

```sh
uplink
```

The useful setup is to bind that command to a global shortcut. Then the path
from anywhere on the desktop to any working environment becomes:

1. Press the shortcut.
2. Select a machine.
3. Press Enter.
4. Continue inside tmux.

When the session ends, whether you exit, detach, or lose the connection, you are
back on the dashboard. Press `q` to close it.

On GNOME, Uplink can register this for you:

```sh
uplink install-shortcut
```

| Shortcut | Action |
| --- | --- |
| `Ctrl+Alt+T` | Open the Uplink dashboard in Alacritty |

It also removes stale bindings left by older Uplink versions. Remove the
shortcut with:

```sh
uplink uninstall-shortcut
```

The dashboard itself is not tied to GNOME or Alacritty. On another desktop or
terminal emulator, bind your preferred shortcut to:

```text
<terminal> -e uplink dashboard
```

You can also skip the dashboard and go straight to a target and mode:

```sh
uplink box          # default mode
uplink box:shell
```

## The dashboard

The left side lists targets in the groups your config gives them, in the order
they first appear. The right
side shows the selected target's status, its declared stats, the exact command
each mode runs, and recent logs. To see this machine's resources, give the host
target `stats`, as in the example below.

A target's status is checked while it is selected, and in the background for
targets with `watch: true`; others show as unknown until you visit them. When a
session exits with a non-zero code, the status line says so.

| Key | Action |
| --- | --- |
| `j` / `k`, `↓` / `↑` | Move between targets |
| Highlighted letter | Jump directly to a pane or target |
| `Tab` / `Shift+Tab` | Cycle through launch modes |
| `Enter` | Enter the selected target using the selected mode |
| `Ctrl+L` | Open the target's logs |
| `Ctrl+S` | Start the target (its `up` hook) |
| `Ctrl+X` | Stop the target (its `down` hook) |
| `Ctrl+E` | Edit the config; the dashboard reloads when the editor closes |
| `q`, `Esc`, `Ctrl+C` | Quit, or leave the log view |

## Configuration

Open the config in `$VISUAL`, `$EDITOR`, or `vi`:

```sh
uplink config edit
```

The file lives at `${XDG_CONFIG_HOME:-$HOME/.config}/uplink/config.yaml`.
Without a `targets` section, Uplink offers only the host.

```yaml
x-linux: &linux
  - { type: text, label: up, unit: seconds, value: cut -d ' ' -f 1 /proc/uptime }
  - { type: gauge, label: load, value: cut -d ' ' -f 1 /proc/loadavg, max: nproc }
  - type: gauge
    label: memory
    unit: bytes
    value: awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print (t-a)*1024}' /proc/meminfo
    max: awk '/^MemTotal:/{print $2*1024}' /proc/meminfo
  - type: gauge
    label: disk
    unit: bytes
    value: df -P -k / | awk 'NR==2{print $3*1024}'
    max: df -P -k / | awk 'NR==2{print $2*1024}'

profiles:
  lima:
    group: vms
    via: ssh -t -F ~/.lima/{name}/ssh.config lima-{name}
    status: limactl list --format '{{.Status}}' {name} | grep -qx Running
    up: limactl start {name}
    down: limactl stop {name}
    logs: tail -n 500 ~/.lima/{name}/serial.log
    watch: true
    stats: *linux
    modes:
      - { name: tmux, run: tmux new-session -A -s 0 }
      - { name: shell }

targets:
  - name: host
    group: local
    expand: true
    logs: journalctl --system -n 500 --no-pager -o short
    stats: *linux
    modes:
      - { name: tmux, run: tmux new-session -A -s host }
      - { name: shell }
      - { name: top, run: htop }

  - { name: devbox, profile: lima }

  - name: prod
    group: remotes
    via: ssh -t -i ~/.ssh/prod_ed25519 deploy@example.com
    modes:
      - { name: tmux, run: "bash -lc 'cd /srv/app && tmux new-session -A -s prod'" }
      - { name: shell }
```

Every command runs on one of two sides. **Host-side** fields (`status`, `up`,
`down`, `logs`) run on this machine through `sh -c`. **Target-side** commands
(modes and the stats probe) run inside the target through `via`. A target
without `via` runs here.

### Target fields

| Field | Meaning |
| --- | --- |
| `name` | Name shown in the dashboard and used by `uplink <name>` |
| `group` | Pane the target is listed under; `ungrouped` when left out |
| `via` | Command that reaches the target; leave it out to run on this machine |
| `exec` | `via` for background probes, where no terminal is attached; defaults to `via` |
| `status` | Host-side check, exit 0 means up; defaults to running `true` through `exec` |
| `up` | Host-side start, run by `Ctrl+S` and before entering a target that is down |
| `down` | Host-side stop, run by `Ctrl+X` |
| `logs` | Host-side command whose output fills the log view |
| `watch` | `true` checks status in the background, not only while selected |
| `expand` | `true` gives each mode its own row in the dashboard |
| `stats` | Metrics shown while the target is selected; see [Stats](#stats). None when left out |
| `modes` | Ways to enter the target; the first is the default |
| `profile` | Profile to take every unset field from |

### How `via` works

Uplink runs the `via` words with **one** extra argument: the mode's `run`,
untouched. Without `via`, that is `sh -c <run>` on this machine. That fits
`ssh`, which hands its argument to the remote shell. For tools that take an
argument list instead, end `via` with `sh -c`:

```yaml
profiles:
  docker:
    group: containers
    via: docker exec -it {name} sh -c
    exec: docker exec -i {name} sh -c
    status: "[ \"$(docker inspect -f '{{.State.Running}}' {name})\" = true ]"
    up: docker start {name}
    down: docker stop {name}
    logs: docker logs --tail 500 {name}
    modes:
      - { name: shell }

  pod:
    group: pods
    via: kubectl exec -it {name} -- sh -c
    modes:
      - { name: shell }
```

`exec` exists for transports like `docker exec -it`, which refuse to run without
a terminal. `ssh -t` only warns, so ssh targets can leave it out.

`via` and `exec` are split like a shell command, quotes included, and a leading
`~/` in any word is expanded. Nothing else is: Uplink never runs them through a
local shell.

A mode without `run` opens the target user's shell: `exec "${SHELL:-/bin/sh}"`.
That shell is not a login shell. When a mode needs something from the login
profile, such as a function or `PATH` entry, ask for one:
`run: exec "$SHELL" -l`, or `run: "bash -lc 'lc && tmux'"`.

### Modes

| Field | Meaning |
| --- | --- |
| `name` | Name shown in the dashboard |
| `run` | Shell command to run on the target; omit it to open the user's shell |

### Profiles

A profile holds any target field except `name`. A target with `profile: lima`
takes every field it leaves unset from the profile; `modes` replace the
profile's modes as a whole. `{name}` in `via`, `exec`, `status`, `up`, `down`,
`logs`, and `run` becomes the target's name. Inside a flow mapping, quote values
that contain `{name}`: `{ name: tmux, run: "tmux new-session -A -s {name}" }`.

To share values between targets that do not share a profile, define a YAML
anchor under a top-level key Uplink does not use, such as the `x-linux` list
above, and refer to it with `stats: *linux`.

### Stats

`stats` lists what the live block shows, in order. A profile's list is taken
whole; `stats: []` turns an inherited list off.

```yaml
stats:
  - { type: text, label: up, unit: seconds, value: cut -d ' ' -f 1 /proc/uptime }
  - { type: gauge, label: gpu, value: "nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits", max: 100 }
  - { type: gauge, label: cache, value: du -sb ~/.cache | cut -f1, max: df -B1 --output=size ~ | tail -1, unit: bytes }
  - { type: text, label: agents, value: pgrep -c claude }
```

| Type | Fields | Shows |
| --- | --- | --- |
| `gauge` | `label`, `value`, `max`, `unit` | `value` out of `max` as a bar |
| `text` | `label`, `value`, `unit` | `value`'s output as is, or formatted by `unit` |

`value` and `max` are shell commands run on the target; the first line of
their output is used. A `max` that is a plain number is used as is. `unit:
bytes` formats numbers as sizes and `unit: seconds` as durations; a value that
is not a number then shows `?`. `{name}` works in `value` and `max`.

Uplink has no built-in metrics, because what to run depends on the target's OS.
The `x-linux` list in the example above covers Linux targets. For macOS
targets:

```yaml
x-macos: &macos
  - type: text
    label: up
    unit: seconds
    value: echo $(( $(date +%s) - $(sysctl -n kern.boottime | awk '{print $4}' | tr -d ,) ))
  - { type: gauge, label: load, value: "sysctl -n vm.loadavg | awk '{print $2}'", max: sysctl -n hw.ncpu }
  - type: gauge
    label: memory
    unit: bytes
    value: >-
      vm_stat | awk '/page size of/{p=$8} /Pages active/{a=$3} /Pages wired down/{w=$4}
      /occupied by compressor/{c=$5} END{print (a+w+c)*p}'
    max: sysctl -n hw.memsize
  - type: gauge
    label: disk
    unit: bytes
    value: df -P -k /System/Volumes/Data | awk 'NR==2{print $3*1024}'
    max: df -P -k /System/Volumes/Data | awk 'NR==2{print $2*1024}'
```

All metrics travel in
one script piped through `exec` into `sh`, one round trip per refresh, but each
command runs in its own `sh -c` with no stdin and, where the target has
`timeout`, a 5 second limit. A metric that fails, hangs, or prints no number
shows `?`; the others are unaffected.

### Probes and timeouts

Probes, hooks, and logs run without a terminal, so a transport that wants a
password shows the target as down instead of prompting. Logs give up after 2
seconds, `status` hooks after 5, probes after 30, and `up` and `down` after 10
minutes. Add `-o ConnectTimeout=2` to an ssh `via` to report an unreachable
host sooner.

### Checking the config

Validate the config and inspect the exact command behind every target and mode:

```sh
uplink config check
```

Target names must be unique and may only hold letters, digits, `.`, `_`, and
`-`. An invalid target is skipped and reported, and the rest still load.

## Upgrade

Release builds can update themselves to the latest Linux x86-64 release:

```sh
uplink upgrade
```

For a source build, pull the repository and run `make install` again.

Run `uplink --help` to explore the rest of the CLI.
