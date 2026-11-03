package inventory

import (
	"github.com/jazho76/uplink/internal/target"
)

type Plan struct {
	Providers   []string
	Explicit    bool
	Invalid     bool
	Declared    []target.Target
	Overlays    []Overlay
	DefaultMode string
	Problems    error
}

func declaredFound(declared []target.Target) Found {
	found := Found{Source: target.ConfigSource, Discovered: make([]Discovered, len(declared))}
	for i, t := range declared {
		found.Discovered[i] = Discovered{Target: t, Status: target.StatusUnknown}
	}
	return found
}
