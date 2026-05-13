package integrate

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

type Paths struct {
	AimDir       string
	DesktopDir   string
	TempDir      string
	IconThemeDir string
}

var defaultPaths Paths

func SetPaths(paths Paths) {
	defaultPaths = paths
}

func requirePaths() (Paths, error) {
	if defaultPaths.AimDir == "" || defaultPaths.DesktopDir == "" || defaultPaths.TempDir == "" || defaultPaths.IconThemeDir == "" {
		return Paths{}, fmt.Errorf("app paths are not configured")
	}
	return defaultPaths, nil
}
