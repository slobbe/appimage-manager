package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

type managedUpdateRunConfig struct {
	targetID   string
	apps       []*models.App
	autoApply  bool
	checkOnly  bool
	opts       runtimeOptions
	checkCache *updateCheckCacheFile
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
	apps, err := collectManagedUpdateTargets(targetID)
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

	var checkCache *updateCheckCacheFile
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

	metadataUpdates := make([]repo.CheckMetadataUpdate, 0, len(checkResults))
	collection.rows = make([]updateOutputRow, 0, len(checkResults))
	checkedAt := util.NowISO()

	for idx, result := range checkResults {
		app := result.app
		update := result.update
		err := result.err
		if err != nil {
			if !cfg.opts.DryRun {
				metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
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
			metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
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
			core.RefreshDesktopIntegrationCaches(ctx)
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

func collectManagedUpdateTargets(targetID string) ([]*models.App, error) {
	if strings.TrimSpace(targetID) != "" {
		app, err := repo.GetApp(targetID)
		if err != nil {
			return nil, wrapManagedAppLookupError(targetID, err)
		}
		return []*models.App{app}, nil
	}

	allApps, err := repo.GetAllApps()
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(allApps))
	for id := range allApps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	apps := make([]*models.App, 0, len(ids))
	for _, id := range ids {
		apps = append(apps, allApps[id])
	}

	return apps, nil
}
