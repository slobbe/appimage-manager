package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/config"
)

func RemoveAppImage(slug string, keep bool) error {
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	appData, exists := db.Apps[slug]
	if !exists {
		return fmt.Errorf("no app with slug %s exists", slug)
	}

	delete(db.Apps, slug)
	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	if err := os.Remove(appData.DesktopLink); err != nil {
		return fmt.Errorf("failed to remove desktop link: %w", err)
	}

	if keep {
		unlinkedDb, err := LoadDB(config.UnlinkedDbSrc)
		if err != nil {
			return err
		}
		unlinkedDb.Apps[slug] = &App{
			Name:        appData.Name,
			Slug:        appData.Slug,
			Version:     appData.Version,
			AppImageSrc: appData.AppImageSrc,
			SHA256:      appData.SHA256,
			DesktopSrc:  appData.DesktopSrc,
			DesktopLink: "",
			IconSrc:     appData.IconSrc,
			AddedAt:     appData.AddedAt,
		}
		if err := SaveDB(config.UnlinkedDbSrc, unlinkedDb); err != nil {
			return err
		}
	} else {
		appDir := filepath.Join(config.AimDir, appData.Slug)
		if err := os.RemoveAll(appDir); err != nil {
			return fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	return nil
}
