package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func RemoveAppImage(slug string, keep bool) (*models.App, error) {
	appData, err := repo.GetApp(slug)
	if err != nil {
		return nil, fmt.Errorf("no app with slug %s exists", slug)
	}

	if err := os.Remove(appData.DesktopLink); err != nil {
		return nil, fmt.Errorf("failed to remove desktop link: %w", err)
	}

	if keep {
		appData.DesktopLink = ""
		if err := repo.AddApp(appData, true); err != nil {
			return appData, err
		}
		
	} else {
		if err := repo.RemoveApp(appData.Slug); err != nil {
			return appData, err
		}

		appDir := filepath.Join(config.AimDir, appData.Slug)
		if err := os.RemoveAll(appDir); err != nil {
			return appData, fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	return appData, nil
}
