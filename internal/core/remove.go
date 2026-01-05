package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/config"
)

func RemoveAppImage(slug string, keep bool) (App, error) {
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return App{}, err
	}

	appData, exists := db.Apps[slug]
	if !exists {
		return App{}, fmt.Errorf("no app with slug %s exists", slug)
	}

	if err := os.Remove(appData.DesktopLink); err != nil {
		return *appData, fmt.Errorf("failed to remove desktop link: %w", err)
	}

	if keep {
		appData.DesktopLink = ""
	} else {
		delete(db.Apps, slug)

		appDir := filepath.Join(config.AimDir, appData.Slug)
		if err := os.RemoveAll(appDir); err != nil {
			return *appData, fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return *appData, err
	}

	return *appData, nil
}
