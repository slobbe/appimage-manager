package commandtest

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/app"
)

// Service provides no-op app.Service methods for command tests.
type Service struct{}

func (Service) Add(ctx context.Context, req app.AddRequest) (app.AddResult, error) {
	return app.AddResult{}, nil
}

func (Service) Remove(ctx context.Context, req app.RemoveRequest) error {
	return nil
}

func (Service) Update(ctx context.Context, req app.UpdateRequest) (app.UpdateResult, error) {
	return app.UpdateResult{}, nil
}

func (Service) SetUpdateSource(ctx context.Context, req app.SetUpdateSourceRequest) (app.SetUpdateSourceResult, error) {
	return app.SetUpdateSourceResult{}, nil
}

func (Service) UnsetUpdateSource(ctx context.Context, req app.UnsetUpdateSourceRequest) error {
	return nil
}

func (Service) SetID(ctx context.Context, req app.SetIDRequest) (app.SetIDResult, error) {
	return app.SetIDResult{}, nil
}

func (Service) List(ctx context.Context, req app.ListRequest) (app.ListResult, error) {
	return app.ListResult{}, nil
}

func (Service) Info(ctx context.Context, req app.InfoRequest) (app.InfoResult, error) {
	return app.InfoResult{}, nil
}

func (Service) SelfUpdate(ctx context.Context, req app.SelfUpdateRequest) (app.SelfUpdateResult, error) {
	return app.SelfUpdateResult{}, nil
}

func (Service) Paths(ctx context.Context, req app.PathsRequest) (app.PathsResult, error) {
	return app.PathsResult{}, nil
}
