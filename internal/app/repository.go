package app

import (
	"context"
	"errors"

	"aim/internal/domain"
)

var ErrAppNotFound = errors.New("app not found")

// AppRepository persists integrated applications.
//
// Implementations belong in the infrastructure layer. The app layer owns this
// port because application use cases decide when apps are saved, loaded, listed,
// or removed.
type AppRepository interface {
	Save(ctx context.Context, app domain.App) error
	Find(ctx context.Context, id string) (domain.App, error)
	List(ctx context.Context) ([]domain.App, error)
	Delete(ctx context.Context, id string) error
}
