package integrate

import (
	"context"
	"fmt"

	appimage "github.com/slobbe/appimage-manager/internal/app/appimage"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	models "github.com/slobbe/appimage-manager/internal/domain"
)

type AppImageService interface {
	ExtractAppImage(ctx context.Context, src string) (*appimage.ExtractionData, error)
	GetAppInfo(ctx context.Context, desktopSrc string) (*appimage.AppInfo, error)
	UpdateDesktopEntry(ctx context.Context, src string, execSrc string, iconSrc string) error
}

type Service struct {
	Store                            AppStore
	Filesystem                       Filesystem
	DesktopLinkResolver              DesktopLinkResolver
	DesktopEntryValidator            DesktopEntryValidator
	DesktopIntegrationCacheRefresher DesktopIntegrationCacheRefresher
	AppImage                         AppImageService
	Paths                            Paths
	IntegrateLocalFile               func(context.Context, string, UpdateOverwritePrompt) (*models.App, error)
	IntegrateLocalFileNoCacheRefresh func(context.Context, string, UpdateOverwritePrompt) (*models.App, error)
	ReintegrateExistingApp           func(context.Context, string) (*models.App, error)
	EmbeddedUpdateInfo               func(string) (*appupdate.UpdateInfo, error)
}

func NewService(service Service) Service {
	return service
}

func (service Service) IntegrateLocal(ctx context.Context, src string, prompt UpdateOverwritePrompt) (*models.App, error) {
	if service.IntegrateLocalFile != nil {
		return service.IntegrateLocalFile(ctx, src, prompt)
	}
	return service.integrateFromLocalFile(ctx, src, prompt, true, true)
}

func (service Service) IntegrateLocalWithoutCacheRefresh(ctx context.Context, src string, prompt UpdateOverwritePrompt) (*models.App, error) {
	if service.IntegrateLocalFileNoCacheRefresh != nil {
		return service.IntegrateLocalFileNoCacheRefresh(ctx, src, prompt)
	}
	return service.integrateFromLocalFile(ctx, src, prompt, false, true)
}

func (service Service) IntegrateLocalWithoutCacheRefreshOrPersist(ctx context.Context, src string, prompt UpdateOverwritePrompt) (*models.App, error) {
	return service.integrateFromLocalFile(ctx, src, prompt, false, false)
}

func (service Service) Reintegrate(ctx context.Context, id string) (*models.App, error) {
	if service.ReintegrateExistingApp != nil {
		return service.ReintegrateExistingApp(ctx, id)
	}
	return service.integrateExisting(ctx, id)
}

func (service Service) requireStore() (AppStore, error) {
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	return service.Store, nil
}

func (service Service) requirePaths() (Paths, error) {
	if service.Paths.AimDir == "" || service.Paths.DesktopDir == "" || service.Paths.TempDir == "" || service.Paths.IconThemeDir == "" {
		return Paths{}, fmt.Errorf("app paths are not configured")
	}
	return service.Paths, nil
}

func (service Service) requireFilesystem() (Filesystem, error) {
	if service.Filesystem == nil {
		return nil, errNotConfigured("integrate filesystem")
	}
	return service.Filesystem, nil
}

func (service Service) requireDesktopLinkResolver() (DesktopLinkResolver, error) {
	if service.DesktopLinkResolver == nil {
		return nil, errNotConfigured("desktop link resolver")
	}
	return service.DesktopLinkResolver, nil
}

func (service Service) requireDesktopEntryValidator() (DesktopEntryValidator, error) {
	if service.DesktopEntryValidator == nil {
		return nil, errNotConfigured("desktop entry validator")
	}
	return service.DesktopEntryValidator, nil
}

func (service Service) requireDesktopIntegrationCacheRefresher() (DesktopIntegrationCacheRefresher, error) {
	if service.DesktopIntegrationCacheRefresher == nil {
		return nil, errNotConfigured("desktop integration cache refresher")
	}
	return service.DesktopIntegrationCacheRefresher, nil
}
