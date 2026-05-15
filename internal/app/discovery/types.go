package discovery

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type DiscoveryBackend interface {
	Name() string
	Resolve(ctx context.Context, ref domain.PackageRef, assetOverride string) (*domain.PackageMetadata, error)
}
