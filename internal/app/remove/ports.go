package remove

import "context"

type Filesystem interface {
	RemoveAll(path string) error
	RemoveFileIfExists(path string) error
}

type IntegrationCacheRefresher interface {
	RefreshIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string)
}
