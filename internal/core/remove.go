package core

import (
	"fmt"
	"os"
	"path/filepath"
)

func RemoveAppImage(slug string, keep bool) error {
	home, _ := os.UserHomeDir()

	aimDir := filepath.Join(home, ".local/share/appimage-manager")
	dbPath := filepath.Join(aimDir, "apps.json")
	unlinkedDbPath := filepath.Join(aimDir, "unlinked.json")

	fmt.Println(aimDir, dbPath, unlinkedDbPath)

	db, err := LoadDB(dbPath)
	if err != nil {
		return err
	}

	appData, exists := db.Apps[slug]
	if !exists {
		return fmt.Errorf("no app with slug %s exists", slug)
	}

	delete(db.Apps, slug)
	if err := SaveDB(dbPath, db); err != nil {
		return err
	}

	if err := os.Remove(appData.DesktopLink); err != nil {
		return fmt.Errorf("failed to remove desktop link: %w", err)
	}

	if keep {
		unlinkedDb, err := LoadDB(unlinkedDbPath)
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
		if err := SaveDB(unlinkedDbPath, unlinkedDb); err != nil {
			return err
		}
	} else {
		appDir := filepath.Join(aimDir, appData.Slug)
		if err := os.RemoveAll(appDir); err != nil {
			return fmt.Errorf("failed to remove app dir: %w", err)
		}
	}

	return nil
}
