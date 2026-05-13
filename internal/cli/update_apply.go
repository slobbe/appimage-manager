package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app/clock"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/cli/config"
	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	"github.com/slobbe/appimage-manager/internal/infra/zsync"
)

var (
	downloadManagedRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(int64, int64)) error {
		return appupdate.Service{
			TempDir:    config.TempDir,
			HTTPClient: download.SharedHTTPClient(),
			NowISO:     clock.NowISO,
		}.DownloadManagedUpdateAsset(ctx, assetURL, destination, onProgress)
	}
	integrateManagedUpdate = appintegrate.IntegrateFromLocalFileWithoutCacheRefreshOrPersist
	zsyncLookPath          = exec.LookPath
	zsyncCommandContext    = exec.CommandContext
)

func managedUpdateService() appupdate.Service {
	return appupdate.Service{
		TempDir:    config.TempDir,
		HTTPClient: download.SharedHTTPClient(),
		NowISO:     clock.NowISO,
		Zsync: zsync.Runner{
			LookPath:       zsyncLookPath,
			CommandContext: zsyncCommandContext,
		},
		DownloadAsset: func(ctx context.Context, assetURL, destination string, onProgress func(downloaded, total int64)) error {
			return downloadManagedRemoteAsset(ctx, assetURL, destination, false, onProgress)
		},
		Integrate: func(ctx context.Context, src string, confirm func(existing, incoming *models.UpdateSource) (bool, error)) (*models.App, error) {
			return integrateManagedUpdate(ctx, src, confirm)
		},
	}
}

func applyManagedUpdate(ctx context.Context, update pendingManagedUpdate, reporter managedApplyReporter) (*models.App, error) {
	return managedUpdateService().ApplyManagedUpdate(ctx, update, reporter)
}

func applyZsyncUpdate(ctx context.Context, update pendingManagedUpdate, destination string) error {
	return rewriteZsyncFailure(appupdate.Service{
		Zsync: zsync.Runner{
			LookPath:       zsyncLookPath,
			CommandContext: zsyncCommandContext,
		},
	}.ApplyManagedZsyncUpdate(ctx, update, destination))
}

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

func verifyDownloadedUpdate(downloadPath string, update pendingManagedUpdate) error {
	return rewriteChecksumError(appupdate.VerifyDownloadedUpdate(downloadPath, update))
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
	downloader := download.Downloader{Client: download.SharedHTTPClient()}
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
