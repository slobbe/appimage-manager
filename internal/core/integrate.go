package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type UpdateOverwritePrompt func(existing, incoming *models.UpdateSource) (bool, error)

func IntegrateFromLocalFile(ctx context.Context, src string, confirmUpdateOverwrite UpdateOverwritePrompt) (*models.App, error) {
	if !util.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be a .AppImage file")
	}

	src, err := util.MakeAbsolute(src)
	if err != nil {
		return nil, err
	}

	extractionData, err := ExtractAppImage(ctx, src)
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

	installedIconPath, desktopIconValue, err := InstallDesktopIcon(appID, extractionData.IconPath)
	if err != nil {
		return nil, err
	}
	extractionData.IconPath = installedIconPath

	if err := UpdateDesktopEntry(ctx, extractionData.DesktopEntryPath, extractionData.ExecPath, desktopIconValue); err != nil {
		return nil, err
	}

	if err := ValidateDesktopEntry(ctx, extractionData.DesktopEntryPath); err != nil {
		return nil, err
	}

	if err := util.MakeExecutable(extractionData.ExecPath); err != nil {
		return nil, err
	}

	desktopEntryLink, err := MakeDesktopLink(extractionData.DesktopEntryPath, "aim-"+appID+".desktop")
	if err != nil {
		return nil, err
	}

	var existingApp *models.App
	if appData, err := repo.GetApp(appID); err == nil {
		existingApp = appData
	} else if !strings.Contains(err.Error(), "does not exists in database") {
		return nil, err
	}

	timestampNow := util.NowISO()

	update := &models.UpdateSource{
		Kind: models.UpdateNone,
	}
	updateFound := false
	updateInfo, err := GetUpdateInfo(extractionData.ExecPath)
	if err == nil && updateInfo.Kind == models.UpdateZsync {
		updateFound = true
		update.Kind = models.UpdateZsync
		update.Zsync = &models.ZsyncUpdateSource{
			UpdateInfo: updateInfo.UpdateInfo,
			Transport:  updateInfo.Transport,
		}
	}

	if existingApp != nil {
		if !updateFound {
			update = existingApp.Update
		} else if existingApp.Update != nil && existingApp.Update.Kind != models.UpdateNone && confirmUpdateOverwrite != nil {
			overwrite, err := confirmUpdateOverwrite(existingApp.Update, update)
			if err != nil {
				return nil, err
			}
			if !overwrite {
				update = existingApp.Update
			}
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
		Kind: models.SourceLocalFile,
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

	if err := ValidateDesktopEntry(ctx, app.DesktopEntryPath); err != nil {
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
