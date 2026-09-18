package app

import (
	"context"
	"errors"
)

type rollbackFunc func(ctx context.Context) error

type rollbackStack struct {
	items []rollbackFunc
}

func (s *rollbackStack) add(fn rollbackFunc) {
	s.items = append(s.items, fn)
}

func (s *rollbackStack) run(ctx context.Context) error {
	rollbackCtx := context.WithoutCancel(ctx)
	var failures []error
	for i := len(s.items) - 1; i >= 0; i-- {
		if err := s.items[i](rollbackCtx); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
