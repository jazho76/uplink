package inventory

import (
	"context"
	"errors"

	"github.com/jazho76/uplink/internal/target"
)

var ErrUnavailable = errors.New("unavailable")

type Provider interface {
	Name() string
	Discover(ctx context.Context) ([]Discovered, error)
}

type Discovered struct {
	Target target.Target
	Status target.Status
	Reason string
}
