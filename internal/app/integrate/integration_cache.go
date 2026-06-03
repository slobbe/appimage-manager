package integrate

import (
	"context"
	"fmt"
)

func (service Service) refreshDesktopIntegrationCaches(ctx context.Context) {
	paths, err := service.requirePaths()
	if err != nil {
		return
	}
	refresher, err := service.requireDesktopIntegrationCacheRefresher()
	if err != nil {
		return
	}
	refresher.RefreshDesktopIntegrationCaches(ctx, paths.DesktopDir, paths.IconThemeDir)
}

func RefreshDesktopIntegrationCaches(ctx context.Context) {
	Service{}.refreshDesktopIntegrationCaches(ctx)
}

func (service Service) requireCachePathsForRefresh() (string, string, error) {
	paths, err := service.requirePaths()
	if err != nil {
		return "", "", fmt.Errorf("failed to refresh integration caches: %w", err)
	}
	return paths.DesktopDir, paths.IconThemeDir, nil
}
