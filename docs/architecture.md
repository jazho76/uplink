# Uplink architecture

This document records the decisions that shape Uplink. It is the reference for
new work, including a possible rewrite.

## 1. Purpose and non-goals

Uplink is a hub for jumping into machines. One shortcut opens the dashboard,
the user picks a machine and a launch mode, and lands in a session, usually
tmux. When the session ends, the dashboard comes back.

Uplink owns:

- listing machines and their state;
- starting and stopping existing machines through user-defined hooks;
- entering a machine in a chosen mode.

Uplink never owns:

- creating or deleting machines, templates, provisioning;
- clipboard, file transfer, or anything that happens inside a session;
- OS-specific knowledge. Everything OS-specific lives in config or presets.

## 2. The Target

There is one centric type: `Target`. Local, VM, container, and remote machines
differ only in data. There are no kinds, no type hierarchy, and no code path
that branches on what a target is. The local machine is an ordinary target.

A target is an immutable, fully resolved value made of optional capabilities.
A missing capability means a missing feature, never a special case.

The dashboard renders by capability: no `stats` means no live block, no `logs`
means no log tail, the footer offers only the keys the target supports.

Runtime state (up, down, starting, readings, timestamps) is not part of the
target. It lives in the dashboard, keyed by target ID.

## 3. From config to Target

```
config.yaml ──> Spec ──resolve()──> Target
providers  ──>  Spec ──┘
```

## 4. Execution contract

Every command runs on one of two sides.

- **Host-side** (`status`, `up`, `down`, `logs`): `sh -c <command>` on this
  machine.
- **Target-side** (modes, stats): the `via` words plus exactly one extra
  argument, the command. Without `via`, that is `sh -c <command>` here. Tools
  that take an argument list end `via` with `sh -c`.
- `exec` is `via` for background work, where no terminal is attached. It
  defaults to `via`.
- `via` and `exec` are split like a shell command but never run through a
  local shell.

One executor owns all target behavior: `Status`, `Up`, `Down`, `Stats`,
`Logs`. Callers sequence operations; they never redefine what an operation
means.

Background commands never prompt: no stdin, their own session, a process group
killed on timeout, `SSH_ASKPASS_REQUIRE=never`. A transport that needs a
password shows the target as down.

## 7. Discovery

Discovery fills the registry from tools that already know about machines, so
Uplink is useful without a config file. It is an input to the existing
pipeline, not a parallel one.

### 7.1 Providers produce Specs

A provider is a contained subsystem with one job: list instances as `Spec`
values. Its output goes through the same `resolve()` as config targets.

```go
type Provider interface {
    Name() string
    Discover(ctx context.Context, entry Entry) ([]target.Spec, error)
}
```

- Providers shell out to the tool's CLI and parse its machine-readable output
  (`limactl list --json`, `docker ps --format json`). No SDKs, no reading the
  tool's private files.
- Providers report no runtime state. Status remains "the status command": a
  provider fills `status` with the right check, and the executor runs it like
  any other.
- A provider never creates or deletes instances.

### 7.2 Facts and preferences

A provider fills only what it knows as facts about an instance, plus
reasonable defaults. Config owns preferences.

## 8. Portability

- The binary contains only POSIX process handling and no OS readers; it builds
  and vets for Linux and macOS.
- Outstanding for macOS: release artifacts, installer, and self-update are
  still Linux amd64 only. `install-shortcut` is GNOME only and fails cleanly
  elsewhere.

