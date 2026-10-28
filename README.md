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

The left side groups targets by kind: `local`, `vms`, and `remotes`. The right
side shows the selected target's status, the exact command each mode runs, live
load, memory, disk, and uptime, and recent logs. The bottom bar keeps the host's
resource usage visible while you browse other machines.

A target's status is learned by probing it while it is selected, so targets you
have not visited yet show as unknown.

| Key | Action |
| --- | --- |
| `j` / `k`, `↓` / `↑` | Move between targets |
| Highlighted letter | Jump directly to a pane or target |
| `Tab` / `Shift+Tab` | Cycle through launch modes |
| `Enter` | Enter the selected target using the selected mode |
| `Ctrl+L` | Open the target's logs |
| `q`, `Esc`, `Ctrl+C` | Quit, or leave the log view |

## Configuration

Open the config in `$VISUAL`, `$EDITOR`, or `vi`:

```sh
uplink config edit
```

The file lives at `${XDG_CONFIG_HOME:-$HOME/.config}/uplink/config.yaml`.
Without a `targets` section, Uplink offers only the host.

```yaml
profiles:
  lima:
    kind: vm
    via: ssh -t -F ~/.lima/{name}/ssh.config lima-{name}
    logs: tail -n 500 ~/.lima/{name}/serial.log
    modes:
      - { name: tmux, run: tmux new-session -A -s 0 }
      - { name: shell }

targets:
  - name: host
    kind: local
    logs: journalctl --system -n 500 --no-pager -o short
    modes:
      - { name: tmux, run: tmux new-session -A -s host }
      - { name: shell }
      - { name: top, run: htop, back: true }

  - { name: devbox, profile: lima }

  - name: prod
    kind: remote
    via: ssh -t -i ~/.ssh/prod_ed25519 deploy@example.com
    modes:
      - { name: tmux, run: "bash -lc 'cd /srv/app && tmux new-session -A -s prod'" }
      - { name: shell }
```

### Target fields

| Field | Meaning |
| --- | --- |
| `name` | Name shown in the dashboard and used by `uplink <name>` |
| `kind` | `local`, `vm`, or `remote`; only picks the pane and glyph |
| `via` | Command that reaches the target; required unless `kind` is `local` |
| `logs` | Host command whose output fills the log view |
| `stats` | `false` skips the load, memory, and disk probe |
| `modes` | Ways to enter the target; the first is the default |
| `profile` | Profile to take every unset field from |

Exactly one target is `local`: the machine Uplink runs on. It takes no `via`,
runs its modes through `$SHELL -c`, and feeds the bottom bar. Each of its modes
gets its own row.

### How `via` works

Uplink runs the `via` words with **one** extra argument: the mode's `run`,
untouched. A mode without `run` runs `via` alone. That fits `ssh`, which hands
its argument to the remote shell. For tools that take an argument list instead,
end `via` with `sh -c`:

```yaml
profiles:
  docker:
    kind: vm
    via: docker exec -it {name} sh -c
    logs: docker logs --tail 500 {name}
    modes:
      - { name: shell, run: exec bash }

  pod:
    kind: remote
    via: kubectl exec -it {name} -- sh -c
    modes:
      - { name: shell, run: exec sh }
```

Such transports cannot open a shell with no command, so give every mode a `run`.

`via` is split like a shell command, quotes included, and a leading `~/` in any
word is expanded. Nothing else is: Uplink never runs `via` through a local
shell.

The remote side runs `run` with its default, non-login shell. When a mode needs
something from the login profile, such as a function or `PATH` entry, wrap it:
`run: "bash -lc 'lc && tmux'"`.

### Modes

| Field | Meaning |
| --- | --- |
| `name` | Name shown in the dashboard |
| `run` | Shell command to run on the target; omit it to open `via` alone |
| `back` | Return to the dashboard when the command exits |

### Profiles

A profile holds any target field except `name`. A target with `profile: lima`
takes every field it leaves unset from the profile; `modes` replace the
profile's modes as a whole. `{name}` in `via`, `logs`, and `run` becomes the
target's name. Inside a flow mapping, quote values that contain `{name}`:
`{ name: tmux, run: "tmux new-session -A -s {name}" }`.

### Probes and timeouts

Stats come from a small POSIX script piped through `via` into `sh`, so the
target needs `/proc`; set `stats: false` otherwise, and Uplink only checks that
`via true` succeeds. Probes and logs run without a terminal and give up after a
few seconds, so a transport that wants a password shows the target as down
instead of prompting.

### Checking the config

Validate the config and inspect the exact command behind every target and mode:

```sh
uplink config check
```

Target names must be unique. An invalid target is skipped and reported, and the
rest still load.

## Upgrade

Release builds can update themselves to the latest Linux x86-64 release:

```sh
uplink upgrade
```

For a source build, pull the repository and run `make install` again.

Run `uplink --help` to explore the rest of the CLI.
