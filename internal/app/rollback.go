package app

import "context"

type rollbackFunc func(ctx context.Context) error

type rollbackStack struct {
	items []rollbackFunc
}

func (s *rollbackStack) add(fn rollbackFunc) {
	s.items = append(s.items, fn)
}

func (s *rollbackStack) run(ctx context.Context) {
	rollbackCtx := context.WithoutCancel(ctx)
	for i := len(s.items) - 1; i >= 0; i-- {
		_ = s.items[i](rollbackCtx)
	}
}
