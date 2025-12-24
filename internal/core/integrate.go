package core

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
)

func IntegrateAppImage(appImageSrc string, move bool) error {
	base := strings.TrimSuffix(filepath.Base(appImageSrc), filepath.Ext(appImageSrc))

	tempExtractDir := filepath.Join(config.TempDir, base)
	if err := os.MkdirAll(config.TempDir, 0755); err != nil {
		return err
	}

	// extract
	if err := ExtractAppImage(appImageSrc, tempExtractDir); err != nil {
		return err
	}

	// locate desktop and icon files
	desktopSrc, err := LocateDesktopFile(tempExtractDir)
	if err != nil {
		return err
	}

	iconSrc, err := LocateIcon(tempExtractDir)
	if err != nil {
		return err
	}

	info, err := ExtractAppInfo(desktopSrc)
	var appName, appVersion, appSlug string
	if err != nil {
		appName = base
		appSlug = util.Slugify(base)
		appVersion = "unknown"
		log.Fatal(err)
	} else {
		appName = info.Name
		appSlug = util.Slugify(info.Name)
		appVersion = info.Version
	}

	extractDir := filepath.Join(config.AimDir, appSlug)

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	// move desktop, icon, and appimage to extract dir
	var newAppImageSrc string
	if move {
		newAppImageSrc, err = util.Move(appImageSrc, filepath.Join(extractDir, appSlug+".AppImage"))
		if err != nil {
			return err
		}
	} else {
		newAppImageSrc, err = util.Copy(appImageSrc, filepath.Join(extractDir, appSlug+".AppImage"))
		if err != nil {
			return err
		}
	}

	newDesktopSrc, err := util.Move(desktopSrc, filepath.Join(extractDir, appSlug+".desktop"))
	if err != nil {
		return err
	}

	newIconSrc, err := util.Move(iconSrc, filepath.Join(extractDir, appSlug+filepath.Ext(iconSrc)))
	if err != nil {
		return err
	}

	os.RemoveAll(tempExtractDir)

	// make appimage executable
	if err := util.MakeExecutable(newAppImageSrc); err != nil {
		return err
	}

	if err := UpdateDesktopFile(newDesktopSrc, newAppImageSrc, newIconSrc); err != nil {
		return err
	}

	// make desktop symlink for system integration
	desktopLink := filepath.Join(config.DesktopDir, "aim-"+appSlug+".desktop")
	_ = os.Remove(desktopLink)
	if err := os.Symlink(newDesktopSrc, desktopLink); err != nil {
		return err
	}

	// add to db
	unlinkedDb, err := LoadDB(config.UnlinkedDbSrc)
	if err != nil {
		return err
	}

	_, exists := unlinkedDb.Apps[appSlug]
	if exists {
		delete(unlinkedDb.Apps, appSlug)
		if err := SaveDB(config.UnlinkedDbSrc, unlinkedDb); err != nil {
			return err
		}
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	sum, err := util.Sha256File(newAppImageSrc)
	if err != nil {
		return err
	}

	db.Apps[appSlug] = &App{
		Name:        appName,
		Slug:        appSlug,
		Version:     appVersion,
		AppImageSrc: newAppImageSrc,
		SHA256:      sum,
		DesktopSrc:  newDesktopSrc,
		DesktopLink: desktopLink,
		IconSrc:     newIconSrc,
		AddedAt:     NowISO(),
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	// refresh desktop cache best-effort
	_ = exec.Command("update-desktop-database", config.DesktopDir).Run()

	fmt.Printf("Successfully added %s v%s (id: %s)\n", appName, appVersion, appSlug)
	return nil
}
