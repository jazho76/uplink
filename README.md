# vmm

Isolated development environments on top of [Lima](https://lima-vm.io/), built
to be effortless to start and maintain. Each environment is a full Linux VM
defined by a **template** (a git repo with its config, dotfiles, and
provisioning), so a clean, sandboxed workspace is one command away and just as
easy to recreate. Version your environments in git, update them with a pull,
and rebuild from scratch whenever you want a fresh slate. The dashboard ties it
together as the launcher of your workflow: browse your VMs, drop into a shell,
start and stop them, and tail logs from one interactive view.

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
# 1. Add a template (a git repo that defines a VM). You pick its local name.
vmm template add git@github.com:you/your_vm.git dev

# 2. Open a shell in it. It is created, provisioned, and started on first use.
vmm connect dev

# 3. Browse and control your VMs interactively.
vmm dashboard
```

## Concepts

- **Template** - a git repo containing a `template.yaml` (the Lima config),
  plus `dotfiles/` and `provision/` scripts. Templates live in
  `~/.local/share/vmm/templates/<name>`. The local `<name>` is both the
  directory and the VM name, and you choose it when you add the template. Add
  the same repo under different names to run independent copies.
- **Instance** - the Lima VM created from a template. It shares the template's
  name.

## Managing templates

```sh
vmm template add <git-url> <name>   # clone a template into the templates dir
vmm template list                   # show installed templates, origins, dirty state
vmm template update <name>          # pull the latest version (fast-forward only)
vmm template remove <name>          # delete it (use --force to skip the prompt)
```

## Managing VMs

```sh
vmm create <name>              # create and provision a VM from its template
vmm connect <name>             # open a shell, creating/starting the VM as needed
vmm logs <name>                # tail the VM's serial console log
vmm push-clipboard [name]      # copy the host clipboard into a VM
vmm refresh-externals <name>   # re-fetch and re-apply a template's externals
```

`vmm connect` is the everyday command: it creates the VM if it does not exist,
starts it if stopped, and drops you into a shell.

## Dashboard

`vmm dashboard` opens an interactive list of your templates and running VMs.

| Key         | Action                            |
| ----------- | --------------------------------- |
| `↑` / `k`   | move up                           |
| `↓` / `j`   | move down                         |
| `Enter`     | connect (or create if new)        |
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
template defines `fetch-externals.sh`, it runs on the host before the VM is
created, and `vmm refresh-externals` re-applies it into a running VM.

## From source

```sh
make install     # build and install to $GOBIN (or $(go env GOPATH)/bin)
make uninstall   # remove it
```
