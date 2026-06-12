# vmm

A tool for managing and launching isolated development environments on top of
[Lima](https://lima-vm.io/). Each environment is a full Linux VM defined by a
template.

## Requirements

- Linux (the prebuilt binary is `linux/amd64`)
- [Lima](https://lima-vm.io/) (`limactl` on your `PATH`)
- `git`
- Optional, for global keybindings: GNOME and [Alacritty](https://alacritty.org/)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/jazho76/vmm/main/scripts/install.sh | sh
```

This drops `vmm` in `~/.local/bin`. If that is not on your `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Keep it current with `vmm upgrade`.

## Quick start

```sh
# 1. Add a template (a git repo that defines a VM). Its name is the repo basename.
vmm template add git@github.com:you/your_vm.git

# 2. Create one or more instances from it. The name defaults to the template's.
vmm create your_vm              # instance "your_vm"
vmm create your_vm your_vm-2    # a second instance from the same template

# 3. Open a shell in an instance (starts it if stopped).
vmm connect your_vm

# 4. Browse and control your instances interactively.
vmm dashboard
```

## Concepts

- **Template** - a git repo containing a `template.yaml` (the Lima config),
  plus `dotfiles/` and `provision/` scripts. Templates live in
  `~/.local/share/vmm/templates/<name>`, where `<name>` is the repo basename.
  A template is a recipe; you launch instances from it.
- **Instance** - a Lima VM created from a template. One template can produce
  many instances under different names; each instance remembers the template it
  came from (Lima persists its `TemplateDir`).

## Managing templates

```sh
vmm template add <git-url>          # clone a template into the templates dir (name = repo basename)
vmm template list                   # show installed templates, origins, dirty state
vmm template update [name]          # pull the latest version, fast-forward only (all if no name)
vmm template remove <name>          # delete it (use --force to skip the prompt)
```

`remove` refuses while any instance created from the template still exists, so
you never orphan a running environment. Delete those instances first (dashboard
`Ctrl-X` or `limactl delete <name>`).

## Managing instances

```sh
vmm create <template> [instance]   # create and provision an instance from a template
vmm connect <instance>             # open a shell, starting the instance if stopped
vmm push-clipboard [instance]      # copy the host clipboard into an instance
vmm refresh-externals <instance>   # re-fetch and re-apply the instance's externals
```

`vmm create` is how you spin up an instance; the name defaults to the template's,
and you pass a second argument to run more than one from the same template.
`vmm connect` is the everyday command: it starts the instance if stopped and
drops you into a shell. To read an instance's serial log, use the dashboard
(`Ctrl-L`) or tail `~/.lima/<name>/serial.log` directly.

## Dashboard

`vmm dashboard` opens an interactive list of your instances. It lists exactly
what Lima knows about, so instances created outside vmm show up too. Create new
instances from the CLI with `vmm create`.

| Key         | Action                            |
| ----------- | --------------------------------- |
| `↑` / `k`   | move up                           |
| `↓` / `j`   | move down                         |
| `Enter`     | connect (starts it if stopped)    |
| `Ctrl-L`    | view logs                         |
| `Ctrl-S`    | stop                              |
| `Ctrl-R`    | restart                           |
| `Ctrl-A`    | toggle autostart                  |
| `Ctrl-X`    | delete (type the name to confirm) |
| `q` / `Esc` | quit                              |

## GNOME shortcuts

On a GNOME session you can register global keybindings:

```sh
vmm install-shortcuts     # Ctrl+Alt+T opens the dashboard, Ctrl+Alt+P pushes the clipboard
vmm uninstall-shortcuts
```

## Authoring a template

A template is a git repo with this shape:

```
template.yaml        # Lima config; declares `param: TemplateDir` and mounts
                     # host paths as {{.Param.TemplateDir}}/dotfiles, etc.
dotfiles/            # mounted read-only into the guest
provision/           # provisioning scripts referenced by template.yaml
fetch-externals.sh   # optional: host-side step to pull secrets before create
```

`vmm` injects the template's directory as `TemplateDir` at create time, so
mounts and provisioning can reference files that ship with the template. If a
template defines `fetch-externals.sh`, it runs on the host before the instance
is created, and `vmm refresh-externals` re-applies it into a running instance.

## From source

```sh
make install     # build and install to $GOBIN (or $(go env GOPATH)/bin)
make uninstall   # remove it
```
