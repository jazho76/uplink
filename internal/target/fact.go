package target

import "time"

type Fact[T any] struct {
	Value T
	Known bool
	At    time.Time
	Err   error
}

func (f *Fact[T]) Observe(value T, at time.Time) {
	f.Value, f.Known, f.At, f.Err = value, true, at, nil
}

func (f *Fact[T]) Fail(err error, at time.Time) {
	f.At, f.Err = at, err
}

func (f Fact[T]) Current() bool { return f.Known && f.Err == nil }
