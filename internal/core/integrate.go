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

func IntegrateAppImage(src string) (string, error) {
	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return "", err
	}

	inputType, src, err := IdentifyInput(src, db)
	if err != nil {
		return "", err
	}

	msg := ""

	switch inputType {
	case InputTypeUnknown:
		return "", fmt.Errorf("invalid input: %s", src)
	case InputTypeIntegrated:
		return "", fmt.Errorf("%s is already integrated", src)
	case InputTypeUnlinked:
		app, err := Reintegrate(src, db)
		if err != nil {
			return "", err
		}
		msg = fmt.Sprintf("successfully reintegrated %s v%s (%s)", app.Name, app.Version, app.Slug)
	case InputTypeAppImage:
		app, err := IntegrateNew(src, db)
		if err != nil {
			return "", err
		}
		msg = fmt.Sprintf("successfully integrated %s v%s (%s)", app.Name, app.Version, app.Slug)
	}

	// refresh desktop cache best-effort
	_ = exec.Command("update-desktop-database", config.DesktopDir).Run()

	return msg, nil
}

func IntegrateNew(src string, db *DB) (App, error) {
	src, err := util.MakeAbsolute(src)
	if err != nil {
		return App{}, err
	}

	appData, err := GetAppImage(src)
	if err != nil {
		return App{}, err
	}

	// make appimage executable
	if err := util.MakeExecutable(appData.AppImage); err != nil {
		return App{}, err
	}

	if err := UpdateDesktopFile(appData.Desktop, appData.AppImage, appData.Icon); err != nil {
		return App{}, err
	}

	// make desktop symlink for system integration
	appData.DesktopLink, err = MakeDesktopLink(appData.Desktop, filepath.Base("aim-"+appData.Slug+".desktop"))
	if err != nil {
		return App{}, err
	}

	appData.SHA256, err = util.Sha256File(appData.AppImage)
	if err != nil {
		return App{}, err
	}
	appData.SHA1, err = util.Sha1(appData.AppImage)
	if err != nil {
		return App{}, err
	}

	now := NowISO()
	appData.AddedAt = now
	appData.UpdatedAt = now

	db.Apps[appData.Slug] = &appData

	if err := SaveDB(config.DbSrc, db); err != nil {
		return App{}, err
	}

	return appData, nil
}

func Reintegrate(slug string, db *DB) (App, error) {
	app, exists := db.Apps[slug]
	if !exists {
		return App{}, fmt.Errorf("%s could not be found in app registry", slug)
	}

	// make desktop symlink for system integration
	desktopLink, err := MakeDesktopLink(app.Desktop, filepath.Base("aim-"+app.Slug+".desktop"))
	if err != nil {
		return *app, err
	}
	app.DesktopLink = desktopLink

	if err := SaveDB(config.DbSrc, db); err != nil {
		return *app, err
	}

	return *app, nil
}

func GetAppImage(appImageSrc string) (App, error) {
	data := App{
		AppImage:    appImageSrc,
		Name:        "",
		Slug:        "",
		Version:     "",
		Icon:        "",
		Desktop:     "",
		DesktopLink: "",
		AddedAt:     "",
		UpdatedAt:   "",
		SHA256:      "",
		SHA1:        "",
	}

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
		data.Version = "unknown"
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
	if _, err = util.Copy(appImageSrc, data.AppImage); err != nil {
		return data, err
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
