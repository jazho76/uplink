package local

import (
	"context"
	"os"

	"github.com/jazho76/uplink/internal/inventory"
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

func (Provider) Discover(context.Context) ([]inventory.Discovered, error) {
	display, _ := hostname()
	return []inventory.Discovered{{
		Status: target.StatusUp,
		Target: target.Target{
			ID:    target.ID{Key: key},
			Name:  display,
			Group: group,
			Logs:  target.DetectedLogs(),
			Stats: target.StatsPolicy{Builtin: true},
		},
	}}, nil
}
