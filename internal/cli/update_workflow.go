package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/slobbe/appimage-manager/internal/app/clock"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func buildManagedUpdateMessage(update pendingManagedUpdate, checkOnly bool) string {
	base := strings.TrimSpace(update.Label)
	if transition := strings.TrimSpace(updateVersionTransition(update)); transition != "" {
		if update.App != nil && strings.TrimSpace(update.App.ID) != "" {
			base = fmt.Sprintf("[%s] %s", strings.TrimSpace(update.App.ID), transition)
		} else {
			base = transition
		}
	} else if update.App != nil && strings.TrimSpace(update.App.ID) != "" {
		base = fmt.Sprintf("[%s] unknown", strings.TrimSpace(update.App.ID))
	}

	if !checkOnly {
		return base
	}

	return base
}

func updateVersionTransition(update pendingManagedUpdate) string {
	if update.App == nil {
		return ""
	}

	current := displayVersion(update.App.Version)
	latestRaw := strings.TrimSpace(update.Latest)
	if latestRaw == "" {
		return formatVersionTransition(current, "unknown")
	}

	latest := displayVersion(latestRaw)
	return formatVersionTransition(current, latest)
}

func formatVersionTransition(current, latest string) string {
	current = strings.TrimSpace(current)
	if current == "" {
		current = "unknown"
	}
	latest = strings.TrimSpace(latest)
	if latest == "" {
		latest = "unknown"
	}
	return fmt.Sprintf("%s -> %s", current, latest)
}

func showManagedUpdateAsset(asset string) bool {
	trimmed := strings.TrimSpace(asset)
	return trimmed != "" && trimmed != "update.AppImage"
}

func updateSummary(update *models.UpdateSource) string {
	if update == nil {
		return ""
	}

	switch update.Kind {
	case models.UpdateZsync:
		if update.Zsync == nil {
			return "zsync: <missing>"
		}
		if update.Zsync.UpdateInfo != "" {
			return fmt.Sprintf("zsync: %s", update.Zsync.UpdateInfo)
		}
		return "zsync"
	case models.UpdateGitHubRelease:
		if update.GitHubRelease == nil {
			return "github: <missing>"
		}
		return fmt.Sprintf("github: %s, asset: %s", update.GitHubRelease.Repo, update.GitHubRelease.Asset)
	default:
		return string(update.Kind)
	}
}

func formatAppRef(app *models.App) string {
	if app == nil {
		return "unknown"
	}
	return fmt.Sprintf("%s %s [%s]", app.Name, displayVersion(app.Version), app.ID)
}

func printManagedCheckFailures(cmd *cobra.Command, failures []managedCheckFailure) {
	if len(failures) == 0 {
		return
	}

	header := fmt.Sprintf("Failed to check updates for %d app(s)", len(failures))
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;31m", header))
	for _, failure := range failures {
		writeLogf(cmd, "  %s\n", strings.TrimSpace(failure.Reason))
	}
	writeLogf(cmd, "  Check network access, provider availability, or rerun a single app with 'aim update <id> --debug'.\n")
}

func runManagedChecks(apps []*models.App) []managedCheckResult {
	appResults := appupdate.CheckManagedUpdates(apps, runAppUpdateCheck)
	results := make([]managedCheckResult, len(appResults))
	for idx, result := range appResults {
		results[idx] = managedCheckResult{app: result.App, update: result.Update, err: result.Error}
	}
	return results
}

func runManagedChecksWithCache(cmd *cobra.Command, apps []*models.App, cache *appupdate.CheckCacheFile) ([]managedCheckResult, error) {
	results := make([]managedCheckResult, len(apps))
	if len(apps) == 0 {
		return results, nil
	}
	if cache == nil {
		return runManagedChecks(apps), nil
	}

	toCheck := make([]*models.App, 0, len(apps))
	toCheckIndices := make([]int, 0, len(apps))
	for idx, app := range apps {
		key := managedCheckCacheKey(app, idx)
		if cached, ok := cachedManagedUpdateForApp(cache, app, key); ok {
			results[idx] = managedCheckResult{app: app, update: cached}
			if app != nil {
				logOperationf(cmd, "Reused cached update check for %s", app.ID)
			}
			continue
		}
		toCheck = append(toCheck, app)
		toCheckIndices = append(toCheckIndices, idx)
	}

	fresh := runManagedChecks(toCheck)
	for idx, result := range fresh {
		results[toCheckIndices[idx]] = result
	}

	return results, nil
}

