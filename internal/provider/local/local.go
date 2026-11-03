package local

import (
	"os"

	"github.com/jazho76/uplink/internal/target"
)

const (
	name  = "local"
	key   = "localhost"
	group = "local"
)

var hostname = os.Hostname

type Provider struct{}

func (Provider) Name() string { return name }

func (Provider) Default() bool { return true }

func (Provider) Targets() ([]target.Target, error) {
	display, _ := hostname()
	return []target.Target{{
		ID:    target.ID{Key: key},
		Name:  display,
		Group: group,
		Logs:  target.LogSource{Journal: true},
		Stats: target.StatsPolicy{Builtin: true},
	}}, nil
}
