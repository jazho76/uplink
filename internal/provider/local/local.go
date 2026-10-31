package local

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/jazho76/uplink/internal/target"
)

const (
	name    = "local"
	key     = "localhost"
	group   = "machines"
	journal = "journalctl --system -n 500 --no-pager -o short"
)

var (
	goos     = runtime.GOOS
	hostname = os.Hostname
	lookPath = exec.LookPath
)

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Default() bool { return true }

func (Provider) Specs() ([]target.Spec, error) {
	display, _ := hostname()
	return []target.Spec{{
		Name:    key,
		Display: display,
		Defaults: &target.Spec{
			Group: group,
			Stats: preset(goos),
			Logs:  logs(),
			Modes: modes(),
		},
	}}, nil
}

func logs() string {
	if !installed("journalctl") {
		return ""
	}
	return journal
}

func modes() []target.ModeSpec {
	var modes []target.ModeSpec
	if installed("tmux") {
		modes = append(modes, target.ModeSpec{Name: "tmux", Run: "tmux new-session -A -s {name}"})
	}
	return append(modes, target.ModeSpec{Name: "shell"})
}

func installed(tool string) bool {
	_, err := lookPath(tool)
	return err == nil
}
