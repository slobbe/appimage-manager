package cli

import (
	"context"
	"errors"
	"fmt"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	"github.com/spf13/cobra"
	"os"
	"strconv"
	"strings"
	"sync"
)

func buildManagedUpdateMessage(update appservices.ManagedUpdate, checkOnly bool) string {
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

func updateVersionTransition(update appservices.ManagedUpdate) string {
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

func runManagedUpdate(ctx context.Context, cmd *cobra.Command, targetID string) error {
	cfg, req, err := parseManagedUpdateRequest(cmd, targetID)
	if err != nil {
		return err
	}

	var (
		controller   managedApplyController
		controllerMu sync.Mutex
	)
	req.ConfirmApply = updateApplyConfirmerFunc(func(confirmation appservices.UpdateApplyConfirmation) (bool, error) {
		printPendingManagedUpdates(cmd, cfg, confirmation.Pending, false)
		return confirmManagedUpdateApply(cmd, cfg, confirmation.Pending)
	})
	req.ReporterFor = func(index, total int, update appservices.ManagedUpdate) appservices.ManagedApplyReporter {
		controllerMu.Lock()
		if controller == nil {
			if total == 1 {
				appID := ""
				if update.App != nil {
					appID = update.App.ID
				}
				controller = newSingleManagedApplyController(cmd, appID)
			} else {
				controller = newBatchManagedApplyController(cmd, total)
			}
		}
		currentController := controller
		controllerMu.Unlock()
		return appservices.ManagedApplyReporterFunc(func(event appservices.ManagedApplyEvent) {
			currentController.Event(event)
		})
	}

	result, err := runtimeServicesFrom(cmd).Update.Update(ctx, req)
	if result != nil {
		controllerMu.Lock()
		currentController := controller
		controllerMu.Unlock()
		if currentController != nil {
			currentController.Finish(managedApplyResultsFromApplyResults(result.Applied))
		} else if len(result.Applied) > 0 {
			printManagedApplySummary(cmd, managedApplyResultsFromApplyResults(result.Applied))
		}
	}
	if result != nil {
		if renderErr := renderUpdateResult(cmd, cfg, result); renderErr != nil {
			return renderErr
		}
	}
	if err != nil {
		return renderManagedUpdateError(cmd, cfg, result, err)
	}
	return nil
}

type updateApplyConfirmerFunc func(appservices.UpdateApplyConfirmation) (bool, error)

func (fn updateApplyConfirmerFunc) ConfirmUpdateApply(confirmation appservices.UpdateApplyConfirmation) (bool, error) {
	return fn(confirmation)
}

func parseManagedUpdateRequest(cmd *cobra.Command, targetID string) (managedUpdateRunConfig, appservices.UpdateRequest, error) {
	cfg, err := prepareManagedUpdateRun(cmd, targetID)
	if err != nil {
		return cfg, appservices.UpdateRequest{}, err
	}
	mode := appservices.UpdateModeManagedCheckApply
	if cfg.checkOnly {
		mode = appservices.UpdateModeCheckOnly
	}
	return cfg, appservices.UpdateRequest{
		TargetID:   cfg.targetID,
		Mode:       mode,
		DryRun:     cfg.opts.DryRun,
		AutoApply:  cfg.autoApply,
		UseCache:   !cfg.opts.DryRun && runtimePrepared(cmd),
		OnCacheHit: func(appID string) { logOperationf(cmd, "Reused cached update check for %s", appID) },
	}, nil
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

func renderUpdateResult(cmd *cobra.Command, cfg managedUpdateRunConfig, result *appservices.UpdateResult) error {
	if result == nil {
		return nil
	}
	rows := updateOutputRowsFromResult(result)
	failures := managedCheckFailuresFromViews(result.Failures)
	if cfg.targetID == "" && len(failures) > 0 {
		printManagedCheckFailures(cmd, failures)
	}
	if cfg.targetID != "" {
		if err := renderSingleTargetCheckFailure(cmd, cfg, result, rows); err != nil {
			return err
		}
	}

	switch {
	case len(result.Pending) == 0:
		return renderUpdateNoPending(cmd, cfg, result, rows)
	case result.Mode == appservices.UpdateModeCheckOnly:
		printPendingManagedUpdates(cmd, cfg, result.Pending, true)
		return renderManagedUpdateCheckOnly(cmd, cfg, rows)
	case cfg.opts.DryRun:
		printPendingManagedUpdates(cmd, cfg, result.Pending, false)
		return renderManagedUpdateDryRun(cmd, cfg, rows)
	case result.ApplySkipped:
		if handled, err := renderManagedUpdateRows(cmd, cfg.opts, rows); handled || err != nil {
			return err
		}
		printWarning(cmd, "No updates applied")
		return nil
	default:
		return renderManagedUpdateResult(cmd, cfg.opts, rows)
	}
}

func updateOutputRowsFromResult(result *appservices.UpdateResult) []updateOutputRow {
	if result == nil {
		return nil
	}
	rows := make([]updateOutputRow, 0, len(result.Rows))
	for _, row := range result.Rows {
		rows = append(rows, newUpdateOutputRow(row.App, row.Update, row.Status, row.CheckedAt))
	}
	return rows
}

func managedCheckFailuresFromViews(failures []appservices.ManagedCheckFailureView) []managedCheckFailure {
	result := make([]managedCheckFailure, 0, len(failures))
	for _, failure := range failures {
		result = append(result, managedCheckFailure{AppID: failure.AppID, Reason: rewriteBatchCheckFailure(failure.AppID, errors.New(failure.Reason))})
	}
	return result
}

func renderSingleTargetCheckFailure(cmd *cobra.Command, cfg managedUpdateRunConfig, result *appservices.UpdateResult, rows []updateOutputRow) error {
	if strings.TrimSpace(cfg.targetID) == "" || result == nil {
		return nil
	}
	for idx, row := range result.Rows {
		if row.Status != "check_failed" {
			continue
		}
		id := cfg.targetID
		if idx < len(rows) && strings.TrimSpace(rows[idx].ID) != "" {
			id = rows[idx].ID
		}
		return withUserGuidance(
			tempFailError(errors.New(row.Error)),
			fmt.Sprintf("Can't check updates for %s.", id),
			fmt.Sprintf("Rerun with 'aim update %s --debug' to see more detail.", id),
		)
	}
	return nil
}

func renderUpdateNoPending(cmd *cobra.Command, cfg managedUpdateRunConfig, result *appservices.UpdateResult, rows []updateOutputRow) error {
	if cfg.targetID != "" {
		for _, row := range rows {
			switch row.Status {
			case "no_update_source":
				printSuccess(cmd, fmt.Sprintf("No update source configured for %s", row.ID))
				return nil
			case "no_update_information":
				printSuccess(cmd, fmt.Sprintf("No update information for %s", row.ID))
				return nil
			case "up_to_date":
				printSuccess(cmd, fmt.Sprintf("Up to date: %s %s", row.ID, displayVersion(row.CurrentVersion)))
				return nil
			}
		}
	}
	if handled, err := renderManagedUpdateRows(cmd, cfg.opts, rows); handled || err != nil {
		return err
	}
	if result != nil && result.CheckFailures > 0 {
		printWarning(cmd, "No updates applied; some checks failed")
		return nil
	}
	printSuccess(cmd, "All apps are up to date")
	return nil
}

func printPendingManagedUpdates(cmd *cobra.Command, cfg managedUpdateRunConfig, pending []appservices.ManagedUpdate, checkOnly bool) {
	for _, update := range pending {
		msg := buildManagedUpdateMessage(update, checkOnly)
		if cfg.targetID == "" && update.App != nil {
			transition := strings.TrimSpace(updateVersionTransition(update))
			if transition == "" {
				transition = "unknown"
			}
			msg = fmt.Sprintf("[%s] %s", update.App.ID, transition)
		}
		printWarning(cmd, msg)
		if checkOnly {
			writeLogf(cmd, "  Download: %s\n", update.URL)
			if showManagedUpdateAsset(update.Asset) {
				writeLogf(cmd, "  Asset: %s\n", strings.TrimSpace(update.Asset))
			}
		}
	}
}

func printManagedApplySummary(cmd *cobra.Command, results []managedApplyResult) {
	failures := 0
	successes := 0
	for _, result := range results {
		if result.err != nil {
			failures++
			appID := "<unknown>"
			if result.app != nil && strings.TrimSpace(result.app.ID) != "" {
				appID = result.app.ID
			}
			printError(cmd, fmt.Sprintf("Failed: %s: %v", appID, result.err))
			continue
		}
		if result.updatedApp != nil {
			successes++
		}
	}
	summary := fmt.Sprintf("Updated %d app(s); %d failed", successes, failures)
	if failures > 0 {
		printWarning(cmd, summary)
		return
	}
	printSuccess(cmd, summary)
}

func managedApplyResultsFromApplyResults(applyResults []appservices.ManagedApplyResult) []managedApplyResult {
	results := make([]managedApplyResult, 0, len(applyResults))
	for _, apply := range applyResults {
		results = append(results, managedApplyResult{index: apply.Index, app: apply.App, updatedApp: apply.UpdatedApp, err: apply.Error})
	}
	return results
}

func renderManagedUpdateError(cmd *cobra.Command, cfg managedUpdateRunConfig, result *appservices.UpdateResult, err error) error {
	_ = cmd
	if strings.TrimSpace(cfg.targetID) != "" && appservices.IsErrorKind(err, appservices.ErrorNotFound) {
		return wrapManagedAppLookupError(cfg.targetID, err)
	}
	if result != nil && result.ApplyFailures > 0 {
		return tempFailError(err)
	}
	return wrapWriteError(err)
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

func confirmManagedUpdateApply(cmd *cobra.Command, cfg managedUpdateRunConfig, pending []appservices.ManagedUpdate) (bool, error) {
	if cfg.autoApply {
		return true, nil
	}

	prompt := formatPrompt("Apply updates to", fmt.Sprintf("%d app(s)", len(pending)))
	if cfg.targetID != "" {
		prompt = formatPrompt("Apply updates to", cfg.targetID)
	}

	return confirmAction(cmd, prompt)
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

var managedApplyRenderInterval = progressThrottleInterval

type managedApplyResult struct {
	index      int
	app        *appservices.App
	updatedApp *appservices.App
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
