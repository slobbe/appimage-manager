package integrate

import (
	"context"
	"fmt"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type Service struct {
	Store                            AppStore
	Filesystem                       Filesystem
	DesktopLinkResolver              DesktopLinkResolver
	DesktopEntryValidator            DesktopEntryValidator
	DesktopIntegrationCacheRefresher DesktopIntegrationCacheRefresher
	Paths                            Paths
	IntegrateLocalFile               func(context.Context, string, UpdateOverwritePrompt) (*models.App, error)
	IntegrateLocalFileNoCacheRefresh func(context.Context, string, UpdateOverwritePrompt) (*models.App, error)
	ReintegrateExistingApp           func(context.Context, string) (*models.App, error)
}

func NewService(service Service) Service {
	return service
}

func (service Service) IntegrateLocal(ctx context.Context, src string, prompt UpdateOverwritePrompt) (*models.App, error) {
	if service.IntegrateLocalFile != nil {
		return service.IntegrateLocalFile(ctx, src, prompt)
	}
	return IntegrateFromLocalFile(ctx, src, prompt)
}

func (service Service) IntegrateLocalWithoutCacheRefresh(ctx context.Context, src string, prompt UpdateOverwritePrompt) (*models.App, error) {
	if service.IntegrateLocalFileNoCacheRefresh != nil {
		return service.IntegrateLocalFileNoCacheRefresh(ctx, src, prompt)
	}
	return IntegrateFromLocalFileWithoutCacheRefresh(ctx, src, prompt)
}

func (service Service) Reintegrate(ctx context.Context, id string) (*models.App, error) {
	if service.ReintegrateExistingApp != nil {
		return service.ReintegrateExistingApp(ctx, id)
	}
	return nil, fmt.Errorf("reintegrate service is not configured")
}
