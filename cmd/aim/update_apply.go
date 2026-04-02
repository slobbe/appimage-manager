package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func persistManagedAppliedApps(apps []*models.App) error {
	if len(apps) == 0 {
		return nil
	}

	if err := addAppsBatch(apps, true); err == nil {
		return nil
	}

	fallbackErrors := make([]string, 0)
	for _, app := range apps {
		if app == nil {
			continue
		}
		if err := addSingleApp(app, true); err != nil {
			fallbackErrors = append(fallbackErrors, fmt.Sprintf("%s: %v", app.ID, err))
		}
	}

	if len(fallbackErrors) > 0 {
		return wrapWriteError(fmt.Errorf("failed to persist applied updates: %s", strings.Join(fallbackErrors, "; ")))
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
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDownload})
		logOperationContextf(ctx, "Downloading update for %s", update.App.ID)
		if err := downloadManagedRemoteAsset(ctx, update.URL, downloadPath, false, func(downloaded, total int64) {
			emitManagedApplyEvent(reporter, managedApplyEvent{
				Stage:         managedApplyStageDownload,
				Downloaded:    downloaded,
				DownloadTotal: total,
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
			fmt.Sprintf("Run 'aim migrate %s' or reinstall the app.", update.App.ID),
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

func downloadUpdateAsset(ctx context.Context, assetURL, destination string, interactive bool) error {
	return downloadUpdateAssetWithProgress(ctx, assetURL, destination, interactive, nil)
}

func downloadUpdateAssetWithProgress(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(downloaded, total int64)) error {
	progress := newProcessSpinnerProgress("Downloading update", interactive)
	defer func() {
		if progress != nil && interactive {
			progress.Clear()
		}
	}()

	meta, err := loadStagedDownloadMetadata(destination)
	if err != nil {
		return wrapWriteError(err)
	}

	var existingSize int64
	if info, statErr := os.Stat(destination); statErr == nil {
		existingSize = info.Size()
	} else if !os.IsNotExist(statErr) {
		return wrapWriteError(statErr)
	}
	if meta != nil && strings.TrimSpace(meta.URL) != "" && strings.TrimSpace(meta.URL) != strings.TrimSpace(assetURL) {
		removeStagedDownload(destination)
		meta = nil
		existingSize = 0
	}

	doRequest := func(rangeStart int64) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
		if err != nil {
			return nil, err
		}
		if rangeStart > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeStart))
		}
		logOperationContextf(ctx, "HTTP GET %s", assetURL)
		return core.SharedHTTPClient().Do(req)
	}

	resp, err := doRequest(existingSize)
	if err != nil {
		return rewriteNetworkDownloadError(err)
	}
	if existingSize > 0 && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		removeStagedDownload(destination)
		existingSize = 0
		meta = nil
		resp, err = doRequest(0)
		if err != nil {
			return rewriteNetworkDownloadError(err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return withUserGuidance(
			unavailableError(fmt.Errorf("download failed with status %s", resp.Status)),
			fmt.Sprintf("Can't download update: server returned %s.", resp.Status),
			"Try again later or check whether the upstream release is available.",
		)
	}

	openFlags := os.O_CREATE | os.O_WRONLY
	if existingSize > 0 && resp.StatusCode == http.StatusPartialContent {
		openFlags |= os.O_APPEND
	} else {
		openFlags |= os.O_TRUNC
		existingSize = 0
	}

	f, err := os.OpenFile(destination, openFlags, 0o644)
	if err != nil {
		return wrapWriteError(err)
	}
	defer f.Close()

	total := resp.ContentLength
	if existingSize > 0 && resp.ContentLength > 0 {
		total = existingSize + resp.ContentLength
	}
	if meta == nil {
		meta = &stagedDownloadMetadata{URL: assetURL}
	}
	meta.URL = assetURL
	meta.ETag = strings.TrimSpace(resp.Header.Get("ETag"))
	meta.LastModified = strings.TrimSpace(resp.Header.Get("Last-Modified"))
	meta.TotalBytes = total
	if err := saveStagedDownloadMetadata(destination, *meta); err != nil {
		return wrapWriteError(err)
	}

	var (
		downloaded   = existingSize
		buffer       = make([]byte, 32*1024)
		progressMode = progressModeSpinner
	)
	if interactive && progress != nil {
		progress.Clear()
		progress = newProcessByteProgress("Downloading update", total, true)
		if total > 0 {
			progressMode = progressModeBytes
		}
		if existingSize > 0 {
			progress.Add(existingSize)
		}
	}

	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := f.Write(buffer[:n]); err != nil {
				return wrapWriteError(err)
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
			if interactive && progress != nil {
				progress.Add(int64(n))
			}
			meta.TotalBytes = total
			if err := saveStagedDownloadMetadata(destination, *meta); err != nil {
				return wrapWriteError(err)
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return rewriteNetworkDownloadError(readErr)
		}
	}

	if interactive {
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
		if onProgress == nil {
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