func managedCheckCacheKey(app *models.App, fallbackIdx int) string {
	return appupdate.ManagedCheckCacheKey(app, fallbackIdx)
}

func clonePendingManagedUpdateForApp(update *pendingManagedUpdate, app *models.App) *pendingManagedUpdate {
	return appupdate.CloneManagedUpdateForApp(update, app)
}

func managedCheckWorkerCount(total int) int {
	return appupdate.ManagedCheckWorkerCount(total)
}

func flushManagedCheckMetadata(updates []checkMetadataUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return updateCheckMetadataBatch(updates)
}

func updateCheckMetadata(app *models.App, checked, available bool, latest string) error {
	if app == nil {
		return nil
	}

	lastCheckedAt := clock.NowISO()

	if err := updateCheckMetadataBatch([]checkMetadataUpdate{{
		ID:            app.ID,
		Checked:       checked,
		Available:     available,
		Latest:        latest,
		LastCheckedAt: lastCheckedAt,
	}}); err != nil {
		return wrapWriteError(err)
	}

	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = lastCheckedAt

	return nil
}

func checkAppUpdate(app *models.App) (*pendingManagedUpdate, error) {
	update, err := appupdate.NewManagedUpdateChecker(appupdate.ManagedUpdateChecker{
		ZsyncCheck:         runZsyncUpdateCheck,
		GitHubReleaseCheck: runGitHubReleaseUpdateCheck,
	}).Check(app)
	if err != nil {
		return nil, softwareError(err)
	}
	return update, nil
}

var (
	downloadManagedRemoteAsset = func(ctx context.Context, assetURL, destination string, interactive bool, onProgress func(int64, int64)) error {
		return appupdate.Service{
			TempDir:        runtimeTempDir(),
			NowISO:         clock.NowISO,
			StagedDownload: stagedDownloadAdapter{client: runtimeDownloadHTTPClient},
		}.DownloadManagedUpdateAsset(ctx, assetURL, destination, onProgress)
	}
	integrateManagedUpdate = appintegrate.IntegrateFromLocalFileWithoutCacheRefreshOrPersist
)

