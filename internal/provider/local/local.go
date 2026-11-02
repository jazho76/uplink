package local

import (
	"os"
	"os/exec"

	"github.com/jazho76/uplink/internal/target"
)

const (
	name    = "local"
	key     = "localhost"
	group   = "local"
	journal = "journalctl --system -n 500 --no-pager -o short"
)

var (
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
		Local:   true,
		Defaults: &target.Spec{
			Group: group,
			Logs:  logs(),
		},
	}}, nil
}

func logs() string {
	if !installed("journalctl") {
		return ""
	}
	return journal
}

func installed(tool string) bool {
	_, err := lookPath(tool)
	return err == nil
}
