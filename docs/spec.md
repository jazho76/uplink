# Uplink

Uplink is a launcher for work environments. You pick one from a dashboard and land in it. When you leave, you return to the dashboard.

## Concepts

### Target

A place you can work: this machine, a VM, a container or a remote host. A target describes how to reach it, and optionally how to check, start and stop it.

### Provider

Automatically discovers one type of target. Today: local, lima, docker and tailscale. A provider that fails or is unavailable only costs its own targets.

### Config

Adds custom targets and chooses which providers run. Everything works without a config. An invalid target is skipped and reported, without affecting the others. A config file that can't be parsed, or that has unknown fields, is reported and yields no targets.

### Inspection

How uplink learns about a target's environment: which tools it has, its stats, and its multiplexer sessions. It runs inside the target, and only while the target is up.

### Mode

A way into a target, such as tmux, zellij or a plain shell. Modes are discovered through inspection, and you can cycle through them in the dashboard. The config can instead declare a target's modes explicitly, as custom ways of accessing it, such as a tmux session as another user. Declared modes replace the discovered ones.

### Default mode

The preferred mode, tmux unless configured otherwise. While uplink doesn't know whether a target has it, it is still offered, and connecting degrades to a normal shell if the tool is missing. Once inspection shows it is missing, the next available multiplexer takes its place, or the shell if there is none.

### Multiplexer

A tool uplink knows how to detect, attach to and list sessions from. Today: tmux and zellij. More can be added later. A plain shell is a mode but not a multiplexer.

## Dashboard

Targets are listed in panes, one per group. Selecting a target shows a preview with:

- **Status:** up, down, starting, stopping or unknown.
- **Stats:** live metrics, built in for the target's OS or declared in the config.
- **Sessions:** the multiplexer sessions and windows of the selected mode.
- **Logs:** the latest output, with a full screen view available.

The selected target is refreshed continuously. Other targets are only checked in the background if they are watched, which discovered targets generally are.

## Lifecycle

Targets that define it can be started and stopped from the dashboard. Connecting to a stopped target that can be started starts it first, then connects if the start succeeded. A session ending always returns to the dashboard.