func managedUpdateService() appupdate.Service {
	return appupdate.Service{
		TempDir:        runtimeTempDir(),
		NowISO:         clock.NowISO,
		Zsync:          runtimeZsyncRunner(),
		StagedDownload: stagedDownloadAdapter{client: runtimeDownloadHTTPClient},
		HashVerifier:   hashVerifierAdapter{},
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
		Zsync: runtimeZsyncRunner(),
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
	service := appupdate.NewService(appupdate.Service{HashVerifier: hashVerifierAdapter{}})
	return rewriteChecksumError(service.VerifyDownloadedUpdate(downloadPath, update))
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

	var (
		downloaded   int64
		total        int64
		progressMode = progressModeSpinner
	)

	logOperationContextf(ctx, "HTTP GET %s", assetURL)
	_, err := runtimeDownload(ctx, runtimeDownloadRequest{URL: assetURL, Destination: destination}, func(event runtimeDownloadProgress) {
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
	})
	if err != nil {
		var statusErr *runtimeDownloadStatusError
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

type managedUpdateRunConfig struct {
	targetID   string
	apps       []*models.App
	autoApply  bool
	checkOnly  bool
	opts       runtimeOptions
	checkCache *appupdate.CheckCacheFile
}

type managedUpdateCollection struct {
	pending             []pendingManagedUpdate
	checkFailures       int
	singleStatusPrinted bool
	failures            []managedCheckFailure
	rows                []updateOutputRow
	rowIndexByID        map[string]int
}

func runManagedUpdate(ctx context.Context, cmd *cobra.Command, targetID string) error {
	cfg, err := prepareManagedUpdateRun(cmd, targetID)
	if err != nil {
		return err
	}

	collection, err := collectManagedUpdateRows(cmd, cfg)
	if err != nil {
		return err
	}

	if cfg.targetID == "" && len(collection.failures) > 0 {
		printManagedCheckFailures(cmd, collection.failures)
	}

	if len(collection.pending) == 0 {
		return renderManagedUpdateNoPending(cmd, cfg, collection)
	}

	if cfg.checkOnly {
		return renderManagedUpdateCheckOnly(cmd, cfg, collection.rows)
	}

	if cfg.opts.DryRun {
		return renderManagedUpdateDryRun(cmd, cfg, collection.rows)
	}

	confirmed, err := confirmManagedUpdateApply(cmd, cfg, collection.pending)
	if err != nil {
		return err
	}
	if !confirmed {
		for idx := range collection.rows {
			if collection.rows[idx].Status == "update_available" {
				collection.rows[idx].Status = "apply_skipped"
			}
		}
		if handled, err := renderManagedUpdateRows(cmd, cfg.opts, collection.rows); handled || err != nil {
			return err
		}
		printWarning(cmd, "No updates applied")
		return nil
	}

	if err := applyManagedUpdates(ctx, cmd, cfg, &collection); err != nil {
		return err
	}

	return renderManagedUpdateResult(cmd, cfg.opts, collection.rows)
}

func prepareManagedUpdateRun(cmd *cobra.Command, targetID string) (managedUpdateRunConfig, error) {
	apps, err := collectManagedUpdateTargets(cmd, targetID)
	if err != nil {
		return managedUpdateRunConfig{}, err
	}

	autoApply, err := flagBool(cmd, "yes")
	if err != nil {
		return managedUpdateRunConfig{}, err
	}
	checkOnly, err := flagBool(cmd, "check-only")
	if err != nil {
		return managedUpdateRunConfig{}, err
	}

	opts := runtimeOptionsFrom(cmd)
	if !opts.DryRun {
		if err := mustEnsureRuntimeDirs(); err != nil {
			return managedUpdateRunConfig{}, err
		}
	}

	var checkCache *appupdate.CheckCacheFile
	if !opts.DryRun && runtimePrepared(cmd) {
		checkCache, err = loadUpdateCheckCache()
		if err != nil {
			return managedUpdateRunConfig{}, wrapWriteError(err)
		}
	}

	return managedUpdateRunConfig{
		targetID:   targetID,
		apps:       apps,
		autoApply:  autoApply,
		checkOnly:  checkOnly,
		opts:       opts,
		checkCache: checkCache,
	}, nil
}

func collectManagedUpdateRows(cmd *cobra.Command, cfg managedUpdateRunConfig) (managedUpdateCollection, error) {
	collection := managedUpdateCollection{
		failures:     make([]managedCheckFailure, 0),
		rowIndexByID: map[string]int{},
	}

	checkResults, err := runManagedChecksWithCache(cmd, cfg.apps, cfg.checkCache)
	if err != nil {
		return collection, wrapWriteError(err)
	}

	metadataUpdates := make([]checkMetadataUpdate, 0, len(checkResults))
	collection.rows = make([]updateOutputRow, 0, len(checkResults))
	checkedAt := clock.NowISO()

	for idx, result := range checkResults {
		app := result.app
		update := result.update
		err := result.err
		if err != nil {
			if !cfg.opts.DryRun {
				metadataUpdates = append(metadataUpdates, checkMetadataUpdate{
					ID:            app.ID,
					Checked:       false,
					Available:     app.UpdateAvailable,
					Latest:        app.LatestVersion,
					LastCheckedAt: checkedAt,
				})
			}

			if cfg.targetID != "" && !cfg.opts.DryRun {
				if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
					return collection, wrapWriteError(metaErr)
				}
				metadataUpdates = metadataUpdates[:0]
			}
			collection.rows = append(collection.rows, newUpdateOutputRow(app, update, "check_failed", checkedAt))
			if app != nil {
				collection.rowIndexByID[app.ID] = len(collection.rows) - 1
			}
			if cfg.targetID != "" {
				return collection, withUserGuidance(
					tempFailError(err),
					fmt.Sprintf("Can't check updates for %s.", app.ID),
					fmt.Sprintf("Rerun with 'aim update %s --debug' to see more detail.", app.ID),
				)
			}

			collection.checkFailures++
			collection.failures = append(collection.failures, managedCheckFailure{
				AppID:  app.ID,
				Reason: rewriteBatchCheckFailure(app.ID, err),
			})
			continue
		}

		if !cfg.opts.DryRun && app != nil {
			setCachedManagedUpdate(cfg.checkCache, app, managedCheckCacheKey(app, idx), update)
		}

		if update == nil {
			status := "no_update_information"
			if app.Update == nil || app.Update.Kind == models.UpdateNone {
				status = "no_update_source"
			}
			collection.rows = append(collection.rows, newUpdateOutputRow(app, nil, status, checkedAt))
			if app != nil {
				collection.rowIndexByID[app.ID] = len(collection.rows) - 1
			}
			if cfg.targetID != "" && !shouldUseStructuredOutput(cmd) {
				if status == "no_update_source" {
					printSuccess(cmd, fmt.Sprintf("No update source configured for %s", app.ID))
				} else {
					printSuccess(cmd, fmt.Sprintf("No update information for %s", app.ID))
				}
				collection.singleStatusPrinted = true
			}
			continue
		}

		if !cfg.opts.DryRun {
			metadataUpdates = append(metadataUpdates, checkMetadataUpdate{
				ID:            app.ID,
				Checked:       true,
				Available:     update.Available,
				Latest:        update.Latest,
				LastCheckedAt: checkedAt,
			})
		}

		if cfg.targetID != "" && !cfg.opts.DryRun {
			if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
				return collection, wrapWriteError(metaErr)
			}
			metadataUpdates = metadataUpdates[:0]
		}

		if update.URL == "" {
			collection.rows = append(collection.rows, newUpdateOutputRow(app, update, "up_to_date", checkedAt))
			if app != nil {
				collection.rowIndexByID[app.ID] = len(collection.rows) - 1
			}
			if cfg.targetID != "" && !shouldUseStructuredOutput(cmd) {
				printSuccess(cmd, fmt.Sprintf("Up to date: %s %s", app.ID, displayVersion(app.Version)))
				collection.singleStatusPrinted = true
			}
			continue
		}

		collection.rows = append(collection.rows, newUpdateOutputRow(app, update, "update_available", checkedAt))
		if app != nil {
			collection.rowIndexByID[app.ID] = len(collection.rows) - 1
		}

		msg := buildManagedUpdateMessage(*update, cfg.checkOnly)
		if cfg.targetID == "" {
			transition := strings.TrimSpace(updateVersionTransition(*update))
			if transition == "" {
				transition = "unknown"
			}
			msg = fmt.Sprintf("[%s] %s", app.ID, transition)
		}
		printWarning(cmd, msg)
		if cfg.checkOnly {
			writeLogf(cmd, "  Download: %s\n", update.URL)
			if showManagedUpdateAsset(update.Asset) {
				writeLogf(cmd, "  Asset: %s\n", strings.TrimSpace(update.Asset))
			}
		}

		collection.pending = append(collection.pending, *update)
	}

	if !cfg.opts.DryRun {
		err = flushManagedCheckMetadata(metadataUpdates)
	}
	if err != nil {
		if cfg.targetID != "" {
			return collection, wrapWriteError(err)
		}
		collection.checkFailures++
		printError(cmd, userMessageForError(wrapWriteError(err)).Summary)
	}
	if !cfg.opts.DryRun && cfg.checkCache != nil {
		if cacheErr := saveUpdateCheckCache(cfg.checkCache); cacheErr != nil {
			return collection, wrapWriteError(cacheErr)
		}
	}

	return collection, nil
}

func renderManagedUpdateNoPending(cmd *cobra.Command, cfg managedUpdateRunConfig, collection managedUpdateCollection) error {
	if cfg.targetID != "" && collection.singleStatusPrinted {
		return nil
	}

	if handled, err := renderManagedUpdateRows(cmd, cfg.opts, collection.rows); handled || err != nil {
		return err
	}

	if collection.checkFailures > 0 {
		printWarning(cmd, "No updates applied; some checks failed")
		return nil
	}
	printSuccess(cmd, "All apps are up to date")
	return nil
}

func renderManagedUpdateCheckOnly(cmd *cobra.Command, cfg managedUpdateRunConfig, rows []updateOutputRow) error {
	if handled, err := renderManagedUpdateRows(cmd, cfg.opts, rows); handled || err != nil {
		return err
	}
	return nil
}

func renderManagedUpdateDryRun(cmd *cobra.Command, cfg managedUpdateRunConfig, rows []updateOutputRow) error {
	for idx := range rows {
		if rows[idx].Status == "update_available" {
			rows[idx].Status = "dry_run_pending"
		}
	}

	if handled, err := renderManagedUpdateRows(cmd, cfg.opts, rows); handled || err != nil {
		return err
	}
	printInfo(cmd, "Dry run: no updates were applied")
	return nil
}

func confirmManagedUpdateApply(cmd *cobra.Command, cfg managedUpdateRunConfig, pending []pendingManagedUpdate) (bool, error) {
	if cfg.autoApply {
		return true, nil
	}

	prompt := formatPrompt("Apply updates to", fmt.Sprintf("%d app(s)", len(pending)))
	if cfg.targetID != "" {
		prompt = formatPrompt("Apply updates to", cfg.targetID)
	}

	return confirmAction(cmd, prompt)
}

func applyManagedUpdates(ctx context.Context, cmd *cobra.Command, cfg managedUpdateRunConfig, collection *managedUpdateCollection) error {
	var (
		applyResults  []managedApplyResult
		applyFailures int
		appliedApps   []*models.App
		persistErr    error
	)

	err := withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Applying %d managed update(s)", len(collection.pending))
		applyResults = runManagedApplies(ctx, cmd, collection.pending)
		applyFailures = 0
		appliedApps = make([]*models.App, 0, len(applyResults))
		for _, result := range applyResults {
			if result.err != nil {
				applyFailures++
				continue
			}
			if result.updatedApp != nil {
				appliedApps = append(appliedApps, result.updatedApp)
				if idx, ok := collection.rowIndexByID[result.updatedApp.ID]; ok {
					collection.rows[idx].Status = "updated"
					collection.rows[idx].CurrentVersion = result.updatedApp.Version
				}
			}
		}

		if len(appliedApps) > 0 {
			appintegrate.RefreshDesktopIntegrationCaches(ctx)
		}

		logOperationf(cmd, "Persisting applied updates")
		persistErr = persistManagedAppliedApps(ctx, appliedApps)
		return nil
	})
	if err != nil {
		return err
	}
	if len(appliedApps) > 0 && cfg.checkCache != nil {
		invalidateCachedManagedUpdates(cfg.checkCache, appliedAppIDs(appliedApps)...)
		if cacheErr := saveUpdateCheckCache(cfg.checkCache); cacheErr != nil {
			return wrapWriteError(cacheErr)
		}
	}

	if applyFailures > 0 {
		if persistErr != nil {
			return wrapWriteError(fmt.Errorf("%d update(s) failed; failed to persist applied updates: %w", applyFailures, persistErr))
		}
		return tempFailError(fmt.Errorf("%d update(s) failed", applyFailures))
	}

	if persistErr != nil {
		return wrapWriteError(persistErr)
	}

	return nil
}

