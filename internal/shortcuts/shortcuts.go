package shortcuts

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jazho76/vmm/internal/run"
)

const (
	schema = "org.gnome.settings-daemon.plugins.media-keys"
	item   = "org.gnome.settings-daemon.plugins.media-keys.custom-keybinding"
	base   = "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings"
)

type shortcut struct {
	id      string
	binding string
	name    string
	command string
}

func defaults(exe string) []shortcut {
	return []shortcut{
		{"vmm-launcher", "<Control><Alt>t", "VMs launcher", "alacritty -e " + exe + " dashboard"},
		{"vmm-clipboard", "<Control><Alt>p", "VMs push clipboard", exe + " push-clipboard"},
	}
}

func Install() error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return fmt.Errorf("gsettings not found; need a GNOME session")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	for _, s := range defaults(exe) {
		if err := set(s); err != nil {
			return err
		}
	}
	return nil
}

func Uninstall() error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return fmt.Errorf("gsettings not found; need a GNOME session")
	}
	for _, s := range defaults("") {
		path := fmt.Sprintf("%s/%s/", base, s.id)
		for _, key := range []string{"name", "command", "binding"} {
			_ = run.Silent("gsettings", "reset", fmt.Sprintf("%s:%s", item, path), key)
		}
		if err := listRemove(path); err != nil {
			return err
		}
	}
	return nil
}

func set(s shortcut) error {
	path := fmt.Sprintf("%s/%s/", base, s.id)
	target := fmt.Sprintf("%s:%s", item, path)
	if err := run.Silent("gsettings", "set", target, "name", s.name); err != nil {
		return err
	}
	if err := run.Silent("gsettings", "set", target, "command", s.command); err != nil {
		return err
	}
	if err := run.Silent("gsettings", "set", target, "binding", s.binding); err != nil {
		return err
	}
	return listAdd(path)
}

func listAdd(path string) error {
	list, err := run.Output("gsettings", "get", schema, "custom-keybindings")
	if err != nil {
		return err
	}
	if strings.Contains(list, "'"+path+"'") {
		return nil
	}
	var next string
	switch list {
	case "@as []", "[]":
		next = fmt.Sprintf("['%s']", path)
	default:
		next = strings.TrimSuffix(list, "]") + fmt.Sprintf(", '%s']", path)
	}
	return run.Silent("gsettings", "set", schema, "custom-keybindings", next)
}

func listRemove(path string) error {
	list, err := run.Output("gsettings", "get", schema, "custom-keybindings")
	if err != nil {
		return err
	}
	next := list
	next = strings.ReplaceAll(next, "'"+path+"', ", "")
	next = strings.ReplaceAll(next, ", '"+path+"'", "")
	next = strings.ReplaceAll(next, "['"+path+"']", "@as []")
	if next == list {
		return nil
	}
	return run.Silent("gsettings", "set", schema, "custom-keybindings", next)
}
