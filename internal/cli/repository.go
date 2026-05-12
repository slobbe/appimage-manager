package cli

import (
	"github.com/slobbe/appimage-manager/internal/cli/config"
	models "github.com/slobbe/appimage-manager/internal/domain"
	repo "github.com/slobbe/appimage-manager/internal/infra/repository"
)

func repositoryStore() *repo.Store {
	return repo.NewStore(config.DbSrc)
}

func defaultAddAppsBatch(apps []*models.App, overwrite bool) error {
	return repositoryStore().AddAppsBatch(apps, overwrite)
}

func defaultAddSingleApp(app *models.App, overwrite bool) error {
	return repositoryStore().AddApp(app, overwrite)
}

func getManagedApp(id string) (*models.App, error) {
	return repositoryStore().GetApp(id)
}

func getAllManagedApps() (map[string]*models.App, error) {
	return repositoryStore().GetAllApps()
}

func updateManagedApp(app *models.App) error {
	return repositoryStore().UpdateApp(app)
}

func updateCheckMetadataBatch(updates []repo.CheckMetadataUpdate) error {
	return repositoryStore().UpdateCheckMetadataBatch(updates)
}