func renderManagedUpdateResult(cmd *cobra.Command, opts runtimeOptions, rows []updateOutputRow) error {
	if handled, err := renderManagedUpdateRows(cmd, opts, rows); handled || err != nil {
		return err
	}
	return nil
}

func renderManagedUpdateRows(cmd *cobra.Command, opts runtimeOptions, rows []updateOutputRow) (bool, error) {
	if opts.JSON {
		return true, printJSONSuccess(cmd, rows)
	}
	if opts.CSV {
		csvRows := make([][]string, 0, len(rows))
		for _, row := range rows {
			csvRows = append(csvRows, row.csvRow())
		}
		return true, writeCSV(cmd, updateCSVHeader(), csvRows)
	}
	if opts.Plain {
		writePlainUpdateRows(cmd, rows)
		return true, nil
	}
	return false, nil
}

func collectManagedUpdateTargets(cmd *cobra.Command, targetID string) ([]*models.App, error) {
	if strings.TrimSpace(targetID) != "" {
		info, err := runtimeServicesFrom(cmd).Info.ManagedAppInfo(cmd.Context(), targetID)
		if err != nil {
			return nil, wrapManagedAppLookupError(targetID, err)
		}
		return []*models.App{info.App}, nil
	}

	result, err := runtimeServicesFrom(cmd).List.List(cmd.Context(), appservices.ListRequest{IncludeIntegrated: true, IncludeUnlinked: true})
	if err != nil {
		return nil, err
	}

	apps := append([]*models.App(nil), result.ManagedApps...)
	sort.SliceStable(apps, func(i, j int) bool {
		if apps[i] == nil {
			return false
		}
		if apps[j] == nil {
			return true
		}
		return strings.TrimSpace(apps[i].ID) < strings.TrimSpace(apps[j].ID)
	})

	return apps, nil
}

