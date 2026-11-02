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

func (Provider) Specs() ([]target.Spec, error) {
	display, _ := hostname()
	return []target.Spec{{
		Name:     key,
		Display:  display,
		Local:    true,
		Defaults: &target.Spec{Group: group},
	}}, nil
}
