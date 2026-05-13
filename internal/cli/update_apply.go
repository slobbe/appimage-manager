package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	core "github.com/slobbe/appimage-manager/internal/app"
	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	util "github.com/slobbe/appimage-manager/internal/infra/helpers"
)

func persistManagedAppliedApps(ctx context.Context, apps []*models.App) error {
	if len(apps) == 0 {
		return nil
	}

	if err := addAppsBatch(apps, true); err == nil {
		return cleanupReplacedManagedApps(ctx, apps)
	}

	persistedApps := make([]*models.App, 0, len(apps))
	fallbackErrors := make([]string, 0)
	for _, app := range apps {
		if app == nil {
			continue
		}
		if err := addSingleApp(app, true); err != nil {
			fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s: %v", app.ID, err))
			continue
		}
		persistedApps = append(persistedApps, app)
	}

	if len(fallbackErrors) > 0 {
		return wrapWriteError(fmt.Errorf("failed to persist applied updates: %s", strings.Join(fallbackErrors, "; ")))
	}

	return cleanupReplacedManagedApps(ctx, persistedApps)
}

func cleanupReplacedManagedApps(ctx context.Context, apps []*models.App) error {
	replaced := map[string]struct{}{}
	for _, app := range apps {
		if app == nil {
			continue
		}
		replacesID := strings.TrimSpace(app.ReplacesID)
		if replacesID == "" || replacesID == strings.TrimSpace(app.ID) {
			continue
		}
		if _, seen := replaced[replacesID]; seen {
			continue
		}
		replaced[replacesID] = struct{}{}
		if _, err := removeManagedApp(ctx, replacesID, false); err != nil {
			return wrapWriteError(fmt.Errorf("failed to remove superseded app %s: %w", replacesID, err))
		}
	}

	return nil
}

func applyManagedUpdate(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageQueued})

	if strings.TrimSpace(update.URL) == "" {
		err := withUserGuidance(
			softwareError(fmt.Errorf("missing download URL")),
			"Can't download an update because the selected source did not provide a download URL.",
			fmt.Sprintf("Reconfigure %s with 'aim update --set %s ...'.", update.App.ID, update.App.ID),
		)
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	fileName := updateDownloadFilename(update.Asset, update.URL)
	downloadPath, err := stableDownloadDestination(update.URL, update.App.ID+"-"+fileName)
	if err != nil {
		err = wrapWriteError(err)
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}
	usedZsync := false
	if strings.TrimSpace(update.ZsyncURL) != "" {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageZsync})
		logOperationContextf(ctx, "Running zsync for %s", update.App.ID)
		if err := applyZsyncUpdate(ctx, update, downloadPath); err == nil {
			usedZsync = true
		}
	}

	if !usedZsync {
		emitManagedApplyEvent(reporter, managedApplyEvent{
			Stage:        managedApplyStageDownload,
			DownloadName: fileName,
		})
		logOperationContextf(ctx, "Downloading update for %s", update.App.ID)
		if err := downloadManagedRemoteAsset(ctx, update.URL, downloadPath, false, func(downloaded, total int64) {
			emitManagedApplyEvent(reporter, managedApplyEvent{
				Stage:         managedApplyStageDownload,
				Downloaded:    downloaded,
				DownloadTotal: total,
				DownloadName:  fileName,
			})
		}); err != nil {
			emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
			return nil, err
		}
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageVerify})
	logOperationContextf(ctx, "Verifying update for %s", update.App.ID)
	if err := verifyDownloadedUpdate(downloadPath, update); err != nil {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageIntegrate})
	logOperationContextf(ctx, "Integrating update for %s", update.App.ID)
	app, err := integrateManagedUpdate(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		return false, nil
	})
	if err != nil {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	if update.App != nil {
		app.Source = update.App.Source
		app.Update = update.App.Update
		if strings.TrimSpace(update.App.AddedAt) != "" {
			app.AddedAt = update.App.AddedAt
		}
		app.LastCheckedAt = update.App.LastCheckedAt
		if strings.TrimSpace(update.App.ID) != "" && strings.TrimSpace(update.App.ID) != strings.TrimSpace(app.ID) {
			app.ReplacesID = update.App.ID
		}
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{
		Stage:   managedApplyStageDone,
		Version: app.Version,
	})
	removeStagedDownload(downloadPath)
	return app, nil
}

func applyZsyncUpdate(ctx context.Context, update pendingManagedUpdate, destination string) error {
	if update.App == nil {
		return withReportableInternalError(fmt.Errorf("missing app"), "Can't apply an update because the managed app record is incomplete.")
	}
	if strings.TrimSpace(update.App.ExecPath) == "" {
		return withUserGuidance(
			notFoundError(fmt.Errorf("missing app exec path")),
			fmt.Sprintf("Can't apply an update for %s because the managed app record is missing its executable path.", update.App.ID),
			fmt.Sprintf("Reinstall %s.", update.App.ID),
		)
	}
	if strings.TrimSpace(update.ZsyncURL) == "" {
		return withUserGuidance(
			softwareError(fmt.Errorf("missing zsync url")),
			"Can't apply a delta update because the configured source is missing its zsync URL.",
			fmt.Sprintf("Reconfigure %s with 'aim update --set %s ...'.", update.App.ID, update.App.ID),
		)
	}

	binary, err := zsyncLookPath("zsync")
	if err != nil {
		return rewriteZsyncFailure(err)
	}

	cmd := zsyncCommandContext(ctx, binary, "-q", "-i", update.App.ExecPath, "-o", destination, update.ZsyncURL)
	cmd.Dir = filepath.Dir(destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return rewriteZsyncFailure(err)
		}
		return rewriteZsyncFailure(fmt.Errorf("%w: %s", err, msg))
	}

	if _, err := os.Stat(destination); err != nil {
		return wrapWriteError(err)
	}

	return nil
}

