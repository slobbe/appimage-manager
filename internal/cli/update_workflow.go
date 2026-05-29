package cli

import (
	"context"
	"errors"
	"fmt"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

func buildManagedUpdateMessage(update appservices.ManagedUpdateView, checkOnly bool) string {
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

func updateVersionTransition(update appservices.ManagedUpdateView) string {
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

func formatAppDetailsRef(app *appservices.AppDetails) string {
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
	targetID  string
	autoApply bool
	checkOnly bool
	opts      runtimeOptions
}

type managedUpdateCollection struct {
	pending             []appservices.ManagedUpdateHandle
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

	return managedUpdateRunConfig{
		targetID:  targetID,
		autoApply: autoApply,
		checkOnly: checkOnly,
		opts:      opts,
	}, nil
}

func collectManagedUpdateRows(cmd *cobra.Command, cfg managedUpdateRunConfig) (managedUpdateCollection, error) {
	collection := managedUpdateCollection{
		failures:     make([]managedCheckFailure, 0),
		rowIndexByID: map[string]int{},
	}

	result, err := runtimeServicesFrom(cmd).Update.CheckManagedUpdates(cmd.Context(), appservices.ManagedUpdateCheckRequest{
		TargetID: cfg.targetID,
		DryRun:   cfg.opts.DryRun,
		UseCache: !cfg.opts.DryRun && runtimePrepared(cmd),
		OnCacheHit: func(appID string) {
			logOperationf(cmd, "Reused cached update check for %s", appID)
		},
	})
	if err != nil {
		if strings.TrimSpace(cfg.targetID) != "" && appservices.IsErrorKind(err, appservices.ErrorNotFound) {
			return collection, wrapManagedAppLookupError(cfg.targetID, err)
		}
		return collection, wrapWriteError(err)
	}
	if result == nil {
		return collection, nil
	}

	collection.pending = append(collection.pending, result.PendingHandles...)
	collection.checkFailures = result.CheckFailures
	collection.rows = make([]updateOutputRow, 0, len(result.Rows))

	for _, row := range result.Rows {
		outputRow := newUpdateOutputRow(row.App, row.Update, row.Status, row.CheckedAt)
		collection.rows = append(collection.rows, outputRow)
		if strings.TrimSpace(outputRow.ID) != "" {
			collection.rowIndexByID[outputRow.ID] = len(collection.rows) - 1
		}

		if row.Status == "check_failed" {
			if cfg.targetID != "" {
				return collection, withUserGuidance(
					tempFailError(errors.New(row.Error)),
					fmt.Sprintf("Can't check updates for %s.", outputRow.ID),
					fmt.Sprintf("Rerun with 'aim update %s --debug' to see more detail.", outputRow.ID),
				)
			}
			collection.failures = append(collection.failures, managedCheckFailure{
				AppID:  outputRow.ID,
				Reason: rewriteBatchCheckFailure(outputRow.ID, errors.New(row.Error)),
			})
			continue
		}

		if cfg.targetID != "" && !shouldUseStructuredOutput(cmd) {
			switch row.Status {
			case "no_update_source":
				printSuccess(cmd, fmt.Sprintf("No update source configured for %s", outputRow.ID))
				collection.singleStatusPrinted = true
			case "no_update_information":
				printSuccess(cmd, fmt.Sprintf("No update information for %s", outputRow.ID))
				collection.singleStatusPrinted = true
			case "up_to_date":
				printSuccess(cmd, fmt.Sprintf("Up to date: %s %s", outputRow.ID, displayVersion(outputRow.CurrentVersion)))
				collection.singleStatusPrinted = true
			}
		}

		if row.Status != "update_available" || row.Update == nil {
			continue
		}
		msg := buildManagedUpdateMessage(*row.Update, cfg.checkOnly)
		if cfg.targetID == "" {
			transition := strings.TrimSpace(updateVersionTransition(*row.Update))
			if transition == "" {
				transition = "unknown"
			}
			msg = fmt.Sprintf("[%s] %s", outputRow.ID, transition)
		}
		printWarning(cmd, msg)
		if cfg.checkOnly {
			writeLogf(cmd, "  Download: %s\n", row.Update.URL)
			if showManagedUpdateAsset(row.Update.Asset) {
				writeLogf(cmd, "  Asset: %s\n", strings.TrimSpace(row.Update.Asset))
			}
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

func confirmManagedUpdateApply(cmd *cobra.Command, cfg managedUpdateRunConfig, pending []appservices.ManagedUpdateHandle) (bool, error) {
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
		appliedIDs    []string
		persistErr    error
	)

	err := withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Applying %d managed update(s)", len(collection.pending))
		var applyErr error
		applyResults, applyErr = runManagedApplies(ctx, cmd, collection.pending)
		persistErr = applyErr
		applyFailures = 0
		appliedIDs = make([]string, 0, len(applyResults))
		for _, result := range applyResults {
			if result.err != nil {
				applyFailures++
				continue
			}
			if result.updatedApp != nil {
				appliedIDs = append(appliedIDs, result.updatedApp.ID)
				if idx, ok := collection.rowIndexByID[result.updatedApp.ID]; ok {
					collection.rows[idx].Status = "updated"
					collection.rows[idx].CurrentVersion = result.updatedApp.Version
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	_ = appliedIDs

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

var managedApplyRenderInterval = progressThrottleInterval

type managedApplyResult struct {
	index      int
	app        *appservices.AppSummary
	updatedApp *appservices.AppDetails
	err        error
}

type managedApplyController interface {
	Event(appservices.ManagedApplyEvent)
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

func runManagedApplies(ctx context.Context, cmd *cobra.Command, pending []appservices.ManagedUpdateHandle) ([]managedApplyResult, error) {
	if len(pending) == 0 {
		return nil, nil
	}

	controller := newManagedApplyController(cmd, pending)
	batch, err := runtimeServicesFrom(cmd).Update.ApplyBatch(ctx, appservices.UpdateApplyBatchRequest{
		Pending: pending,
		ReporterFor: func(index, total int, update appservices.ManagedUpdateView) appservices.ManagedApplyReporter {
			return appservices.ManagedApplyReporterFunc(func(event appservices.ManagedApplyEvent) {
				controller.Event(event)
			})
		},
	})
	views := []appservices.ManagedApplyResultView(nil)
	if batch != nil {
		views = batch.Results
	}
	if batch == nil && err != nil {
		views = make([]appservices.ManagedApplyResultView, len(pending))
		for idx, update := range pending {
			views[idx] = appservices.ManagedApplyResultView{Index: idx, App: update.View.App, Error: err.Error()}
		}
	}
	results := make([]managedApplyResult, len(views))
	for idx, result := range views {
		results[idx] = managedApplyResult{index: result.Index, app: result.App, updatedApp: result.UpdatedApp}
		if strings.TrimSpace(result.Error) != "" {
			results[idx].err = errors.New(result.Error)
		}
	}
	controller.Finish(results)
	return results, err
}

func newManagedApplyController(cmd *cobra.Command, pending []appservices.ManagedUpdateHandle) managedApplyController {
	if len(pending) == 1 {
		appID := ""
		if pending[0].View.App != nil {
			appID = strings.TrimSpace(pending[0].View.App.ID)
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

func (c *batchManagedApplyController) Event(event appservices.ManagedApplyEvent) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if event.Index < 0 || event.Index >= len(c.completed) {
		return
	}

	if event.Stage != appservices.ManagedApplyStageDone && event.Stage != appservices.ManagedApplyStageFailed {
		return
	}
	if c.completed[event.Index] {
		return
	}

	c.completed[event.Index] = true
	if event.Stage == appservices.ManagedApplyStageFailed {
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

func (c *singleManagedApplyController) Event(event appservices.ManagedApplyEvent) {
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
	case appservices.ManagedApplyStageDownload:
		c.updateDownloadProgress(event.Downloaded, event.DownloadTotal)
	case appservices.ManagedApplyStageZsync:
		c.setSpinnerDescription(fmt.Sprintf("Applying delta update to %s", c.appID))
	case appservices.ManagedApplyStageVerify:
		c.setSpinnerDescription(fmt.Sprintf("Verifying %s", c.appID))
	case appservices.ManagedApplyStageIntegrate:
		c.setSpinnerDescription(fmt.Sprintf("Integrating %s", c.appID))
	case appservices.ManagedApplyStageQueued:
		c.setSpinnerDescription(fmt.Sprintf("Preparing %s", c.appID))
	case appservices.ManagedApplyStageDone, appservices.ManagedApplyStageFailed:
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
