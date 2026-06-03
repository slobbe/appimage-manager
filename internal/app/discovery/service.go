package discovery

import (
	"context"
	"fmt"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type Service struct {
	Backends []DiscoveryBackend
}

func NewService(backends ...DiscoveryBackend) Service {
	return Service{Backends: backends}
}

func (service Service) Resolve(ctx context.Context, ref domain.PackageRef, assetOverride string) (*domain.PackageMetadata, error) {
	for _, backend := range service.Backends {
		if backend == nil {
			continue
		}
		metadata, err := backend.Resolve(ctx, ref, assetOverride)
		if err != nil {
			return nil, err
		}
		if metadata != nil {
			return metadata, nil
		}
	}
	return nil, fmt.Errorf("failed to resolve package metadata for %s", domain.FormatPackageRef(ref))
}