func verifyDownloadedUpdate(downloadPath string, update pendingManagedUpdate) error {
	expectedSHA256 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA256))
	expectedSHA1 := strings.ToLower(strings.TrimSpace(update.ExpectedSHA1))

	if expectedSHA256 != "" && expectedSHA1 != "" {
		sha256sum, sha1sum, err := util.Sha256AndSha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sha256sum) != expectedSHA256 {
			return rewriteChecksumError(fmt.Errorf("downloaded file sha256 mismatch"))
		}
		if strings.ToLower(sha1sum) != expectedSHA1 {
			return rewriteChecksumError(fmt.Errorf("downloaded file sha1 mismatch"))
		}
		return nil
	}

	if expectedSHA256 != "" {
		sum, err := util.Sha256File(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return rewriteChecksumError(fmt.Errorf("downloaded file sha256 mismatch"))
		}
	}

	if expectedSHA1 != "" {
		sum, err := util.Sha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA1 {
			return rewriteChecksumError(fmt.Errorf("downloaded file sha1 mismatch"))
		}
	}

	return nil
}

type downloadDescriptionContextKey struct{}

func withDownloadDescription(ctx context.Context, description string) context.Context {
	description = strings.TrimSpace(description)
	if description == "" {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, downloadDescriptionContextKey{}, description)
}

func downloadDescriptionFromContext(ctx context.Context, fallback string) string {
	if ctx != nil {
		if description, ok := ctx.Value(downloadDescriptionContextKey{}).(string); ok {
			if description = strings.TrimSpace(description); description != "" {
				return description
			}
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "Downloading update"
	}
	return fallback
}

func downloadUpdateAsset(ctx context.Context, assetURL, destination string, interactive bool) error {
	description := downloadDescriptionFromContext(ctx, "Downloading update")
	return downloadUpdateAssetWithDescription(ctx, assetURL, destination, description, interactive, nil)
}

func downloadUpdateAssetWithProgress(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(downloaded, total int64)) error {
	description := downloadDescriptionFromContext(ctx, "Downloading update")
	return downloadUpdateAssetWithDescription(ctx, assetURL, destination, description, interactive, onProgress)
}

func downloadUpdateAssetWithDescription(ctx context.Context, assetURL, destination, description string, interactive bool, onProgress func(downloaded, total int64)) error {
	description = strings.TrimSpace(description)
	if description == "" {
		description = "Downloading update"
	}

	ownsProgress := onProgress == nil
	var progress progressHandle
	if ownsProgress {
		progress = newProcessSpinnerProgress(description, interactive)
	}
	defer func() {
		if progress != nil && interactive {
			progress.Clear()
		}
	}()

	stagedMeta, err := loadStagedDownloadMetadata(destination)
	if err != nil {
		return wrapWriteError(err)
	}

	meta := downloadMetadataFromStaged(stagedMeta)
	if meta != nil && strings.TrimSpace(meta.URL) != "" && strings.TrimSpace(meta.URL) != strings.TrimSpace(assetURL) {
		removeStagedDownload(destination)
		meta = nil
	}

	var (
		downloaded   int64
		total        int64
		progressMode = progressModeSpinner
	)

	logOperationContextf(ctx, "HTTP GET %s", assetURL)
	downloader := download.Downloader{Client: core.SharedDownloadHTTPClient()}
	resultMeta, err := downloader.Download(ctx, download.Request{
		URL:         assetURL,
		Destination: destination,
		Metadata:    meta,
	}, func(event download.Progress) {
		delta := event.Downloaded - downloaded
		downloaded = event.Downloaded
		total = event.Total

		if ownsProgress && interactive && progress != nil && progressMode == progressModeSpinner {
			progress.Clear()
			progress = newProcessByteProgress(description, total, true)
			if total > 0 {
				progressMode = progressModeBytes
			}
			if downloaded > 0 {
				progress.Add(downloaded)
			}
			delta = 0
		}

		if onProgress != nil {
			onProgress(downloaded, total)
		}
		if ownsProgress && interactive && progress != nil && delta > 0 {
			progress.Add(delta)
		}
		if err := saveStagedDownloadMetadata(destination, stagedMetadataFromDownload(&event.Metadata)); err != nil {
			logOperationContextf(ctx, "failed to save staged download metadata: %v", err)
		}
	})
	if resultMeta != nil {
		if err := saveStagedDownloadMetadata(destination, stagedMetadataFromDownload(resultMeta)); err != nil {
			return wrapWriteError(err)
		}
	}
	if err != nil {
		var statusErr *download.StatusError
		if errors.As(err, &statusErr) {
			return withUserGuidance(
				unavailableError(statusErr),
				fmt.Sprintf("Can't download update: server returned %s.", statusErr.Status),
				"Try again later or check whether the upstream release is available.",
			)
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return wrapWriteError(pathErr)
		}
		return rewriteNetworkDownloadError(err)
	}

	if ownsProgress && interactive {
		if progress != nil {
			if progressMode == progressModeBytes {
				progress.Finish()
			} else {
				progress.Clear()
				writeProcessLogf("Downloaded %s\n", formatByteSize(downloaded))
			}
			progress = nil
		}
	} else {
		if onProgress != nil {
			onProgress(downloaded, total)
		}
		if ownsProgress {
			writeProcessLogf("  Downloaded %s\n", formatByteSize(downloaded))
		}
	}

	return nil
}

func formatByteSize(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%dB", value)
	}

	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value)
	unit := -1
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}

	rounded := strconv.FormatFloat(size, 'f', 1, 64)
	return rounded + units[unit]
}
