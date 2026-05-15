package remove

import "context"

type Filesystem interface {
	RemoveAll(path string) error
	RemoveFileIfExists(path string) error
}

type IntegrationCacheRefresher interface {
	RefreshIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string)
}

var (
	defaultFilesystem              Filesystem
	defaultIntegrationCacheRefresh IntegrationCacheRefresher
)

func SetFilesystem(filesystem Filesystem) {
	defaultFilesystem = filesystem
}

func SetIntegrationCacheRefresher(refresher IntegrationCacheRefresher) {
	defaultIntegrationCacheRefresh = refresher
}

func requireFilesystem() (Filesystem, error) {
	if defaultFilesystem == nil {
		return nil, errNotConfigured("remove filesystem")
	}
	return defaultFilesystem, nil
}

func requireIntegrationCacheRefresher() (IntegrationCacheRefresher, error) {
	if defaultIntegrationCacheRefresh == nil {
		return nil, errNotConfigured("integration cache refresher")
	}
	return defaultIntegrationCacheRefresh, nil
}