func stagedDownloadDir() string {
	return filepath.Join(runtimeTempDir(), "downloads")
}

func updateCheckCacheFilePath() string {
	return filepath.Join(runtimeTempDir(), "update-check-cache.json")
}

func stableDownloadDestination(assetURL, nameHint string) (string, error) {
	destination, err := runtimeStableDownloadDestination(stagedDownloadDir(), assetURL, nameHint)
	if err != nil {
		return "", wrapWriteError(err)
	}
	return destination, nil
}

func removeStagedDownload(downloadPath string) {
	runtimeRemoveStagedDownload(downloadPath)
}

func loadUpdateCheckCache() (*appupdate.CheckCacheFile, error) {
	path := updateCheckCacheFilePath()
	data, ok, err := runtimeReadFileIfExists(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return appupdate.NewCheckCacheFile(), nil
	}

	var cache appupdate.CheckCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return appupdate.NormalizeCheckCache(&cache), nil
}

func saveUpdateCheckCache(cache *appupdate.CheckCacheFile) error {
	if cache == nil {
		return nil
	}
	if err := runtimeEnsureDir(runtimeTempDir()); err != nil {
		return err
	}
	appupdate.NormalizeCheckCache(cache)

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return runtimeWriteAtomicFile(updateCheckCacheFilePath(), data, 0o644)
}

