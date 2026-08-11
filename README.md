# uplink

A tool for managing and launching isolated development environments on top of
[Lima](https://lima-vm.io/). Each environment is a full Linux VM defined by a
template.

![uplink dashboard](docs/dashboard.png)

## Requirements

- Linux (the prebuilt binary is `linux/amd64`)
- [Lima](https://lima-vm.io/) (`limactl` on your `PATH`)
- `git`
- Optional, for global keybindings: GNOME and [Alacritty](https://alacritty.org/)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/jazho76/uplink/main/scripts/install.sh | sh
```

This drops `uplink` in `~/.local/bin`. If that is not on your `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Keep it current with `uplink upgrade`.

## Quick start

```sh
# 1. Add a template (a git repo that defines a VM). Its name is the repo basename.
uplink template add git@github.com:you/your_vm.git

# 2. Create one or more instances from it. The name defaults to the template's.
uplink create your_vm              # instance "your_vm"
uplink create your_vm your_vm-2    # a second instance from the same template

# 3. Open a shell in an instance (starts it if stopped).
uplink connect your_vm

# 4. Browse and control your instances interactively.
uplink dashboard
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
uplink template add <git-url>          # clone a template into the templates dir (name = repo basename)
uplink template list                   # show installed templates, origins, dirty state
uplink template update [name]          # pull the latest version, fast-forward only (all if no name)
uplink template remove <name>          # delete it (use --force to skip the prompt)
```

## Managing instances

```sh
uplink create <template> [instance]   # create and provision an instance from a template
uplink connect <instance>             # open a shell, starting the instance if stopped
uplink push-clipboard [instance]      # copy the host clipboard into an instance
uplink refresh-externals <instance>   # re-fetch and re-apply the instance's externals
```

`uplink create` is how you spin up an instance; the name defaults to the template's,
and you pass a second argument to run more than one from the same template.

## Dashboard

`uplink dashboard` is the interactive hub: a two-pane TUI over everything Lima
knows about, so instances created outside uplink show up too.

| Key             | Action                  |
| --------------- | ----------------------- |
| `↑`/`k` `↓`/`j` | move                    |
| `1`-`9`         | connect to that vm      |
| `Enter`         | connect the selected vm |
| `Ctrl-L`        | view logs               |
| `Ctrl-S`        | stop                    |
| `Ctrl-R`        | restart                 |
| `Ctrl-A`        | toggle autostart        |
| `Ctrl-X`        | delete                  |
| `q` / `Esc`     | quit                    |

## Launcher

On a GNOME session, register global keybindings so uplink behaves like a native
app launcher: one hotkey pops the dashboard in an Alacritty window, no terminal
needed.

```sh
uplink install-shortcuts     # Ctrl+Alt+T opens the dashboard, Ctrl+Alt+P pushes the clipboard
uplink uninstall-shortcuts
```

## Authoring a template

A template is a git repo with this shape:

```
template.yaml        # Lima config; declares `param: TemplateDir` and mounts
                     # host paths as {{.Param.TemplateDir}}/dotfiles, etc.
dotfiles/            # mounted read-only into the guest
provision/           # provisioning scripts referenced by template.yaml
fetch-externals.sh   # optional: host-side step to pull externals before create
```

## From source

```sh
make install     # build and install to $GOBIN (or $(go env GOPATH)/bin)
make uninstall   # remove it
```
