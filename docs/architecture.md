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

| Capability | Fields | Absent means |
| --- | --- | --- |
| identity | `name`, `group` | required / `ungrouped` |
| transport | `via`, `exec` | runs on this machine through `sh -c` |
| lifecycle | `status`, `up`, `down` | assumed reachable; no start or stop |
| observation | `stats`, `logs` | no live block, no logs |
| entry | `modes` | required, at least one |
| presentation | `watch` | not polled in the background |

The dashboard renders by capability: no `stats` means no live block, no `logs`
means no log tail, the footer offers only the keys the target supports.

Runtime state (up, down, starting, readings, timestamps) is not part of the
target. It lives in the dashboard, keyed by target name.

## 3. From config to Target

```
config.yaml ──> Spec ──resolve()──> Target
providers  ──>  Spec ──┘
```

- `Spec` is the raw, string-typed description of a target, as written in config
  or produced by a discovery provider.
- Profiles are the class, targets are the instance. A target with
  `profile: x` takes every field it leaves unset from profile `x`. Lists
  (`modes`, `stats`) are taken whole.
- `{name}` in command fields becomes the target's name.
- `resolve()` is the only place a Spec becomes a Target: profile merge,
  interpolation, splitting `via`/`exec` into words, expanding a leading `~/`,
  validating stats, building each mode's argv.
- Every source of targets goes through the same `resolve()`. There is one
  registry function that returns all targets; nothing else constructs them.
- Names are unique and limited to letters, digits, `.`, `_`, `-`.


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

- **Status** is the `status` hook (exit 0 is up), or `true` run through
  `exec`. Nothing else implies status.
- **Stats** run only after status reports up, and only for the selected target.
- **Start** has a single implementation, used by Ctrl+S and by Enter on a down
  target.

Background commands never prompt: no stdin, their own session, a process group
killed on timeout, `SSH_ASKPASS_REQUIRE=never`. A transport that needs a
password shows the target as down.

| Operation | Timeout |
| --- | --- |
| logs | 2s |
| status hook | 5s |
| each metric (where the target has `timeout`) | 5s |
| probes (`exec true`, stats round trip) | 30s |
| `up`, `down` | 10m |

## 5. Stats

Stats exist only in config. Uplink has no built-in metrics, because what to run
depends on the target's OS.

- Two shapes: `gauge` (`value` out of `max`, drawn as a bar) and `text`
  (`value` as is).
- Two units: `bytes` and `seconds`. With a unit, the value must be a number.
- `value` and `max` are target-side commands; the first line of output counts.
  A plain-number `max` is a constant.
- All metrics of a target travel in one script through `exec`, one round trip
  per refresh. Each metric runs isolated (own `sh -c`, no stdin, its own
  timeout, tagged output), so a failing or hanging metric shows `?` without
  affecting the others.
- OS presets (Linux, macOS) live in the README and the config template, not in
  the binary.

No Go code reads OS state (`/proc`, `sysctl`). This keeps the binary portable.

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

### 7.4 Activation

- No `discover` section: the default providers run when their tool is on
  `PATH`. Defaults are **lima** and **docker**.
- A `discover` section: exactly the listed entries run.
- `discover: []`: discovery is off.
- `ssh_config`-style sources are never on by default; they are noisy
  (`github.com`, jump hosts). Use the `command` provider.

### 7.6 Lifecycle of discovery

- Discovery runs when the dashboard starts and when the config reloads. There
  is no periodic re-discovery.
- It runs asynchronously: explicit targets render at once, discovered ones
  appear when their provider reports.
- Each provider has a timeout. A failing provider (tool missing, daemon down)
  contributes no targets and a status line note; it is never fatal.

### 7.7 Default provider behavior

- **lima**: all instances, running or stopped. `via` uses the instance's
  `ssh.config`; `status` checks `Running`; `up`/`down` are `limactl
  start/stop`; `logs` tails `serial.log`.
- **docker**: all containers, running or stopped, narrowed with `filter`.
  `via` is `docker exec -it {name} sh -c`, `exec` is `docker exec -i {name} sh
  -c`; `up`/`down` are `docker start/stop`; `logs` is `docker logs`.

## 8. Portability

- The binary contains only POSIX process handling and no OS readers; it builds
  and vets for Linux and macOS.
- Outstanding for macOS: release artifacts, installer, and self-update are
  still Linux amd64 only. `install-shortcut` is GNOME only and fails cleanly
  elsewhere.

