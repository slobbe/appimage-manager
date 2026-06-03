package integrate

import (
	"fmt"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

func errNotConfigured(name string) error {
	return fmt.Errorf("%s is not configured", name)
}

type AppStore interface {
	AddApp(app *models.App, overwrite bool) error
	RemoveApp(id string) error
	GetApp(id string) (*models.App, error)
	GetAllApps() (map[string]*models.App, error)
}

type Paths struct {
	AimDir       string
	DesktopDir   string
	TempDir      string
	IconThemeDir string
}