func cachedManagedUpdateForApp(cache *appupdate.CheckCacheFile, app *models.App, sourceKey string) (*pendingManagedUpdate, bool) {
	return appupdate.CachedManagedUpdateForApp(cache, app, sourceKey, time.Now(), appupdate.DefaultCheckCacheTTL)
}

func setCachedManagedUpdate(cache *appupdate.CheckCacheFile, app *models.App, sourceKey string, update *pendingManagedUpdate) {
	appupdate.SetCachedManagedUpdate(cache, app, sourceKey, update, clock.NowISO())
}

func invalidateCachedManagedUpdates(cache *appupdate.CheckCacheFile, appIDs ...string) {
	appupdate.InvalidateCachedManagedUpdates(cache, appIDs...)
}

func appliedAppIDs(apps []*models.App) []string {
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		if app == nil || strings.TrimSpace(app.ID) == "" {
			continue
		}
		ids = append(ids, strings.TrimSpace(app.ID))
	}
	return ids
}

var managedApplyRenderInterval = progressThrottleInterval

type managedApplyStage = appupdate.ManagedApplyStage

const (
	managedApplyStageQueued    managedApplyStage = appupdate.ManagedApplyStageQueued
	managedApplyStageZsync     managedApplyStage = appupdate.ManagedApplyStageZsync
	managedApplyStageDownload  managedApplyStage = appupdate.ManagedApplyStageDownload
	managedApplyStageVerify    managedApplyStage = appupdate.ManagedApplyStageVerify
	managedApplyStageIntegrate managedApplyStage = appupdate.ManagedApplyStageIntegrate
	managedApplyStageDone      managedApplyStage = appupdate.ManagedApplyStageDone
	managedApplyStageFailed    managedApplyStage = appupdate.ManagedApplyStageFailed
)

type managedApplyEvent = appupdate.ManagedApplyEvent

type managedApplyReporter = appupdate.ManagedApplyReporter

type managedApplyReporterFunc = appupdate.ManagedApplyReporterFunc

type managedApplyResult struct {
	index      int
	app        *models.App
	updatedApp *models.App
	err        error
}

type managedApplyController interface {
	Event(managedApplyEvent)
	Finish([]managedApplyResult)
}

