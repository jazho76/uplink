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

type Failure struct {
	Reason string
	Err    error
}

func (f Failure) Error() string { return f.Err.Error() }

func (f Failure) Unwrap() error { return f.Err }
