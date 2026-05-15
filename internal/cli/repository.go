package cli

import (
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appremove "github.com/slobbe/appimage-manager/internal/app/remove"
	"github.com/slobbe/appimage-manager/internal/cli/config"
	models "github.com/slobbe/appimage-manager/internal/domain"
	repo "github.com/slobbe/appimage-manager/internal/infra/repository"
)

func repositoryStore() *repo.Store {
	return repo.NewStore(config.DbSrc)
}

func configureRepositoryStores() {
	appintegrate.SetStore(repositoryStore())
	appremove.SetStore(repositoryStore())
}

type checkMetadataUpdate struct {
	ID            string
	Checked       bool
	Available     bool
	Latest        string
	LastCheckedAt string
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

func updateCheckMetadataBatch(updates []checkMetadataUpdate) error {
	repositoryUpdates := make([]repo.CheckMetadataUpdate, 0, len(updates))
	for _, update := range updates {
		repositoryUpdates = append(repositoryUpdates, repo.CheckMetadataUpdate{
			ID:            update.ID,
			Checked:       update.Checked,
			Available:     update.Available,
			Latest:        update.Latest,
			LastCheckedAt: update.LastCheckedAt,
		})
	}
	return repositoryStore().UpdateCheckMetadataBatch(repositoryUpdates)
}
