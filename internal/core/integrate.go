package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func IntegrateFromLocalFile(ctx context.Context, src string) (*models.App, error) {
	if !util.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be a .AppImage file")
	}

	src, err := util.MakeAbsolute(src)
	if err != nil {
		return nil, err
	}

	extractionData, err := NExtractAppImage(ctx, src)
	if err != nil {
		return nil, err
	}

	appInfo, err := GetAppInfo(ctx, extractionData.DesktopEntryPath)
	if err != nil {
		return nil, err
	}

	appID := appInfo.ID

	outDir := filepath.Join(config.AimDir, appID)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	tmpDir := (*extractionData).Dir
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	extractionData.Dir = outDir

	if extractionData.ExecPath, err = util.Move(extractionData.ExecPath, filepath.Join(extractionData.Dir, appID+filepath.Ext(extractionData.ExecPath))); err != nil {
		return nil, err
	}
	if extractionData.DesktopEntryPath, err = util.Move(extractionData.DesktopEntryPath, filepath.Join(extractionData.Dir, appID+filepath.Ext(extractionData.DesktopEntryPath))); err != nil {
		return nil, err
	}
	if extractionData.IconPath, err = util.Move(extractionData.IconPath, filepath.Join(extractionData.Dir, appID+filepath.Ext(extractionData.IconPath))); err != nil {
		return nil, err
	}

	if err := NUpdateDesktopEntry(ctx, extractionData.DesktopEntryPath, extractionData.ExecPath, extractionData.IconPath); err != nil {
		return nil, err
	}

	if err := util.MakeExecutable(extractionData.ExecPath); err != nil {
		return nil, err
	}

	desktopEntryLink, err := MakeDesktopLink(extractionData.DesktopEntryPath, "aim-"+appID+".desktop")
	if err != nil {
		return nil, err
	}

	timestampNow := util.NowISO()

	update := &models.UpdateSource{
		Kind: "none",
	}
	updateInfo, err := GetUpdateInfo(extractionData.ExecPath)
	if err == nil && updateInfo.Kind == "zsync" {
		update.Kind = "zsync"
		update.Zsync = &models.ZsyncSource{
			UpdateInfo:   updateInfo.UpdateInfo,
			UpdateUrl:    updateInfo.UpdateUrl,
			DownloadedAt: timestampNow,
			Transport:    updateInfo.Transport,
		}
	}

	_ = exec.Command("update-desktop-database", config.DesktopDir).Run()

	sha256sum, err := util.Sha256File(extractionData.ExecPath)
	if err != nil {
		return nil, err
	}

	sha1sum, err := util.Sha1(extractionData.ExecPath)
	if err != nil {
		return nil, err
	}

	source := models.Source{
		Kind: "local_file",
		LocalFile: &models.LocalFileSource{
			IntegratedAt: timestampNow,
			OriginalPath: src,
		},
	}

	app := &models.App{
		Name:             appInfo.Name,
		ID:               appInfo.ID,
		Version:          appInfo.Version,
		ExecPath:         extractionData.ExecPath,
		DesktopEntryPath: extractionData.DesktopEntryPath,
		DesktopEntryLink: desktopEntryLink,
		IconPath:         extractionData.IconPath,
		AddedAt:          timestampNow,
		UpdatedAt:        timestampNow,
		SHA256:           sha256sum,
		SHA1:             sha1sum,
		Source:           source,
		Update:           update,
	}

	if err := repo.AddApp(app, true); err != nil {
		return nil, err
	}

	return app, nil
}

func IntegrateExisting(ctx context.Context, id string) (*models.App, error) {
	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return app, err
	}

	if err := util.MakeExecutable(app.ExecPath); err != nil {
		return nil, err
	}

	app.DesktopEntryLink, err = MakeDesktopLink(app.DesktopEntryPath, "aim-"+app.ID+".desktop")
	if err != nil {
		return app, err
	}

	_ = exec.Command("update-desktop-database", config.DesktopDir).Run()

	if err := repo.AddApp(app, true); err != nil {
		return app, err
	}

	return app, nil
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

// ======================================================
/*
func IntegrateAppImage(src string) (*models.App, error) {
	inputType, src, err := IdentifyInput(src)

	var app *models.App
	switch inputType {
	case InputTypeUnknown:
		return nil, fmt.Errorf("invalid input: %s", src)
	case InputTypeIntegrated:
		return nil, fmt.Errorf("%s is already integrated", src)
	case InputTypeUnlinked:
		app, err = IntegrateExisting(src)
		if err != nil {
			return nil, err
		}
	case InputTypeAppImage:
		app, err = IntegrateNew(src)
		if err != nil {
			return nil, err
		}
	}

	// refresh desktop cache best-effort
	_ = exec.Command("update-desktop-database", config.DesktopDir).Run()

	return app, nil
}

func IntegrateNew(src string) (*models.App, error) {
	src, err := util.MakeAbsolute(src)
	if err != nil {
		return nil, err
	}

	appData, err := GetAppImage(src)
	if err != nil {
		return nil, err
	}

	// make appimage executable
	if err := util.MakeExecutable(appData.AppImage); err != nil {
		return nil, err
	}

	if err := UpdateDesktopFile(appData.Desktop, appData.AppImage, appData.Icon); err != nil {
		return nil, err
	}

	// make desktop symlink for system integration
	if appData.DesktopLink, err = MakeDesktopLink(appData.Desktop, filepath.Base("aim-"+appData.Slug+".desktop")); err != nil {
		return nil, err
	}

	_, err = SetMetadata(appData)

	now := util.NowISO()
	appData.AddedAt = now
	appData.UpdatedAt = now

	if err := repo.AddApp(appData, true); err != nil {
		return appData, err
	}

	return appData, nil
}

func IntegrateExisting(slug string) (*models.App, error) {
	app, err := repo.GetApp(slug)
	if err != nil {
		return nil, fmt.Errorf("%s could not be found in app registry", slug)
	}

	// make desktop symlink for system integration
	desktopLink, err := MakeDesktopLink(app.Desktop, filepath.Base("aim-"+app.Slug+".desktop"))
	if err != nil {
		return app, err
	}
	app.DesktopLink = desktopLink

	SetMetadata(app)

	now := util.NowISO()
	app.AddedAt = now
	app.UpdatedAt = now

	if err := repo.AddApp(app, true); err != nil {
		return app, err
	}

	return app, nil
}

func GetAppImage(appImageSrc string) (*models.App, error) {
	data := &models.App{
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

	defer os.RemoveAll(tempExtractDir)

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

	return data, nil
}

func SetMetadata(appData *models.App) (*models.App, error) {
	var err error

	if appData.SHA256, err = util.Sha256File(appData.AppImage); err != nil {
		return appData, err
	}

	if appData.SHA1, err = util.Sha1(appData.AppImage); err != nil {
		return appData, err
	}

	if appData.Type, err = Type(appData.AppImage); err != nil {
		return appData, err
	}

	if appData.Type == "type-2" {
		appData.UpdateInfo, _ = GetUpdateInfo(appData.AppImage)
	}

	return appData, nil
}
*/
