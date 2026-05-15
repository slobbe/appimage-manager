package integrate

import (
	"context"
	"fmt"
)

func refreshDesktopIntegrationCaches(ctx context.Context) {
	paths, err := requirePaths()
	if err != nil {
		return
	}
	refresher, err := requireDesktopIntegrationCacheRefresher()
	if err != nil {
		return
	}
	refresher.RefreshDesktopIntegrationCaches(ctx, paths.DesktopDir, paths.IconThemeDir)
}

func RefreshDesktopIntegrationCaches(ctx context.Context) {
	refreshDesktopIntegrationCaches(ctx)
}

func requireCachePathsForRefresh() (string, string, error) {
	paths, err := requirePaths()
	if err != nil {
		return "", "", fmt.Errorf("failed to refresh integration caches: %w", err)
	}
	return paths.DesktopDir, paths.IconThemeDir, nil
}
