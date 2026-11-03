# Uplink

Uplink is a launcher for work environments. You pick one from a dashboard and land in it. When you leave, you return to the dashboard.

## Concepts

### Target

A place you can work: this machine, a VM, a container or a remote host. A target describes how to reach it, and optionally how to check, start and stop it.

### Provider

Automatically discovers one type of target and reports whether each one is alive. Today: local, lima, docker and tailscale. A provider that fails or is unavailable only costs its own targets.

### Config

Adds custom targets, adjusts or hides discovered ones, and chooses which providers run. Everything works without a config. An invalid target is skipped and reported, without affecting the others. A config file that can't be parsed, or that has unknown fields, is reported and contributes nothing.

### Inventory

The targets you can choose from. Providers discover targets, then the config adds its own and adjusts or hides discovered ones. The inventory stays current while the dashboard is open.

### Observation

What uplink currently knows about a target: its status, inspection and logs. Each is the latest known value, and may be missing or out of date. The selected target is observed continuously. Others only have their status observed, and only if they are watched, which discovered targets generally are.

### Status

Up, down, starting, stopping or unknown. Up means the target's owner reports it alive, not that you can log in to it. A provider is the owner of what it discovers. A custom target reports through its check or, without one, by uplink trying to reach it. Unknown means uplink couldn't find out.

### Inspection

How uplink learns about a target's environment: which tools it has, its stats, and its multiplexer sessions. It runs inside the target, as the user uplink reaches it with, and only while the target is up.

### Mode

A way into a target, such as tmux, zellij or a plain shell. Modes are discovered through inspection, and you can cycle through them in the dashboard. The config can instead declare a target's modes explicitly, as custom ways of accessing it. Declared modes replace the discovered ones, and only show sessions if they name their multiplexer.

### Default mode

The preferred mode, tmux unless configured otherwise. While uplink doesn't know whether a target has it, it is still offered, and connecting degrades to a normal shell if the tool is missing. Once inspection shows it is missing, the next available multiplexer takes its place, or the shell if there is none.

### Multiplexer

A tool uplink knows how to detect, attach to and list sessions from. Today: tmux and zellij. More can be added later. A plain shell is a mode but not a multiplexer.

### Lifecycle

Targets that define it can be started and stopped from the dashboard.

### Connection

Your stay in a target, through one of its modes. Connecting to a stopped target that can be started starts it first, then connects if the start succeeded. If its status is still unknown, uplink finds out before deciding, and if it still can't tell, it connects without starting. A target that can't be started has nothing to decide, and connects right away. A connection ending always returns to the dashboard.

## Dashboard

Targets are listed in panes, one per group. Selecting a target shows a preview of its observation:

- **Status:** with its reason when unknown.
- **Stats:** live metrics, built in for the target's OS or declared in the config.
- **Sessions:** the multiplexer sessions and windows of the selected mode.
- **Logs:** the latest output, with a full screen view available.
