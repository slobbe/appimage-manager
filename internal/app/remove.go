package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

func Remove(ctx context.Context, id string, unlink bool) (*models.App, error) {
	store, err := requireStore()
	if err != nil {
		return nil, err
	}

	return remove(ctx, store, id, unlink)
}

func remove(ctx context.Context, store AppStore, id string, unlink bool) (*models.App, error) {
	paths, err := requirePaths()
	if err != nil {
		return nil, err
	}

	appData, err := store.GetApp(id)
	if err != nil {
		return nil, fmt.Errorf("no app with id %s exists", id)
	}

	if err := fsys.RemoveFileIfExists(appData.DesktopEntryLink); err != nil {
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
				_ = fsys.RemoveFileIfExists(iconPath)
			}
		}

		if err := fsys.RemoveAll(appDir); err != nil {
			return appData, fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	refreshDesktopIntegrationCaches(ctx)

	return appData, nil
}
