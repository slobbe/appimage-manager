package integrate

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	core "github.com/slobbe/appimage-manager/internal/app"
	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

type UpdateOverwritePrompt func(existing, incoming *models.UpdateSource) (bool, error)

var getEmbeddedUpdateInfo = core.GetUpdateInfo

func IntegrateFromLocalFile(ctx context.Context, src string, confirmUpdateOverwrite UpdateOverwritePrompt) (*models.App, error) {
	return integrateFromLocalFile(ctx, src, confirmUpdateOverwrite, true, true)
}

func IntegrateFromLocalFileWithoutCacheRefresh(ctx context.Context, src string, confirmUpdateOverwrite UpdateOverwritePrompt) (*models.App, error) {
	return integrateFromLocalFile(ctx, src, confirmUpdateOverwrite, false, true)
}

func IntegrateFromLocalFileWithoutCacheRefreshOrPersist(ctx context.Context, src string, confirmUpdateOverwrite UpdateOverwritePrompt) (*models.App, error) {
	return integrateFromLocalFile(ctx, src, confirmUpdateOverwrite, false, false)
}

func integrateFromLocalFile(ctx context.Context, src string, confirmUpdateOverwrite UpdateOverwritePrompt, refreshCaches bool, persist bool) (*models.App, error) {
	store, err := requireStore()
	if err != nil {
		return nil, err
	}
	paths, err := requirePaths()
	if err != nil {
		return nil, err
	}

	if !fsys.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be a .AppImage file")
	}

	src, err = fsys.MakeAbsolute(src)
	if err != nil {
		return nil, err
	}

	extractionData, err := core.ExtractAppImage(ctx, src)
	if err != nil {
		return nil, err
	}

	appInfo, err := core.GetAppInfo(ctx, extractionData.DesktopEntryPath)
	if err != nil {
		return nil, err
	}
	if extractionData.DesktopStem != "" {
		appInfo.DesktopStem = extractionData.DesktopStem
		appInfo.ID = extractionData.DesktopStem
	}

	tmpDir := (*extractionData).Dir
	defer func() {
		_ = fsys.RemoveAll(tmpDir)
	}()

	var updateFromAppImage *models.UpdateSource
	if updateInfo, err := getEmbeddedUpdateInfo(extractionData.ExecPath); err == nil && updateInfo.Kind == models.UpdateZsync {
		updateFromAppImage = &models.UpdateSource{
			Kind: models.UpdateZsync,
			Zsync: &models.ZsyncUpdateSource{
				UpdateInfo: updateInfo.UpdateInfo,
				Transport:  updateInfo.Transport,
			},
		}
	}

	timestampNow := core.NowISO()
	addedAt := timestampNow
	lastCheckedAt := ""
	latestVersion := ""
	updateAvailable := false

	update := &models.UpdateSource{
		Kind: models.UpdateNone,
	}
	updateFound := updateFromAppImage != nil
	if updateFound {
		update = updateFromAppImage
	}

	source := models.Source{
		Kind: models.SourceLocalFile,
		LocalFile: &models.LocalFileSource{
			IntegratedAt: timestampNow,
			OriginalPath: src,
		},
	}

	upstreamID := appInfo.ID
	incomingIdentity := &models.App{
		ID:     upstreamID,
		Name:   appInfo.Name,
		Source: source,
		Update: update,
	}

	appID, replacementApp, err := resolveManagedAppID(store, appInfo.Name, upstreamID, src, incomingIdentity)
	if err != nil {
		return nil, err
	}

	var existingApp *models.App
	if replacementApp != nil {
		existingApp = replacementApp
	} else if appData, err := store.GetApp(appID); err == nil {
		existingApp = appData
	} else if !strings.Contains(err.Error(), "does not exists in database") {
		return nil, err
	}

	if existingApp != nil {
		if strings.TrimSpace(existingApp.AddedAt) != "" {
			addedAt = existingApp.AddedAt
		}
		lastCheckedAt = existingApp.LastCheckedAt

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

	outDir := filepath.Join(paths.AimDir, appID)
	if err := fsys.EnsureDir(outDir); err != nil {
		return nil, err
	}

	extractionData.Dir = outDir

	if extractionData.ExecPath, err = fsys.Move(extractionData.ExecPath, filepath.Join(extractionData.Dir, appID+filepath.Ext(extractionData.ExecPath))); err != nil {
		return nil, err
	}
	if extractionData.DesktopEntryPath, err = fsys.Move(extractionData.DesktopEntryPath, filepath.Join(extractionData.Dir, appID+filepath.Ext(extractionData.DesktopEntryPath))); err != nil {
		return nil, err
	}
	if extractionData.IconPath, err = fsys.Move(extractionData.IconPath, filepath.Join(extractionData.Dir, appID+filepath.Ext(extractionData.IconPath))); err != nil {
		return nil, err
	}

	installedIconPath, desktopIconValue, err := InstallDesktopIcon(appID, extractionData.IconPath)
	if err != nil {
		return nil, err
	}
	extractionData.IconPath = installedIconPath

	if existingApp != nil && replacementApp == nil {
		removeStaleInstalledIcon(store, existingApp.IconPath, installedIconPath, appID)
	}

	if err := core.UpdateDesktopEntry(ctx, extractionData.DesktopEntryPath, extractionData.ExecPath, desktopIconValue); err != nil {
		return nil, err
	}

	if err := ValidateDesktopEntry(ctx, extractionData.DesktopEntryPath); err != nil {
		return nil, err
	}

	if err := fsys.MakeExecutable(extractionData.ExecPath); err != nil {
		return nil, err
	}

	desktopEntryLink, err := MakeDesktopLink(extractionData.DesktopEntryPath, appID+".desktop", "aim-"+appID+".desktop")
	if err != nil {
		return nil, err
	}

	var (
		sha256sum string
		sha1sum   string
		hashErr   error
	)

	var postProcessWG sync.WaitGroup
	taskCount := 1
	if refreshCaches {
		taskCount++
	}
	postProcessWG.Add(taskCount)

	if refreshCaches {
		go func() {
			defer postProcessWG.Done()
			refreshDesktopIntegrationCaches(ctx)
		}()
	}

	go func() {
		defer postProcessWG.Done()
		sha256sum, sha1sum, hashErr = fsys.Sha256AndSha1(extractionData.ExecPath)
	}()

	postProcessWG.Wait()
	if hashErr != nil {
		return nil, hashErr
	}

	app := &models.App{
		Name:             appInfo.Name,
		ID:               appID,
		Version:          appInfo.Version,
		ExecPath:         extractionData.ExecPath,
		DesktopEntryPath: extractionData.DesktopEntryPath,
		DesktopEntryLink: desktopEntryLink,
		IconPath:         extractionData.IconPath,
		AddedAt:          addedAt,
		UpdatedAt:        timestampNow,
		LastCheckedAt:    lastCheckedAt,
		UpdateAvailable:  updateAvailable,
		LatestVersion:    latestVersion,
		SHA256:           sha256sum,
		SHA1:             sha1sum,
		Source:           source,
		Update:           update,
	}

	if replacementApp != nil {
		app.ReplacesID = replacementApp.ID
	}

	if persist {
		if err := store.AddApp(app, true); err != nil {
			return nil, err
		}
		if replacementApp != nil {
			if err := removeManagedApp(ctx, store, replacementApp.ID); err != nil {
				return nil, err
			}
			app.ReplacesID = ""
		}
	}

	return app, nil
}

