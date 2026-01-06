package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func Remove(ctx context.Context, id string, keep bool) (*models.App, error) {
	appData, err := repo.GetApp(id)
	if err != nil {
		return nil, fmt.Errorf("no app with id %s exists", id)
	}

	if err := os.Remove(appData.DesktopEntryLink); err != nil {
		return nil, fmt.Errorf("failed to remove desktop link: %w", err)
	}

	if keep {
		appData.DesktopEntryLink = ""
		if err := repo.AddApp(appData, true); err != nil {
			return appData, err
		}
	} else {
		if err := repo.RemoveApp(appData.ID); err != nil {
			return appData, err
		}

		appDir := filepath.Join(config.AimDir, appData.ID)
		if err := os.RemoveAll(appDir); err != nil {
			return appData, fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	return appData, nil
}