type batchManagedApplyController struct {
	cmd       *cobra.Command
	total     int
	progress  progressHandle
	mu        sync.Mutex
	completed []bool
	failures  int
}

type singleManagedApplyController struct {
	cmd           *cobra.Command
	appID         string
	handle        progressHandle
	handleMode    progressMode
	downloaded    int64
	downloadTotal int64
	downloadName  string
}

func runManagedApplies(ctx context.Context, cmd *cobra.Command, pending []pendingManagedUpdate) []managedApplyResult {
	if len(pending) == 0 {
		return nil
	}

	controller := newManagedApplyController(cmd, pending)
	batch, err := runtimeServicesFrom(cmd).Update.ApplyBatch(ctx, appservices.UpdateApplyBatchRequest{
		Pending: pending,
		ReporterFor: func(index, total int, update pendingManagedUpdate) appupdate.ManagedApplyReporter {
			return appupdate.WithManagedApplyEventDefaults(managedApplyReporterFunc(func(event managedApplyEvent) {
				controller.Event(event)
			}), index, total, update)
		},
	})
	appResults := []appupdate.ManagedApplyResult(nil)
	if batch != nil {
		appResults = batch.Results
	}
	if err != nil {
		appResults = make([]appupdate.ManagedApplyResult, len(pending))
		for idx, update := range pending {
			appResults[idx] = appupdate.ManagedApplyResult{Index: idx, App: update.App, Error: err}
		}
	}
	results := make([]managedApplyResult, len(appResults))
	for idx, result := range appResults {
		results[idx] = managedApplyResult{index: result.Index, app: result.App, updatedApp: result.UpdatedApp, err: result.Error}
	}
	controller.Finish(results)
	return results
}

func managedApplyWorkerCount(total int) int {
	return appupdate.ManagedApplyWorkerCount(total)
}

func emitManagedApplyEvent(reporter managedApplyReporter, event managedApplyEvent) {
	if reporter != nil {
		reporter.Event(event)
	}
}

func newManagedApplyController(cmd *cobra.Command, pending []pendingManagedUpdate) managedApplyController {
	if len(pending) == 1 {
		appID := ""
		if pending[0].App != nil {
			appID = strings.TrimSpace(pending[0].App.ID)
		}
		return newSingleManagedApplyController(cmd, appID)
	}
	return newBatchManagedApplyController(cmd, len(pending))
}

func newBatchManagedApplyController(cmd *cobra.Command, total int) *batchManagedApplyController {
	return &batchManagedApplyController{
		cmd:       cmd,
		total:     total,
		progress:  newCountProgress(cmd, managedApplyAggregateDescription(total, 0, 0), int64(total)),
		completed: make([]bool, total),
	}
}

func (c *batchManagedApplyController) Event(event managedApplyEvent) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if event.Index < 0 || event.Index >= len(c.completed) {
		return
	}

	if event.Stage != managedApplyStageDone && event.Stage != managedApplyStageFailed {
		return
	}
	if c.completed[event.Index] {
		return
	}

	c.completed[event.Index] = true
	if event.Stage == managedApplyStageFailed {
		c.failures++
	}
	if c.progress != nil {
		c.progress.Add(1)
		c.progress.Describe(managedApplyAggregateDescription(c.total, completedCount(c.completed), c.failures))
	}
}

func (c *batchManagedApplyController) Finish(results []managedApplyResult) {
	if c == nil {
		return
	}
	if c.progress != nil {
		c.progress.Finish()
	}

	failures := 0
	successes := 0
	for _, result := range results {
		if result.err != nil {
			failures++
			appID := "<unknown>"
			if result.app != nil && strings.TrimSpace(result.app.ID) != "" {
				appID = result.app.ID
			}
			printError(c.cmd, fmt.Sprintf("Failed: %s: %v", appID, result.err))
			continue
		}
		if result.updatedApp != nil {
			successes++
		}
	}

	summary := fmt.Sprintf("Updated %d app(s); %d failed", successes, failures)
	if failures > 0 {
		printWarning(c.cmd, summary)
		return
	}
	printSuccess(c.cmd, summary)
}

