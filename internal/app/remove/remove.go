package remove

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type Service struct {
	Store                     AppStore
	Filesystem                Filesystem
	IntegrationCacheRefresher IntegrationCacheRefresher
	Paths                     Paths
}

func NewService(service Service) Service {
	return service
}

func Remove(ctx context.Context, id string, unlink bool) (*models.App, error) {
	return Service{}.Remove(ctx, id, unlink)
}

func (service Service) Remove(ctx context.Context, id string, unlink bool) (*models.App, error) {
	store, err := service.requireStore()
	if err != nil {
		return nil, err
	}

	return service.remove(ctx, store, id, unlink)
}

func (service Service) remove(ctx context.Context, store AppStore, id string, unlink bool) (*models.App, error) {
	paths, err := service.requirePaths()
	if err != nil {
		return nil, err
	}
	filesystem, err := service.requireFilesystem()
	if err != nil {
		return nil, err
	}
	cacheRefresher, err := service.requireIntegrationCacheRefresher()
	if err != nil {
		return nil, err
	}

	appData, err := store.GetApp(id)
	if err != nil {
		return nil, fmt.Errorf("no app with id %s exists", id)
	}

	if err := filesystem.RemoveFileIfExists(appData.DesktopEntryLink); err != nil {
		return nil, fmt.Errorf("failed to remove desktop link: %w", err)
	}

	if unlink {
		appData.DesktopEntryLink = ""
		if err := store.AddApp(appData, true); err != nil {
			return appData, err
		}
	} else {
		if err := store.RemoveApp(appData.ID); err != nil {
			return appData, err
		}

		appDir := filepath.Join(paths.AimDir, appData.ID)
		if appData.IconPath != "" {
			iconPath := filepath.Clean(appData.IconPath)
			if iconPath != appDir && !strings.HasPrefix(iconPath, appDir+string(filepath.Separator)) {
				_ = filesystem.RemoveFileIfExists(iconPath)
			}
		}

		if err := filesystem.RemoveAll(appDir); err != nil {
			return appData, fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	cacheRefresher.RefreshIntegrationCaches(ctx, paths.DesktopDir, paths.IconThemeDir)

	return appData, nil
}

func (service Service) requireStore() (AppStore, error) {
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	return service.Store, nil
}

func (service Service) requirePaths() (Paths, error) {
	if service.Paths.AimDir == "" || service.Paths.DesktopDir == "" || service.Paths.IconThemeDir == "" {
		return Paths{}, fmt.Errorf("remove paths are not configured")
	}
	return service.Paths, nil
}

func (service Service) requireFilesystem() (Filesystem, error) {
	if service.Filesystem == nil {
		return nil, errNotConfigured("remove filesystem")
	}
	return service.Filesystem, nil
}

func (service Service) requireIntegrationCacheRefresher() (IntegrationCacheRefresher, error) {
	if service.IntegrationCacheRefresher == nil {
		return nil, errNotConfigured("integration cache refresher")
	}
	return service.IntegrationCacheRefresher, nil
}
