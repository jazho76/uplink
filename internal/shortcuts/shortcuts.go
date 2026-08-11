package shortcuts

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jazho76/uplink/internal/run"
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

var legacyIDs = []string{"vmm-launcher", "vmm-clipboard"}

func defaults(exe string) []shortcut {
	return []shortcut{
		{"uplink-launcher", "<Control><Alt>t", "uplink launcher", "alacritty -e " + exe + " dashboard"},
		{"uplink-clipboard", "<Control><Alt>p", "uplink push clipboard", exe + " vm push-clipboard"},
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

	if err := purgeLegacy(); err != nil {
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
	if err := purgeLegacy(); err != nil {
		return err
	}
	for _, s := range defaults("") {
		if err := remove(s.id); err != nil {
			return err
		}
	}
	return nil
}

func purgeLegacy() error {
	for _, id := range legacyIDs {
		if err := remove(id); err != nil {
			return err
		}
	}
	return nil
}

func remove(id string) error {
	path := fmt.Sprintf("%s/%s/", base, id)
	for _, key := range []string{"name", "command", "binding"} {
		_ = run.Silent("gsettings", "reset", fmt.Sprintf("%s:%s", item, path), key)
	}
	return listRemove(path)
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