func newSingleManagedApplyController(cmd *cobra.Command, appID string) *singleManagedApplyController {
	if strings.TrimSpace(appID) == "" {
		appID = "app"
	}
	return &singleManagedApplyController{
		cmd:        cmd,
		appID:      appID,
		handle:     newSpinnerProgress(cmd, fmt.Sprintf("Updating %s", appID)),
		handleMode: progressModeSpinner,
	}
}

func (c *singleManagedApplyController) Event(event managedApplyEvent) {
	if c == nil {
		return
	}
	if strings.TrimSpace(event.AppID) != "" {
		c.appID = strings.TrimSpace(event.AppID)
	}
	if strings.TrimSpace(event.DownloadName) != "" {
		c.downloadName = strings.TrimSpace(event.DownloadName)
	}

	switch event.Stage {
	case managedApplyStageDownload:
		c.updateDownloadProgress(event.Downloaded, event.DownloadTotal)
	case managedApplyStageZsync:
		c.setSpinnerDescription(fmt.Sprintf("Applying delta update to %s", c.appID))
	case managedApplyStageVerify:
		c.setSpinnerDescription(fmt.Sprintf("Verifying %s", c.appID))
	case managedApplyStageIntegrate:
		c.setSpinnerDescription(fmt.Sprintf("Integrating %s", c.appID))
	case managedApplyStageQueued:
		c.setSpinnerDescription(fmt.Sprintf("Preparing %s", c.appID))
	case managedApplyStageDone, managedApplyStageFailed:
		// Final state is rendered after the worker result is collected.
	}
}

func (c *singleManagedApplyController) Finish(results []managedApplyResult) {
	if c == nil {
		return
	}
	if c.handle != nil {
		c.handle.Clear()
		c.handle = nil
	}

	failures := 0
	successes := 0
	for _, result := range results {
		if result.err != nil {
			failures++
			appID := c.appID
			if result.app != nil && strings.TrimSpace(result.app.ID) != "" {
				appID = result.app.ID
			}
			printError(c.cmd, fmt.Sprintf("Failed: %s: %v", appID, result.err))
			continue
		}
		if result.updatedApp != nil {
			successes++
		}
	}

	summary := fmt.Sprintf("Updated %d app(s); %d failed", successes, failures)
	if failures > 0 {
		printWarning(c.cmd, summary)
		return
	}
	printSuccess(c.cmd, summary)
}

func (c *singleManagedApplyController) setSpinnerDescription(description string) {
	description = strings.TrimSpace(description)
	if description == "" {
		return
	}
	if c.handle == nil || c.handleMode != progressModeSpinner {
		if c.handle != nil {
			c.handle.Clear()
		}
		c.handle = newSpinnerProgress(c.cmd, description)
		c.handleMode = progressModeSpinner
		c.downloaded = 0
		c.downloadTotal = 0
		return
	}
	c.handle.Describe(description)
}

func (c *singleManagedApplyController) updateDownloadProgress(downloaded, total int64) {
	downloadName := c.appID
	if strings.TrimSpace(c.downloadName) != "" {
		downloadName = strings.TrimSpace(c.downloadName)
	}
	description := fmt.Sprintf("Downloading %s", downloadName)
	if downloaded <= 0 && total == 0 {
		c.setSpinnerDescription(description)
		return
	}
	if c.handle == nil || c.handleMode != progressModeBytes || c.downloadTotal != total {
		if c.handle != nil {
			c.handle.Clear()
		}
		c.handle = newByteProgress(c.cmd, description, total)
		c.handleMode = progressModeBytes
		c.downloaded = 0
		c.downloadTotal = total
	}

	c.handle.Describe(description)
	delta := downloaded - c.downloaded
	if delta > 0 {
		c.handle.Add(delta)
		c.downloaded = downloaded
	}
}

func completedCount(completed []bool) int {
	total := 0
	for _, value := range completed {
		if value {
			total++
		}
	}
	return total
}

func activeCount(total, completed int) int {
	active := total - completed
	if active < 0 {
		return 0
	}
	return active
}

func managedApplyAggregateDescription(total, completed, failures int) string {
	active := activeCount(total, completed)
	return fmt.Sprintf("Updating apps (%d/%d complete, %d failed, %d active)", completed, total, failures, active)
}