func removeManagedApp(ctx context.Context, store AppStore, id string) error {
	_ = ctx
	paths, err := requirePaths()
	if err != nil {
		return err
	}
	appData, err := store.GetApp(id)
	if err != nil {
		return fmt.Errorf("no app with id %s exists", id)
	}
	if err := fsys.RemoveFileIfExists(appData.DesktopEntryLink); err != nil {
		return fmt.Errorf("failed to remove desktop link: %w", err)
	}
	if appData.IconPath != "" {
		appDir := filepath.Join(paths.AimDir, appData.ID)
		iconPath := filepath.Clean(appData.IconPath)
		if iconPath != appDir && !strings.HasPrefix(iconPath, appDir+string(filepath.Separator)) {
			_ = fsys.RemoveFileIfExists(iconPath)
		}
	}
	if err := store.RemoveApp(appData.ID); err != nil {
		return err
	}
	if err := fsys.RemoveAll(filepath.Join(paths.AimDir, appData.ID)); err != nil {
		return fmt.Errorf("failed to remove app dir: %w", err)
	}
	return nil
}

func IntegrateExisting(ctx context.Context, id string) (*models.App, error) {
	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	store, err := requireStore()
	if err != nil {
		return nil, err
	}

	app, err := store.GetApp(id)
	if err != nil {
		return app, err
	}

	if err := fsys.MakeExecutable(app.ExecPath); err != nil {
		return nil, err
	}

	if err := ValidateDesktopEntry(ctx, app.DesktopEntryPath); err != nil {
		return nil, err
	}

	app.DesktopEntryLink, err = MakeDesktopLink(app.DesktopEntryPath, app.ID+".desktop", "aim-"+app.ID+".desktop")
	if err != nil {
		return app, err
	}

	refreshDesktopIntegrationCaches(ctx)

	if err := store.AddApp(app, true); err != nil {
		return app, err
	}

	return app, nil
}

func MakeDesktopLink(src, preferredName, fallbackName string) (string, error) {
	paths, err := requirePaths()
	if err != nil {
		return "", err
	}

	if src == "" {
		return "", fmt.Errorf("source cannot be empty")
	}

	if preferredName == "" && fallbackName == "" {
		return "", fmt.Errorf("desktop link name cannot be empty")
	}

	desktopLink, err := desktop.ResolveDesktopLinkPath(paths.DesktopDir, src, preferredName, fallbackName)
	if err != nil {
		return "", err
	}

	if err := fsys.ReplaceSymlink(src, desktopLink); err != nil {
		return "", err
	}

	return desktopLink, nil
}

func removeStaleInstalledIcon(store AppStore, oldPath, newPath, appID string) {
	paths, err := requirePaths()
	if err != nil {
		return
	}

	oldPath = filepath.Clean(strings.TrimSpace(oldPath))
	newPath = filepath.Clean(strings.TrimSpace(newPath))
	if oldPath == "." || oldPath == "" || oldPath == newPath {
		return
	}

	appDir := filepath.Join(paths.AimDir, appID)
	if oldPath == appDir || strings.HasPrefix(oldPath, appDir+string(filepath.Separator)) {
		return
	}

	allApps, err := store.GetAllApps()
	if err != nil {
		return
	}
	for _, app := range allApps {
		if app == nil || strings.TrimSpace(app.ID) == appID {
			continue
		}
		if filepath.Clean(strings.TrimSpace(app.IconPath)) == oldPath {
			return
		}
	}

	_ = fsys.RemoveFileIfExists(oldPath)
}
