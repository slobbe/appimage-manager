package app

import (
	"fmt"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type AppStore interface {
	AddApp(app *models.App, overwrite bool) error
	RemoveApp(id string) error
	GetApp(id string) (*models.App, error)
	GetAllApps() (map[string]*models.App, error)
}

var defaultStore AppStore

func SetStore(store AppStore) {
	defaultStore = store
}

func requireStore() (AppStore, error) {
	if defaultStore == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	return defaultStore, nil
}
