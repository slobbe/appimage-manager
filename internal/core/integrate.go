package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
)

func IntegrateAppImage(appImageSrc string, move bool) error {
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	inputType, err := IdentifyInput(appImageSrc, db)
	if err != nil {
		return err
	}

	switch inputType {
	case InputTypeUnknown:
		return fmt.Errorf("invalid input")
	case InputTypeIntegrated:
		fmt.Println("already integrated")
		return nil
	case InputTypeUnlinked:
		fmt.Println("unlinked")
		db, err := LoadDB(config.DbSrc)
		if err != nil {
			return err
		}

		app, _ := db.Apps[appImageSrc]

		if app == nil {
			return fmt.Errorf("app entry is nil: %s", appImageSrc)
		}

		desktopLink, err := MakeDesktopLink(app.Desktop, filepath.Base("aim-"+app.Slug+".desktop"))
		if err != nil {
			return err
		}

		fmt.Println(desktopLink)

		db.Apps[appImageSrc].DesktopLink = desktopLink

	case InputTypeAppImage:
		fmt.Println("appimage")
		if !filepath.IsAbs(appImageSrc) {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			appImageSrc = filepath.Join(dir, appImageSrc)
		}

		appData, err := GetAppImage(appImageSrc, move)
		if err != nil {
			return err
		}

		// make appimage executable
		if err := util.MakeExecutable(appData.AppImage); err != nil {
			return err
		}

		if err := UpdateDesktopFile(appData.Desktop, appData.AppImage, appData.Icon); err != nil {
			return err
		}

		// make desktop symlink for system integration
		desktopLink, err := MakeDesktopLink(appData.Desktop, filepath.Base("aim-"+appData.Slug+".desktop"))
		if err != nil {
			return err
		}

		sum, err := util.Sha256File(appData.AppImage)
		if err != nil {
			return err
		}

		db.Apps[appData.Slug] = &App{
			Name:        appData.Name,
			Slug:        appData.Slug,
			Version:     appData.Version,
			AppImage:    appData.AppImage,
			SHA256:      sum,
			Desktop:     appData.Desktop,
			DesktopLink: desktopLink,
			Icon:        appData.Icon,
			AddedAt:     NowISO(),
		}
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}
	// refresh desktop cache best-effort
	_ = exec.Command("update-desktop-database", config.DesktopDir).Run()

	//fmt.Printf("Successfully added %s v%s (ID: %s)\n", appData.Name, appData.Version, appData.Slug)

	return nil
}

func MakeDesktopLink(src string, name string) (string, error) {
	if src == "" {
		return "", fmt.Errorf("source cannot be empty")
	}

	if name == "" {
		return "", fmt.Errorf("name cannot be empty")
	}

	desktopLink := filepath.Join(config.DesktopDir, name)

	_ = os.Remove(desktopLink)

	if err := os.Symlink(src, desktopLink); err != nil {
		return src, err
	}

	return desktopLink, nil
}

func GetAppImage(appImageSrc string, move bool) (App, error) {
	data := App{appImageSrc, "", "", "", "", "", "", "", ""}

	fileName := strings.TrimSuffix(filepath.Base(appImageSrc), filepath.Ext(appImageSrc))
	tempExtractDir := filepath.Join(config.TempDir, fileName)

	if err := os.MkdirAll(config.TempDir, 0755); err != nil {
		return data, err
	}

	if err := ExtractAppImage(appImageSrc, tempExtractDir); err != nil {
		return data, err
	}

	// locate desktop and icon files
	desktopSrc, err := LocateDesktopFile(tempExtractDir)
	if err != nil {
		return data, err
	}

	iconSrc, err := LocateIcon(tempExtractDir)
	if err != nil {
		return data, err
	}

	info, err := ExtractAppInfo(desktopSrc)

	if err != nil {
		data.Name = fileName
		data.Slug = util.Slugify(fileName)
		data.Version = "N/A"
	} else {
		data.Name = info.Name
		data.Slug = util.Slugify(info.Name)
		data.Version = info.Version
	}

	extractDir := filepath.Join(config.AimDir, data.Slug)

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return data, err
	}

	// move desktop, icon, and appimage to extract dir
	data.AppImage = filepath.Join(extractDir, data.Slug+".AppImage")

	if move {
		if _, err = util.Move(appImageSrc, data.AppImage); err != nil {
			return data, err
		}
	} else {
		if _, err = util.Copy(appImageSrc, data.AppImage); err != nil {
			return data, err
		}
	}

	data.Desktop = filepath.Join(extractDir, data.Slug+".desktop")
	if _, err := util.Move(desktopSrc, data.Desktop); err != nil {
		return data, err
	}

	data.Icon = filepath.Join(extractDir, data.Slug+filepath.Ext(iconSrc))
	if _, err := util.Move(iconSrc, filepath.Join(extractDir, data.Slug+filepath.Ext(iconSrc))); err != nil {
		return data, err
	}

	_ = os.RemoveAll(tempExtractDir)

	return data, nil
}
