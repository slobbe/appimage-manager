package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/slobbe/appimage-manager/internal/core"
	"github.com/slobbe/appimage-manager/internal/discovery"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

func RootCmd(cmd *cobra.Command, args []string) error {
	upgrade, err := cmd.Flags().GetBool("upgrade")
	if err != nil {
		return usageError(err)
	}
	if upgrade {
		if len(args) > 0 {
			return usageError(fmt.Errorf("--upgrade does not accept positional arguments"))
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		return runUpgrade(cmd.Context(), cmd)
	}

	writeDataf(cmd, "%s", renderConciseHelp(cmd))
	return nil
}

func runUpgrade(ctx context.Context, cmd *cobra.Command) error {
	if ctx == nil {
		ctx = context.Background()
	}

	logOperationf(cmd, "Checking for aim updates")
	checkResult, err := runWithBusyIndicator(cmd, progressCheckAimUpdates(), func() (*core.AimUpgradeCheckResult, error) {
		return checkAimUpgrade(ctx, version)
	})
	if err != nil {
		return err
	}
	if checkResult != nil && checkResult.Comparable && !checkResult.HasUpdate {
		current := checkResult.LatestVersion
		if strings.TrimSpace(current) == "" {
			current = checkResult.CurrentVersion
		}
		printSuccess(cmd, fmt.Sprintf("aim is up to date (%s)", displayVersion(current)))
		return nil
	}

	logOperationf(cmd, "Downloading and running the aim installer")
	result, err := runWithBusyIndicator(cmd, progressUpgradeAim(), func() (*core.InstallerUpgradeResult, error) {
		return runUpgradeViaInstaller(ctx, version)
	})
	if err != nil {
		return err
	}
	if result != nil && strings.TrimSpace(result.InstalledVersion) != "" {
		printSuccess(cmd, fmt.Sprintf(
			"Upgraded aim %s -> %s",
			displayVersion(result.PreviousVersion),
			displayVersion(result.InstalledVersion),
		))
		return nil
	}
	printSuccess(cmd, "Upgraded aim")
	return nil
}

func MigrateCmd(cmd *cobra.Command, args []string) error {
	if cmd != nil && strings.TrimSpace(cmd.CalledAs()) == "repair" {
		printWarning(cmd, "repair is deprecated; use 'aim migrate'")
	}
	if len(args) > 1 {
		return usageError(fmt.Errorf("too many arguments"))
	}
	opts := runtimeOptionsFrom(cmd)

	if len(args) == 0 {
		if opts.DryRun {
			plan, err := repo.PlanMigrationToCurrentPaths("")
			if err != nil {
				return err
			}
			if opts.JSON {
				return printJSONSuccess(cmd, plan)
			}
			writeDataf(cmd, "Dry run: migration planned=%t\n", plan.WouldChangeAnything)
			return nil
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		changed, err := func() (bool, error) {
			var changed bool
			err := withStateWriteLock(cmd, func() error {
				logOperationf(cmd, "Migrating managed apps")
				var migrateErr error
				changed, migrateErr = runWithBusyIndicator(cmd, progressMigrateApps(), func() (bool, error) {
					return migrateAllApps()
				})
				return migrateErr
			})
			return changed, err
		}()
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"changed": changed,
				"scope":   "all",
			})
		}
		if !changed {
			printSuccess(cmd, successMigrationNoop(""))
			return nil
		}
		printSuccess(cmd, successMigrationComplete(""))
		return nil
	}

	id := strings.TrimSpace(args[0])
	if id == "" {
		return usageError(fmt.Errorf("missing required argument <id>"))
	}
	if opts.DryRun {
		plan, err := repo.PlanMigrationToCurrentPaths(id)
		if err != nil {
			return wrapManagedAppLookupError(id, err)
		}
		if opts.JSON {
			return printJSONSuccess(cmd, plan)
		}
		writeDataf(cmd, "Dry run: migration planned for %s = %t\n", id, plan.WouldChangeAnything)
		return nil
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}

	changed, err := func() (bool, error) {
		var changed bool
		err := withStateWriteLock(cmd, func() error {
			logOperationf(cmd, "Migrating %s", id)
			var migrateErr error
			changed, migrateErr = runWithBusyIndicator(cmd, progressMigrateApp(id), func() (bool, error) {
				return migrateSingleApp(id)
			})
			return migrateErr
		})
		return changed, err
	}()
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}
	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"changed": changed,
			"scope":   id,
		})
	}
	if !changed {
		printSuccess(cmd, successMigrationNoop(id))
		return nil
	}
	printSuccess(cmd, successMigrationComplete(id))
	return nil
}

func AddCmd(cmd *cobra.Command, args []string) error {
	selection, err := resolveAddInput(cmd, args)
	if err != nil {
		return err
	}

	run := func() error {
		if selection.HasRef {
			return runInstallPackageRef(cmd.Context(), cmd, selection.Ref)
		}
		if strings.TrimSpace(selection.DirectURL) != "" {
			return runInstallTarget(cmd.Context(), cmd, selection.DirectURL)
		}
		if _, err := resolveIntegrateTarget(selection.Positional); err == nil {
			if err := validateAddIntegrateFlags(cmd); err != nil {
				return err
			}
			return runIntegrateTarget(cmd.Context(), cmd, selection.Positional)
		}

		return usageError(fmt.Errorf("unknown add target %q; expected <id> or <Path/To.AppImage>", selection.Positional))
	}

	if runtimeOptionsFrom(cmd).DryRun {
		return run()
	}
	return withStateWriteLock(cmd, run)
}

type addInputSelection struct {
	Positional string
	DirectURL  string
	Ref        discovery.PackageRef
	HasRef     bool
}

func resolveAddInput(cmd *cobra.Command, args []string) (addInputSelection, error) {
	ref, ok, err := resolveProviderFlagRef(cmd, args)
	if err != nil || ok {
		return addInputSelection{Ref: ref, HasRef: ok}, err
	}

	urlValue, err := flagString(cmd, "url")
	if err != nil {
		return addInputSelection{}, err
	}

	targetCount := 0
	if len(args) > 0 {
		targetCount++
	}
	if strings.TrimSpace(urlValue) != "" {
		targetCount++
	}
	if targetCount > 1 {
		return addInputSelection{}, usageError(fmt.Errorf("choose exactly one add selector: positional target, --url, --github, or --gitlab"))
	}

	if strings.TrimSpace(urlValue) != "" {
		target, err := resolveInstallTarget(urlValue)
		if err != nil {
			return addInputSelection{}, err
		}
		if err := validateInstallTargetFlags(cmd, target); err != nil {
			return addInputSelection{}, err
		}
		return addInputSelection{DirectURL: target.URL}, nil
	}

	value, err := resolveSingleInputOrPrompt(cmd, args, "<id|Path/To.AppImage>", "Local AppImage path or managed app id: ", missingInputErrorForAdd())
	if err != nil {
		if isMissingArgumentError(err) || err.Error() == missingInputErrorForAdd().Error() {
			return addInputSelection{}, printConciseHelpError(cmd, missingInputErrorForAdd().Error())
		}
		return addInputSelection{}, err
	}

	if addTargetLooksRemote(value) {
		return addInputSelection{}, positionalAddRemoteGuidance(value)
	}

	return addInputSelection{Positional: value}, nil
}

type integrateTargetKind string

const (
	integrateTargetLocalFile  integrateTargetKind = "local_file"
	integrateTargetUnlinked   integrateTargetKind = "unlinked"
	integrateTargetIntegrated integrateTargetKind = "integrated"
)

type integrateTarget struct {
	Kind      integrateTargetKind
	App       *models.App
	LocalPath string
}

func runIntegrateTarget(ctx context.Context, cmd *cobra.Command, input string) error {
	target, err := resolveIntegrateTarget(input)
	if err != nil {
		return err
	}
	opts := runtimeOptionsFrom(cmd)

	switch target.Kind {
	case integrateTargetIntegrated:
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"status": "already_integrated",
				"app":    target.App,
			})
		}
		printSuccess(cmd, fmt.Sprintf("Already integrated: %s", formatAppRef(target.App)))
		return nil
	case integrateTargetUnlinked:
		if opts.DryRun {
			result := map[string]interface{}{
				"status": "dry_run",
				"action": "reintegrate",
				"app":    target.App,
			}
			if opts.JSON {
				return printJSONSuccess(cmd, result)
			}
			writeDataf(cmd, "Dry run: would reintegrate %s [%s]\n", target.App.Name, target.App.ID)
			return nil
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		app, err := integrateExistingApp(ctx, target.App.ID)
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"status": "reintegrated",
				"app":    app,
			})
		}
		printSuccess(cmd, fmt.Sprintf("Reintegrated: %s", formatAppRef(app)))
		return nil
	case integrateTargetLocalFile:
		if opts.DryRun {
			plan, err := buildLocalIntegrateDryRunPlan(ctx, target.LocalPath)
			if err != nil {
				return err
			}
			if opts.JSON {
				return printJSONSuccess(cmd, plan)
			}
			writeDataf(cmd, "Dry run: would integrate %s\n", plan["input"])
			if appID, ok := plan["app_id"].(string); ok && appID != "" {
				writeDataf(cmd, "  Managed ID: %s\n", appID)
			}
			for _, path := range plan["planned_paths"].([]string) {
				writeDataf(cmd, "  %s\n", path)
			}
			return nil
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
		inputLabel := strings.TrimSpace(filepath.Base(target.LocalPath))
		if inputLabel == "" || inputLabel == "." || inputLabel == string(filepath.Separator) {
			inputLabel = strings.TrimSpace(target.LocalPath)
		}

		app, err := runWithBusyIndicator(cmd, fmt.Sprintf("Integrating %s", inputLabel), func() (*models.App, error) {
			return integrateLocalApp(ctx, target.LocalPath, func(existing, incoming *models.UpdateSource) (bool, error) {
				printCurrentIncoming(cmd, updateSummary(existing), updateSummary(incoming))
				prompt := formatPrompt("Replace source from", "AppImage metadata")
				return confirmAction(cmd, prompt)
			})
		})
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"status": "integrated",
				"app":    app,
			})
		}
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppRef(app)))
		return nil
	default:
		return softwareError(fmt.Errorf("unknown integrate target %q", input))
	}
}

func resolveIntegrateTarget(input string) (*integrateTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, usageError(fmt.Errorf("missing required argument <Path/To.AppImage|id>"))
	}

	if app, err := repo.GetApp(trimmed); err == nil {
		kind := integrateTargetIntegrated
		if strings.TrimSpace(app.DesktopEntryLink) == "" {
			kind = integrateTargetUnlinked
		}
		return &integrateTarget{Kind: kind, App: app}, nil
	}

	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "github:") || strings.HasPrefix(trimmed, "gitlab:") {
		return nil, usageError(fmt.Errorf("remote sources are added with 'aim add'"))
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, usageError(fmt.Errorf("direct URLs must use https; use 'aim add --url https://...'"))
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return &integrateTarget{Kind: integrateTargetLocalFile, LocalPath: trimmed}, nil
	}

	return nil, usageError(fmt.Errorf("unknown argument %s", input))
}

type installTargetKind string

const (
	installTargetDirectURL installTargetKind = "direct_url"
	installTargetGitHub    installTargetKind = "github_release"
	installTargetGitLab    installTargetKind = "gitlab_release"
)

type installTarget struct {
	Kind    installTargetKind
	URL     string
	Repo    string
	Project string
}

func resolveInstallTarget(input string) (*installTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, usageError(fmt.Errorf("missing required argument <url>"))
	}

	if isHTTPSURL(trimmed) {
		return &installTarget{Kind: installTargetDirectURL, URL: trimmed}, nil
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, usageError(fmt.Errorf("--url must use https"))
	}

	if app, err := repo.GetApp(trimmed); err == nil && app != nil {
		return nil, usageError(fmt.Errorf("managed app IDs are added with 'aim add <id>'"))
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return nil, usageError(fmt.Errorf("local AppImages are added with 'aim add <Path/To.AppImage>'"))
	}

	return nil, usageError(fmt.Errorf("--url must be a valid https URL"))
}

func validateInstallTargetFlags(cmd *cobra.Command, target *installTarget) error {
	if target == nil {
		return usageError(fmt.Errorf("missing add target"))
	}

	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}

	switch target.Kind {
	case installTargetGitHub, installTargetGitLab:
		if sha256 != "" {
			return usageError(fmt.Errorf("--sha256 is only supported with direct https URLs"))
		}
	case installTargetDirectURL:
		if assetPattern != "" {
			return usageError(fmt.Errorf("--asset is only supported with GitHub or GitLab provider sources"))
		}
		if sha256 != "" && !isSHA256Hex(sha256) {
			return usageError(fmt.Errorf("--sha256 must be a valid 64-character hexadecimal SHA-256"))
		}
	default:
		return softwareError(fmt.Errorf("unsupported add target"))
	}

	return nil
}

func integrateFromDirectURL(ctx context.Context, cmd *cobra.Command, target *installTarget, sha256 string) (*models.App, error) {
	if strings.TrimSpace(sha256) == "" {
		printWarning(cmd, "No SHA-256 provided; skipping checksum verification")
	}

	return integrateRemoteInstall(ctx, cmd, remoteInstallRequest{
		DisplayLabel:   target.URL,
		DownloadURL:    target.URL,
		ExpectedSHA256: sha256,
		BuildSource: func(app *models.App) models.Source {
			return models.Source{
				Kind: models.SourceDirectURL,
				DirectURL: &models.DirectURLSource{
					URL:          target.URL,
					SHA256:       sha256,
					DownloadedAt: app.UpdatedAt,
				},
			}
		},
		BuildUpdate: func(*models.App) *models.UpdateSource {
			return &models.UpdateSource{Kind: models.UpdateNone}
		},
	})
}

func integrateGitHubReleaseAsset(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string, release *core.GitHubReleaseAsset) (*models.App, error) {
	return integrateRemoteInstall(ctx, cmd, remoteInstallRequest{
		DisplayLabel: target.Repo,
		DownloadURL:  release.DownloadURL,
		AssetName:    release.AssetName,
		BuildSource: func(app *models.App) models.Source {
			return models.Source{
				Kind: models.SourceGitHubRelease,
				GitHubRelease: &models.GitHubReleaseSource{
					Repo:         target.Repo,
					Asset:        assetPattern,
					Tag:          release.TagName,
					AssetName:    release.AssetName,
					DownloadedAt: app.UpdatedAt,
				},
			}
		},
		BuildUpdate: func(*models.App) *models.UpdateSource {
			return &models.UpdateSource{
				Kind: models.UpdateGitHubRelease,
				GitHubRelease: &models.GitHubReleaseUpdateSource{
					Repo:  target.Repo,
					Asset: assetPattern,
				},
			}
		},
	})
}

func integrateGitLabReleaseAsset(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string, release *core.GitLabReleaseAsset) (*models.App, error) {
	return integrateRemoteInstall(ctx, cmd, remoteInstallRequest{
		DisplayLabel: target.Project,
		DownloadURL:  release.DownloadURL,
		AssetName:    release.AssetName,
		BuildSource: func(app *models.App) models.Source {
			return models.Source{
				Kind: models.SourceGitLabRelease,
				GitLabRelease: &models.GitLabReleaseSource{
					Project:      target.Project,
					Asset:        assetPattern,
					Tag:          release.TagName,
					AssetName:    release.AssetName,
					DownloadedAt: app.UpdatedAt,
				},
			}
		},
		BuildUpdate: func(*models.App) *models.UpdateSource {
			return &models.UpdateSource{
				Kind: models.UpdateGitLabRelease,
				GitLabRelease: &models.GitLabReleaseUpdateSource{
					Project: target.Project,
					Asset:   assetPattern,
				},
			}
		},
	})
}

type remoteInstallRequest struct {
	DisplayLabel   string
	DownloadURL    string
	AssetName      string
	ExpectedSHA256 string
	BuildSource    func(app *models.App) models.Source
	BuildUpdate    func(app *models.App) *models.UpdateSource
}

func integrateRemoteInstall(ctx context.Context, cmd *cobra.Command, req remoteInstallRequest) (*models.App, error) {
	fileName := updateDownloadFilename(req.AssetName, req.DownloadURL)
	downloadPath, err := stableDownloadDestination(req.DownloadURL, fileName)
	if err != nil {
		return nil, err
	}

	logOperationf(cmd, "Downloading install asset from %s", req.DownloadURL)
	if err := downloadRemoteAsset(ctx, req.DownloadURL, downloadPath, isTerminalStderr()); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.ExpectedSHA256) != "" {
		logOperationf(cmd, "Verifying %s", fileName)
		printInfo(cmd, fmt.Sprintf("Verifying %s", fileName))
		if err := verifyDownloadedUpdate(downloadPath, pendingManagedUpdate{ExpectedSHA256: req.ExpectedSHA256}); err != nil {
			return nil, err
		}
	}

	logOperationf(cmd, "Integrating %s", fileName)
	app, err := runWithBusyIndicator(cmd, fmt.Sprintf("Integrating %s", fileName), func() (*models.App, error) {
		return integrateLocalApp(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
			_ = existing
			_ = incoming
			return false, nil
		})
	})
	if err != nil {
		return nil, err
	}

	incomingUpdate := req.BuildUpdate(app)
	finalUpdate, err := chooseRemoteUpdateSource(cmd, app.ID, incomingUpdate)
	if err != nil {
		return nil, err
	}

	app.Source = req.BuildSource(app)
	app.Update = finalUpdate

	logOperationf(cmd, "Persisting %s", app.ID)
	if err := addSingleApp(app, true); err != nil {
		return nil, wrapWriteError(err)
	}

	removeStagedDownload(downloadPath)

	return app, nil
}

func chooseRemoteUpdateSource(cmd *cobra.Command, id string, incoming *models.UpdateSource) (*models.UpdateSource, error) {
	if incoming == nil {
		incoming = &models.UpdateSource{Kind: models.UpdateNone}
	}

	existingApp, err := repo.GetApp(id)
	if err != nil {
		return incoming, nil
	}

	existing := existingApp.Update
	if existing == nil || existing.Kind == models.UpdateNone {
		return incoming, nil
	}
	if updateSourcesEqual(existing, incoming) {
		return incoming, nil
	}

	printCurrentIncoming(cmd, updateSummary(existing), updateSummary(incoming))
	prompt := formatPrompt("Replace source for", id)
	confirmed, err := confirmAction(cmd, prompt)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		return existing, nil
	}

	return incoming, nil
}

func updateSourcesEqual(a, b *models.UpdateSource) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case models.UpdateNone:
		return true
	case models.UpdateGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	case models.UpdateGitLabRelease:
		return a.GitLabRelease != nil && b.GitLabRelease != nil &&
			strings.TrimSpace(a.GitLabRelease.Project) == strings.TrimSpace(b.GitLabRelease.Project) &&
			strings.TrimSpace(a.GitLabRelease.Asset) == strings.TrimSpace(b.GitLabRelease.Asset)
	case models.UpdateZsync:
		return a.Zsync != nil && b.Zsync != nil &&
			strings.TrimSpace(a.Zsync.UpdateInfo) == strings.TrimSpace(b.Zsync.UpdateInfo) &&
			strings.TrimSpace(a.Zsync.Transport) == strings.TrimSpace(b.Zsync.Transport)
	default:
		return false
	}
}

func isSHA256Hex(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 64 {
		return false
	}
	_, err := hex.DecodeString(trimmed)
	return err == nil
}

func RemoveCmd(cmd *cobra.Command, args []string) error {
	id, err := resolveSingleInputOrPrompt(cmd, args, "<id>", "Managed app id to remove: ", missingInputErrorForRemove())
	if err != nil {
		if isMissingArgumentError(err) || err.Error() == missingInputErrorForRemove().Error() {
			return printConciseHelpError(cmd, missingInputErrorForRemove().Error())
		}
		return err
	}
	unlink, err := flagBool(cmd, "unlink")
	if err != nil {
		return err
	}

	opts := runtimeOptionsFrom(cmd)
	if opts.DryRun {
		app, err := repo.GetApp(id)
		if err != nil {
			return wrapManagedAppLookupError(id, err)
		}
		plan := removeDryRunPlan(app, unlink)
		if opts.JSON {
			return printJSONSuccess(cmd, plan)
		}
		writeDataf(cmd, "Dry run: would %s %s [%s]\n", plan["action"], app.Name, app.ID)
		for _, path := range plan["paths"].([]string) {
			writeDataf(cmd, "  %s\n", path)
		}
		return nil
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}

	var app *models.App
	err = withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Removing %s", id)
		var removeErr error
		app, removeErr = removeManagedApp(cmd.Context(), id, unlink)
		return removeErr
	})
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}

	label := "Removed"
	if unlink {
		label = "Unlinked"
	}
	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"action": strings.ToLower(label),
			"app":    app,
			"unlink": unlink,
			"paths":  removeDryRunPlan(app, unlink)["paths"],
		})
	}
	printSuccess(cmd, fmt.Sprintf("%s: %s [%s]", label, app.Name, app.ID))
	return nil
}

func ListCmd(cmd *cobra.Command, args []string) error {
	_ = args
	all, err := flagBool(cmd, "all")
	if err != nil {
		return err
	}
	integrated, err := flagBool(cmd, "integrated")
	if err != nil {
		return err
	}
	unlinked, err := flagBool(cmd, "unlinked")
	if err != nil {
		return err
	}

	if (integrated && unlinked) || (all && (integrated || unlinked)) {
		return usageError(fmt.Errorf("flags --all, --integrated, and --unlinked are mutually exclusive"))
	}

	if !all && !integrated && !unlinked {
		all = true
	}

	apps, err := repo.GetAllApps()
	if err != nil {
		return err
	}

	opts := runtimeOptionsFrom(cmd)
	if len(apps) == 0 {
		if opts.JSON {
			return printJSONSuccess(cmd, []listOutputRow{})
		}
		if opts.CSV {
			return writeCSV(cmd, listCSVHeader(), nil)
		}
		if opts.Plain {
			writePlainList(cmd, nil)
			return nil
		}
		printSuccess(cmd, "No managed apps")
		return nil
	}

	orderedApps := sortAppsByID(apps)
	integratedRows := make([]*models.App, 0, len(apps))
	unlinkedRows := make([]*models.App, 0, len(apps))
	for _, app := range orderedApps {
		if len(app.DesktopEntryLink) > 0 {
			integratedRows = append(integratedRows, app)
			continue
		}
		unlinkedRows = append(unlinkedRows, app)
	}

	if integrated && len(integratedRows) == 0 {
		printSuccess(cmd, "No integrated apps")
		return nil
	}
	if unlinked && len(unlinkedRows) == 0 {
		printSuccess(cmd, "No unlinked apps")
		return nil
	}

	selected := make([]*models.App, 0, len(orderedApps))
	switch {
	case all:
		selected = append(selected, integratedRows...)
		selected = append(selected, unlinkedRows...)
	case integrated:
		selected = append(selected, integratedRows...)
	case unlinked:
		selected = append(selected, unlinkedRows...)
	}

	if opts.JSON {
		rows := make([]listOutputRow, 0, len(selected))
		for _, app := range selected {
			rows = append(rows, newListOutputRow(app))
		}
		return printJSONSuccess(cmd, rows)
	}
	if opts.CSV {
		rows := make([][]string, 0, len(selected))
		for _, app := range selected {
			rows = append(rows, newListOutputRow(app).csvRow())
		}
		return writeCSV(cmd, listCSVHeader(), rows)
	}
	if opts.Plain {
		writePlainList(cmd, selected)
		return nil
	}

	idWidth := listIDColumnWidth(integratedRows, unlinkedRows)
	nameWidth := listNameDisplayWidth(integratedRows, unlinkedRows)
	header := fmt.Sprintf("%-*s %-*s %s", idWidth, "ID", nameWidth, "App Name", "Version")
	printSection(cmd, header)

	if all || integrated {
		for _, app := range integratedRows {
			writeDataf(cmd, "%s\n", formatListRow(app, idWidth, nameWidth))
		}
	}

	if all || unlinked {
		for _, app := range unlinkedRows {
			row := formatListRow(app, idWidth, nameWidth)
			writeDataf(cmd, "%s\n", colorize(shouldColorStdout(cmd), "\033[2m\033[3m", row))
		}
	}

	return nil
}

func InfoCmd(cmd *cobra.Command, args []string) error {
	input, ref, ok, err := resolveInfoInput(cmd, args)
	if err != nil {
		return err
	}
	if ok {
		return runShowPackageRef(cmd.Context(), cmd, ref)
	}

	if _, err := resolveInspectTarget(input); err == nil {
		return runInspectTarget(cmd.Context(), cmd, input)
	}

	if refErr := positionalInfoRemoteGuidance(input); refErr != nil {
		return refErr
	}

	return usageError(fmt.Errorf("unknown info target %q; expected <id> or <Path/To.AppImage>", input))
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

type inspectTargetKind string

const (
	inspectTargetManaged inspectTargetKind = "managed"
	inspectTargetLocal   inspectTargetKind = "local"
)

type inspectTarget struct {
	Kind inspectTargetKind
	App  *models.App
	Path string
}

func runInspectTarget(ctx context.Context, cmd *cobra.Command, input string) error {
	target, err := resolveInspectTarget(input)
	if err != nil {
		return err
	}

	switch target.Kind {
	case inspectTargetManaged:
		return inspectManagedApp(ctx, cmd, target.App)
	case inspectTargetLocal:
		return inspectLocalAppImage(ctx, cmd, target.Path)
	default:
		return softwareError(fmt.Errorf("unknown inspect target %q", input))
	}
}

func runShowPackageRef(ctx context.Context, cmd *cobra.Command, ref discovery.PackageRef) error {
	metadata, err := resolvePackageMetadataWithProgress(cmd, formatProviderRef(ref), func() (*discovery.PackageMetadata, error) {
		return resolvePackageMetadataFromRef(ctx, ref, "")
	})
	if err != nil {
		return err
	}

	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":     "package_metadata",
			"metadata": packageMetadataOutput(metadata),
		})
	}

	printPackageMetadata(cmd, metadata)
	return nil
}

func runInstallTarget(ctx context.Context, cmd *cobra.Command, refArg string) error {
	target, err := resolveInstallTarget(refArg)
	if err != nil {
		return err
	}
	if err := validateInstallTargetFlags(cmd, target); err != nil {
		return err
	}

	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	opts := runtimeOptionsFrom(cmd)

	if opts.DryRun {
		plan, err := buildInstallDryRunPlan(ctx, cmd, refArg, target, assetPattern, sha256)
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, plan)
		}
		writeDataf(cmd, "Dry run: would install %s\n", plan["target"])
		return nil
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}

	var app *models.App
	switch target.Kind {
	case installTargetDirectURL:
		app, err = integrateFromDirectURL(ctx, cmd, target, sha256)
	case installTargetGitHub, installTargetGitLab:
		var metadata *discovery.PackageMetadata
		metadata, err = resolveInstallablePackageMetadataFromTarget(ctx, cmd, refArg, target, assetPattern)
		if err == nil {
			app, err = installPackageMetadata(ctx, cmd, metadata)
		}
	default:
		err = softwareError(fmt.Errorf("unsupported add target"))
	}
	if err != nil {
		return err
	}

	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"status": "installed",
			"app":    app,
		})
	}
	printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppRef(app)))
	return nil
}

func runInstallPackageRef(ctx context.Context, cmd *cobra.Command, ref discovery.PackageRef) error {
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	if sha256 != "" {
		return usageError(fmt.Errorf("--sha256 is only supported with direct https URLs"))
	}
	opts := runtimeOptionsFrom(cmd)

	if opts.DryRun {
		metadata, err := resolvePackageMetadataWithProgress(cmd, formatProviderRef(ref), func() (*discovery.PackageMetadata, error) {
			return resolvePackageMetadataFromRef(ctx, ref, assetPattern)
		})
		if err != nil {
			return err
		}
		if opts.JSON {
			return printJSONSuccess(cmd, map[string]interface{}{
				"action":   "install",
				"target":   formatProviderRef(ref),
				"provider": ref,
				"metadata": packageMetadataOutput(metadata),
			})
		}
		writeDataf(cmd, "Dry run: would install %s\n", formatProviderRef(ref))
		return nil
	}
	if err := mustEnsureRuntimeDirs(); err != nil {
		return err
	}

	metadata, err := resolveInstallablePackageMetadataFromRef(ctx, cmd, ref, assetPattern)
	if err != nil {
		return err
	}

	app, err := installPackageMetadata(ctx, cmd, metadata)
	if err != nil {
		return err
	}

	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"status": "installed",
			"app":    app,
		})
	}
	printSuccess(cmd, fmt.Sprintf("Installed: %s", formatAppRef(app)))
	return nil
}

func commandSingleArg(args []string, usage string) (string, error) {
	if len(args) > 1 {
		return "", usageError(fmt.Errorf("too many arguments"))
	}

	value := ""
	if len(args) > 0 {
		value = strings.TrimSpace(args[0])
	}
	if value == "" {
		return "", usageError(fmt.Errorf("missing required argument %s", usage))
	}

	return value, nil
}

func validateAddIntegrateFlags(cmd *cobra.Command) error {
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	if assetPattern != "" {
		return usageError(fmt.Errorf("--asset is only supported with GitHub or GitLab provider sources"))
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}
	if sha256 != "" {
		return usageError(fmt.Errorf("--sha256 is only supported with direct https:// add sources"))
	}

	return nil
}

func resolveInspectTarget(input string) (*inspectTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, usageError(fmt.Errorf("missing required argument <id|Path/To.AppImage>"))
	}

	if app, err := repo.GetApp(trimmed); err == nil {
		return &inspectTarget{Kind: inspectTargetManaged, App: app}, nil
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return &inspectTarget{Kind: inspectTargetLocal, Path: trimmed}, nil
	}

	return nil, rewriteMissingAppError(trimmed, fmt.Errorf("no app with id %s", trimmed))
}

func inspectManagedApp(ctx context.Context, cmd *cobra.Command, app *models.App) error {
	if app == nil {
		return fmt.Errorf("managed app cannot be empty")
	}

	embeddedSource, _ := embeddedUpdateSourceForPath(app.ExecPath)

	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":            "managed_app",
			"app":             app,
			"embedded_update": embeddedSource,
		})
	}

	printSection(cmd, sectionApp)
	writeDataf(cmd, "Name: %s\n", strings.TrimSpace(app.Name))
	writeDataf(cmd, "ID: %s\n", strings.TrimSpace(app.ID))
	writeDataf(cmd, "Version: %s\n", displayVersion(app.Version))
	writeDataf(cmd, "Exec path: %s\n", strings.TrimSpace(app.ExecPath))

	printSection(cmd, sectionUpdates)
	writeDataf(cmd, "Configured source: %s\n", updateSummaryOrNone(app.Update))
	writeDataf(cmd, "Embedded source: %s\n", updateSummaryOrNone(embeddedSource))

	printSection(cmd, sectionState)
	writeDataf(cmd, "Update available: %s\n", yesNo(app.UpdateAvailable))
	if strings.TrimSpace(app.LatestVersion) != "" {
		writeDataf(cmd, "Latest known version: %s\n", displayVersion(app.LatestVersion))
	}
	if strings.TrimSpace(app.LastCheckedAt) != "" {
		writeDataf(cmd, "Last checked: %s\n", strings.TrimSpace(app.LastCheckedAt))
	}

	_ = ctx
	return nil
}

func inspectLocalAppImage(ctx context.Context, cmd *cobra.Command, src string) error {
	label := strings.TrimSpace(filepath.Base(src))
	if label == "" || label == "." || label == string(filepath.Separator) {
		label = strings.TrimSpace(src)
	}

	result, err := runWithBusyIndicator(cmd, fmt.Sprintf("Inspecting %s", label), func() (*struct {
		info           *core.AppInfo
		embeddedSource *models.UpdateSource
	}, error) {
		info, err := readAppImageInfo(ctx, src)
		if err != nil {
			return nil, err
		}

		embeddedSource, _ := embeddedUpdateSourceForPath(src)
		return &struct {
			info           *core.AppInfo
			embeddedSource *models.UpdateSource
		}{
			info:           info,
			embeddedSource: embeddedSource,
		}, nil
	})
	if err != nil {
		return err
	}
	info := result.info
	embeddedSource := result.embeddedSource

	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"kind":            "local_appimage",
			"path":            strings.TrimSpace(src),
			"app":             info,
			"embedded_update": embeddedSource,
		})
	}

	printSection(cmd, sectionAppImage)
	writeDataf(cmd, "Path: %s\n", strings.TrimSpace(src))
	writeDataf(cmd, "Name: %s\n", strings.TrimSpace(info.Name))
	writeDataf(cmd, "ID: %s\n", strings.TrimSpace(info.ID))
	writeDataf(cmd, "Version: %s\n", displayVersion(info.Version))

	printSection(cmd, sectionUpdates)
	writeDataf(cmd, "Embedded source: %s\n", updateSummaryOrNone(embeddedSource))

	return nil
}

func updateSummaryOrNone(update *models.UpdateSource) string {
	if update == nil || update.Kind == models.UpdateNone {
		return "none"
	}
	return updateSummary(update)
}

func updateSourceFromEmbeddedInfo(info *core.UpdateInfo) (*models.UpdateSource, error) {
	if info == nil {
		return nil, softwareError(fmt.Errorf("missing embedded update info"))
	}
	if info.Kind != models.UpdateZsync {
		return nil, softwareError(fmt.Errorf("unsupported embedded update info kind %q", info.Kind))
	}

	return &models.UpdateSource{
		Kind: models.UpdateZsync,
		Zsync: &models.ZsyncUpdateSource{
			UpdateInfo: strings.TrimSpace(info.UpdateInfo),
			Transport:  strings.TrimSpace(info.Transport),
		},
	}, nil
}

func embeddedUpdateSourceForPath(path string) (*models.UpdateSource, error) {
	info, err := getAppImageUpdateInfo(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	return updateSourceFromEmbeddedInfo(info)
}

func UpdateCmd(cmd *cobra.Command, args []string) error {
	setID, err := flagString(cmd, "set")
	if err != nil {
		return err
	}
	unsetID, err := flagString(cmd, "unset")
	if err != nil {
		return err
	}
	checkOnlyChanged := flagChanged(cmd, "check-only")
	setSpecified := flagChanged(cmd, "set")
	unsetSpecified := flagChanged(cmd, "unset")
	hasSourceFlags := hasUpdateSetFlags(cmd)

	if len(args) > 1 {
		return usageError(fmt.Errorf("too many arguments"))
	}
	targetID := ""
	if len(args) == 1 {
		targetID = strings.TrimSpace(args[0])
		if targetID == "" {
			return usageError(fmt.Errorf("missing required argument <id>"))
		}
	}
	setID = strings.TrimSpace(setID)
	unsetID = strings.TrimSpace(unsetID)

	switch {
	case setSpecified && unsetSpecified:
		return usageError(fmt.Errorf("--set and --unset are mutually exclusive"))
	case setSpecified && targetID != "":
		return usageError(fmt.Errorf("--set is not supported with positional update targets; use either 'aim update <id>' or 'aim update --set <id> ...'"))
	case unsetSpecified && targetID != "":
		return usageError(fmt.Errorf("--unset is not supported with positional update targets; use either 'aim update <id>' or 'aim update --unset <id>'"))
	case setSpecified && checkOnlyChanged:
		return usageError(fmt.Errorf("--check-only is not supported with 'aim update --set'"))
	case unsetSpecified && checkOnlyChanged:
		return usageError(fmt.Errorf("--check-only is not supported with 'aim update --unset'"))
	case unsetSpecified && hasSourceFlags:
		return usageError(fmt.Errorf("update source flags can only be used with 'aim update --set <id> ...'"))
	case setSpecified && setID == "":
		return printConciseHelpError(cmd, "missing required input; pass --set <id> to configure an update source")
	case unsetSpecified && unsetID == "":
		return printConciseHelpError(cmd, "missing required input; pass --unset <id> to remove an update source")
	case !setSpecified && !unsetSpecified && hasSourceFlags:
		return usageError(fmt.Errorf("update source flags can only be used with 'aim update --set <id> ...'"))
	}

	if setSpecified {
		return runUpdateSetMode(cmd, setID)
	}
	if unsetSpecified {
		return runUpdateUnsetMode(cmd, unsetID)
	}

	return runManagedUpdate(cmd.Context(), cmd, targetID)
}

func UpdateCheckRemovedCmd(cmd *cobra.Command, args []string) error {
	_ = cmd
	_ = args
	return fmt.Errorf("`aim update check` has been removed; use `aim update [<id>]` for managed apps")
}

func unsetManagedUpdateSource(cmd *cobra.Command, app *models.App, prompt string, showCurrent bool) (bool, error) {
	if app == nil {
		return false, softwareError(fmt.Errorf("managed app cannot be empty"))
	}
	if app.Update == nil || app.Update.Kind == models.UpdateNone {
		printSuccess(cmd, fmt.Sprintf("No update source configured for %s", app.ID))
		return false, nil
	}

	if showCurrent {
		printCurrentValue(cmd, updateSummary(app.Update))
	}

	confirmed, err := confirmAction(cmd, prompt)
	if err != nil {
		return false, err
	}
	if !confirmed {
		printWarning(cmd, "Update source unchanged")
		return false, nil
	}

	app.Update = &models.UpdateSource{Kind: models.UpdateNone}
	if err := repo.UpdateApp(app); err != nil {
		return false, wrapWriteError(err)
	}

	printSuccess(cmd, "Update source unset")
	return true, nil
}

func hasUpdateSetFlags(cmd *cobra.Command) bool {
	keys := []string{"github", "gitlab", "asset", "zsync", "embedded", "manifest-url", "url", "sha256"}
	for _, key := range keys {
		if flagChanged(cmd, key) {
			return true
		}
	}
	return false
}

func validateEmbeddedUpdateSetFlags(cmd *cobra.Command) error {
	githubRepo, err := flagString(cmd, "github")
	if err != nil {
		return err
	}
	gitlabProject, err := flagString(cmd, "gitlab")
	if err != nil {
		return err
	}
	zsyncURL, err := flagString(cmd, "zsync")
	if err != nil {
		return err
	}
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return err
	}
	manifestURL, err := flagString(cmd, "manifest-url")
	if err != nil {
		return err
	}
	directURL, err := flagString(cmd, "url")
	if err != nil {
		return err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return err
	}

	if manifestURL != "" {
		return usageError(fmt.Errorf("--manifest-url is no longer supported; use --github, --gitlab, --zsync, or --embedded"))
	}
	if directURL != "" {
		return usageError(fmt.Errorf("--url is no longer supported; use --github, --gitlab, --zsync, or --embedded"))
	}
	if sha256 != "" {
		return usageError(fmt.Errorf("--sha256 is no longer supported; use --github, --gitlab, --zsync, or --embedded"))
	}
	if assetPattern != "" {
		return usageError(fmt.Errorf("--asset is only supported with --github or --gitlab"))
	}

	selectorCount := 1
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}
	if selectorCount > 1 {
		return usageError(fmt.Errorf("update source flags are mutually exclusive"))
	}

	return nil
}

func runUpdateSetMode(cmd *cobra.Command, id string) error {
	if strings.TrimSpace(id) == "" {
		return printConciseHelpError(cmd, "missing required input; pass --set <id> to configure an update source")
	}
	opts := runtimeOptionsFrom(cmd)
	embedded, err := flagBool(cmd, "embedded")
	if err != nil {
		return err
	}

	var incomingSource *models.UpdateSource
	if !embedded {
		incomingSource, err = resolveUpdateSourceFromSetFlags(cmd)
		if err != nil {
			return err
		}
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}

	if embedded {
		if err := validateEmbeddedUpdateSetFlags(cmd); err != nil {
			return err
		}

		incomingSource, err = embeddedUpdateSourceForPath(app.ExecPath)
		if err != nil {
			printWarning(cmd, warningNoEmbeddedSource())
			if app.Update == nil || app.Update.Kind == models.UpdateNone {
				if opts.JSON {
					return printJSONSuccess(cmd, buildUpdateUnsetDryRunResult(id, app.Update))
				}
				return nil
			}

			printCurrentValue(cmd, updateSummary(app.Update))
			prompt := formatPrompt("Unset source for", id)
			_, err := unsetManagedUpdateSource(cmd, app, prompt, false)
			return err
		}
	}

	if app.Update != nil && app.Update.Kind != models.UpdateNone && !updateSourcesEqual(app.Update, incomingSource) {
		printCurrentIncoming(cmd, updateSummary(app.Update), updateSummary(incomingSource))
		prompt := formatPrompt("Replace source for", id)
		confirmed, err := confirmAction(cmd, prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			printWarning(cmd, "Update source unchanged")
			return nil
		}
	}

	if opts.DryRun {
		result := buildUpdateSetDryRunResult(id, app.Update, incomingSource)
		if opts.JSON {
			return printJSONSuccess(cmd, result)
		}
		writeDataf(cmd, "Dry run: would set update source for %s\n", id)
		return nil
	}

	if err := withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Setting update source for %s", id)
		app.Update = incomingSource
		if err := repo.UpdateApp(app); err != nil {
			return wrapWriteError(err)
		}
		return nil
	}); err != nil {
		return err
	}

	if opts.JSON {
		return printJSONSuccess(cmd, map[string]interface{}{
			"action": "set_update_source",
			"id":     id,
			"source": incomingSource,
		})
	}
	printSuccess(cmd, fmt.Sprintf("Update source set: %s", updateSummary(incomingSource)))
	return nil
}

func runUpdateUnsetMode(cmd *cobra.Command, id string) error {
	if strings.TrimSpace(id) == "" {
		return printConciseHelpError(cmd, "missing required input; pass --unset <id> to remove an update source")
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return wrapManagedAppLookupError(id, err)
	}

	if runtimeOptionsFrom(cmd).DryRun {
		result := buildUpdateUnsetDryRunResult(id, app.Update)
		if runtimeOptionsFrom(cmd).JSON {
			return printJSONSuccess(cmd, result)
		}
		writeDataf(cmd, "Dry run: would unset update source for %s\n", id)
		return nil
	}

	prompt := formatPrompt("Unset source for", id)
	_, err = func() (bool, error) {
		var changed bool
		err := withStateWriteLock(cmd, func() error {
			logOperationf(cmd, "Unsetting update source for %s", id)
			var unsetErr error
			changed, unsetErr = unsetManagedUpdateSource(cmd, app, prompt, true)
			return unsetErr
		})
		return changed, err
	}()
	return err
}

func removedUpdateSetCommandError() error {
	return usageError(fmt.Errorf("aim update set has been removed; use 'aim update --set <id> ...'"))
}

func removedUpdateUnsetCommandError() error {
	return usageError(fmt.Errorf("aim update unset has been removed; use 'aim update --unset <id>'"))
}

func resolveUpdateSourceFromSetFlags(cmd *cobra.Command) (*models.UpdateSource, error) {
	githubRepo, err := flagString(cmd, "github")
	if err != nil {
		return nil, err
	}
	gitlabProject, err := flagString(cmd, "gitlab")
	if err != nil {
		return nil, err
	}
	assetPattern, err := flagString(cmd, "asset")
	if err != nil {
		return nil, err
	}
	zsyncURL, err := flagString(cmd, "zsync")
	if err != nil {
		return nil, err
	}
	manifestURL, err := flagString(cmd, "manifest-url")
	if err != nil {
		return nil, err
	}
	directURL, err := flagString(cmd, "url")
	if err != nil {
		return nil, err
	}
	sha256, err := flagString(cmd, "sha256")
	if err != nil {
		return nil, err
	}

	if manifestURL != "" {
		return nil, usageError(fmt.Errorf("--manifest-url is no longer supported; use --github, --gitlab, --zsync, or --embedded"))
	}
	if directURL != "" {
		return nil, usageError(fmt.Errorf("--url is no longer supported; use --github, --gitlab, --zsync, or --embedded"))
	}
	if sha256 != "" {
		return nil, usageError(fmt.Errorf("--sha256 is no longer supported; use --github, --gitlab, --zsync, or --embedded"))
	}

	selectorCount := 0
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}

	if selectorCount == 0 {
		return nil, usageError(fmt.Errorf("missing update source; set one of --github, --gitlab, --zsync, or --embedded"))
	}
	if selectorCount > 1 {
		return nil, usageError(fmt.Errorf("update source flags are mutually exclusive"))
	}

	if githubRepo != "" {
		ref, err := validateGitHubRepoFlag(githubRepo)
		if err != nil {
			return nil, err
		}
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitHubRelease,
			GitHubRelease: &models.GitHubReleaseUpdateSource{
				Repo:  ref.ProviderRef,
				Asset: assetPattern,
			},
		}, nil
	}

	if gitlabProject != "" {
		ref, err := validateGitLabProjectFlag(gitlabProject)
		if err != nil {
			return nil, err
		}
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitLabRelease,
			GitLabRelease: &models.GitLabReleaseUpdateSource{
				Project: ref.ProviderRef,
				Asset:   assetPattern,
			},
		}, nil
	}

	if zsyncURL != "" {
		if assetPattern != "" {
			return nil, usageError(fmt.Errorf("--asset is only supported with --github or --gitlab"))
		}
		if !isHTTPSURL(zsyncURL) {
			return nil, usageError(fmt.Errorf("--zsync must be a valid https URL"))
		}
		return &models.UpdateSource{
			Kind: models.UpdateZsync,
			Zsync: &models.ZsyncUpdateSource{
				UpdateInfo: "zsync|" + zsyncURL,
				Transport:  "zsync",
			},
		}, nil
	}

	if assetPattern != "" {
		return nil, usageError(fmt.Errorf("--asset is only supported with --github or --gitlab"))
	}
	return nil, usageError(fmt.Errorf("missing update source; set one of --github, --gitlab, --zsync, or --embedded"))
}

type pendingManagedUpdate struct {
	App            *models.App
	URL            string
	Asset          string
	Label          string
	Available      bool
	Latest         string
	ExpectedSHA1   string
	ExpectedSHA256 string
	Transport      string
	ZsyncURL       string
	FromKind       models.UpdateKind
}

type managedCheckResult struct {
	app    *models.App
	update *pendingManagedUpdate
	err    error
}

type managedCheckFailure struct {
	AppID  string
	Reason string
}

var runAppUpdateCheck = checkAppUpdate
var runZsyncUpdateCheck = core.ZsyncUpdateCheck
var runGitHubReleaseUpdateCheck = core.GitHubReleaseUpdateCheck
var runGitLabReleaseUpdateCheck = core.GitLabReleaseUpdateCheck
var discoveryBackends = func() []discovery.DiscoveryBackend {
	return []discovery.DiscoveryBackend{
		discovery.GitHubBackend{},
		discovery.GitLabBackend{},
	}
}
var resolveGitHubReleaseAsset = core.ResolveGitHubReleaseAsset
var resolveGitLabReleaseAsset = core.ResolveGitLabReleaseAsset
var downloadRemoteAsset = downloadUpdateAsset
var downloadManagedRemoteAsset = downloadUpdateAssetWithProgress
var integrateManagedUpdate = core.IntegrateFromLocalFileWithoutCacheRefreshOrPersist
var zsyncLookPath = exec.LookPath
var zsyncCommandContext = exec.CommandContext
var checkAimUpgrade = core.CheckForAimUpgrade
var runUpgradeViaInstaller = core.UpgradeViaInstaller
var runManagedApply = applyManagedUpdate
var integrateExistingApp = core.IntegrateExisting
var integrateLocalApp = core.IntegrateFromLocalFile
var readAppImageInfo = core.ReadAppImageInfo
var getAppImageUpdateInfo = core.GetUpdateInfo
var migrateAllApps = repo.MigrateToCurrentPathsChanged
var migrateSingleApp = repo.MigrateAppToCurrentPaths
var removeManagedApp = core.Remove
var addAppsBatch = repo.AddAppsBatch
var addSingleApp = repo.AddApp

const defaultReleaseAssetPattern = "*.AppImage"

func runManagedUpdate(ctx context.Context, cmd *cobra.Command, targetID string) error {
	apps, err := collectManagedUpdateTargets(targetID)
	if err != nil {
		return err
	}

	autoApply, err := flagBool(cmd, "yes")
	if err != nil {
		return err
	}
	checkOnly, err := flagBool(cmd, "check-only")
	if err != nil {
		return err
	}

	opts := runtimeOptionsFrom(cmd)
	if !opts.DryRun {
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
	}
	var checkCache *updateCheckCacheFile
	if !opts.DryRun && runtimePrepared(cmd) {
		checkCache, err = loadUpdateCheckCache()
		if err != nil {
			return wrapWriteError(err)
		}
	}
	var pending []pendingManagedUpdate
	checkFailures := 0
	singleStatusPrinted := false
	failures := make([]managedCheckFailure, 0)
	checkResults, err := runManagedChecksWithCache(cmd, apps, checkCache)
	if err != nil {
		return wrapWriteError(err)
	}
	metadataUpdates := make([]repo.CheckMetadataUpdate, 0, len(checkResults))
	rows := make([]updateOutputRow, 0, len(checkResults))
	rowIndexByID := map[string]int{}
	checkedAt := util.NowISO()

	for idx, result := range checkResults {
		app := result.app
		update := result.update
		err := result.err
		if err != nil {
			if !opts.DryRun {
				metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
					ID:            app.ID,
					Checked:       false,
					Available:     app.UpdateAvailable,
					Latest:        app.LatestVersion,
					LastCheckedAt: checkedAt,
				})
			}

			if targetID != "" && !opts.DryRun {
				if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
					return wrapWriteError(metaErr)
				}
				metadataUpdates = metadataUpdates[:0]
			}
			rows = append(rows, newUpdateOutputRow(app, update, "check_failed", checkedAt))
			if app != nil {
				rowIndexByID[app.ID] = len(rows) - 1
			}
			if targetID != "" {
				return withUserGuidance(
					tempFailError(err),
					fmt.Sprintf("Can't check updates for %s.", app.ID),
					fmt.Sprintf("Rerun with 'aim update %s --debug' to see more detail.", app.ID),
				)
			}

			checkFailures++
			failures = append(failures, managedCheckFailure{
				AppID:  app.ID,
				Reason: rewriteBatchCheckFailure(app.ID, err),
			})
			continue
		}

		if !opts.DryRun && app != nil {
			setCachedManagedUpdate(checkCache, app, managedCheckCacheKey(app, idx), update)
		}

		if update == nil {
			status := "no_update_information"
			if app.Update == nil || app.Update.Kind == models.UpdateNone {
				status = "no_update_source"
			}
			rows = append(rows, newUpdateOutputRow(app, nil, status, checkedAt))
			if app != nil {
				rowIndexByID[app.ID] = len(rows) - 1
			}
			if targetID != "" && !shouldUseStructuredOutput(cmd) {
				if status == "no_update_source" {
					printSuccess(cmd, fmt.Sprintf("No update source configured for %s", app.ID))
				} else {
					printSuccess(cmd, fmt.Sprintf("No update information for %s", app.ID))
				}
				singleStatusPrinted = true
			}
			continue
		}

		if !opts.DryRun {
			metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
				ID:            app.ID,
				Checked:       true,
				Available:     update.Available,
				Latest:        update.Latest,
				LastCheckedAt: checkedAt,
			})
		}

		if targetID != "" && !opts.DryRun {
			if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
				return wrapWriteError(metaErr)
			}
			metadataUpdates = metadataUpdates[:0]
		}

		if update.URL == "" {
			rows = append(rows, newUpdateOutputRow(app, update, "up_to_date", checkedAt))
			if app != nil {
				rowIndexByID[app.ID] = len(rows) - 1
			}
			if targetID != "" && !shouldUseStructuredOutput(cmd) {
				printSuccess(cmd, fmt.Sprintf("Up to date: %s %s", app.ID, displayVersion(app.Version)))
				singleStatusPrinted = true
			}
			continue
		}

		rows = append(rows, newUpdateOutputRow(app, update, "update_available", checkedAt))
		if app != nil {
			rowIndexByID[app.ID] = len(rows) - 1
		}

		msg := buildManagedUpdateMessage(*update, checkOnly)
		if targetID == "" {
			transition := strings.TrimSpace(updateVersionTransition(*update))
			if transition == "" {
				transition = "unknown"
			}
			msg = fmt.Sprintf("[%s] %s", app.ID, transition)
		}
		printWarning(cmd, msg)
		if checkOnly {
			writeLogf(cmd, "  Download: %s\n", update.URL)
			if showManagedUpdateAsset(update.Asset) {
				writeLogf(cmd, "  Asset: %s\n", strings.TrimSpace(update.Asset))
			}
		}

		pending = append(pending, *update)
	}

	if !opts.DryRun {
		err = flushManagedCheckMetadata(metadataUpdates)
	}
	if err != nil {
		if targetID != "" {
			return wrapWriteError(err)
		}
		checkFailures++
		printError(cmd, userMessageForError(wrapWriteError(err)).Summary)
	}
	if !opts.DryRun && checkCache != nil {
		if cacheErr := saveUpdateCheckCache(checkCache); cacheErr != nil {
			return wrapWriteError(cacheErr)
		}
	}

	if targetID == "" && len(failures) > 0 {
		printManagedCheckFailures(cmd, failures)
	}

	if len(pending) == 0 {
		if targetID != "" && singleStatusPrinted {
			return nil
		}
		if opts.JSON {
			return printJSONSuccess(cmd, rows)
		}
		if opts.CSV {
			csvRows := make([][]string, 0, len(rows))
			for _, row := range rows {
				csvRows = append(csvRows, row.csvRow())
			}
			return writeCSV(cmd, updateCSVHeader(), csvRows)
		}
		if opts.Plain {
			writePlainUpdateRows(cmd, rows)
			return nil
		}
		if checkFailures > 0 {
			printWarning(cmd, "No updates applied; some checks failed")
			return nil
		}
		printSuccess(cmd, "All apps are up to date")
		return nil
	}

	if checkOnly {
		if opts.JSON {
			return printJSONSuccess(cmd, rows)
		}
		if opts.CSV {
			csvRows := make([][]string, 0, len(rows))
			for _, row := range rows {
				csvRows = append(csvRows, row.csvRow())
			}
			return writeCSV(cmd, updateCSVHeader(), csvRows)
		}
		if opts.Plain {
			writePlainUpdateRows(cmd, rows)
			return nil
		}
		return nil
	}

	if opts.DryRun {
		for idx := range rows {
			if rows[idx].Status == "update_available" {
				rows[idx].Status = "dry_run_pending"
			}
		}
		if opts.JSON {
			return printJSONSuccess(cmd, rows)
		}
		if opts.CSV {
			csvRows := make([][]string, 0, len(rows))
			for _, row := range rows {
				csvRows = append(csvRows, row.csvRow())
			}
			return writeCSV(cmd, updateCSVHeader(), csvRows)
		}
		if opts.Plain {
			writePlainUpdateRows(cmd, rows)
			return nil
		}
		printInfo(cmd, "Dry run: no updates were applied")
		return nil
	}

	if !autoApply {
		prompt := formatPrompt("Apply updates to", fmt.Sprintf("%d app(s)", len(pending)))
		if targetID != "" {
			prompt = formatPrompt("Apply updates to", targetID)
		}

		confirmed, err := confirmAction(cmd, prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			for idx := range rows {
				if rows[idx].Status == "update_available" {
					rows[idx].Status = "apply_skipped"
				}
			}
			if opts.JSON {
				return printJSONSuccess(cmd, rows)
			}
			if opts.CSV {
				csvRows := make([][]string, 0, len(rows))
				for _, row := range rows {
					csvRows = append(csvRows, row.csvRow())
				}
				return writeCSV(cmd, updateCSVHeader(), csvRows)
			}
			if opts.Plain {
				writePlainUpdateRows(cmd, rows)
				return nil
			}
			printWarning(cmd, "No updates applied")
			return nil
		}
	}

	var (
		applyResults  []managedApplyResult
		applyFailures int
		appliedApps   []*models.App
		persistErr    error
	)
	err = withStateWriteLock(cmd, func() error {
		logOperationf(cmd, "Applying %d managed update(s)", len(pending))
		applyResults = runManagedApplies(ctx, cmd, pending)
		applyFailures = 0
		appliedApps = make([]*models.App, 0, len(applyResults))
		for _, result := range applyResults {
			if result.err != nil {
				applyFailures++
				continue
			}
			if result.updatedApp != nil {
				appliedApps = append(appliedApps, result.updatedApp)
				if idx, ok := rowIndexByID[result.updatedApp.ID]; ok {
					rows[idx].Status = "updated"
					rows[idx].CurrentVersion = result.updatedApp.Version
				}
			}
		}

		if len(appliedApps) > 0 {
			core.RefreshDesktopIntegrationCaches(ctx)
		}

		logOperationf(cmd, "Persisting applied updates")
		persistErr = persistManagedAppliedApps(appliedApps)
		return nil
	})
	if err != nil {
		return err
	}
	if len(appliedApps) > 0 && checkCache != nil {
		invalidateCachedManagedUpdates(checkCache, appliedAppIDs(appliedApps)...)
		if cacheErr := saveUpdateCheckCache(checkCache); cacheErr != nil {
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

	if opts.JSON {
		return printJSONSuccess(cmd, rows)
	}
	if opts.CSV {
		csvRows := make([][]string, 0, len(rows))
		for _, row := range rows {
			csvRows = append(csvRows, row.csvRow())
		}
		return writeCSV(cmd, updateCSVHeader(), csvRows)
	}
	if opts.Plain {
		writePlainUpdateRows(cmd, rows)
		return nil
	}

	return nil
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
	results := make([]managedCheckResult, len(apps))
	if len(apps) == 0 {
		return results
	}

	groups := make(map[string][]int, len(apps))
	orderedKeys := make([]string, 0, len(apps))
	for idx, app := range apps {
		key := managedCheckCacheKey(app, idx)
		if _, exists := groups[key]; !exists {
			orderedKeys = append(orderedKeys, key)
		}
		groups[key] = append(groups[key], idx)
	}

	jobs := make(chan int, len(orderedKeys))
	workerCount := managedCheckWorkerCount(len(orderedKeys))

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for keyIdx := range jobs {
				key := orderedKeys[keyIdx]
				indices := groups[key]
				firstIdx := indices[0]
				primaryApp := apps[firstIdx]

				update, err := runAppUpdateCheck(primaryApp)
				for _, idx := range indices {
					app := apps[idx]
					results[idx] = managedCheckResult{
						app:    app,
						update: clonePendingManagedUpdateForApp(update, app),
						err:    err,
					}
				}
			}
		}()
	}

	for keyIdx := range orderedKeys {
		jobs <- keyIdx
	}
	close(jobs)

	wg.Wait()
	return results
}

func runManagedChecksWithCache(cmd *cobra.Command, apps []*models.App, cache *updateCheckCacheFile) ([]managedCheckResult, error) {
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
	if app == nil || app.Update == nil {
		return fmt.Sprintf("none:%d", fallbackIdx)
	}

	kind := strings.TrimSpace(string(app.Update.Kind))
	version := normalizeCheckKeyValue(app.Version)
	sha1 := normalizeCheckKeyValue(app.SHA1)

	switch app.Update.Kind {
	case models.UpdateZsync:
		if app.Update.Zsync == nil {
			return fmt.Sprintf("zsync:missing:%s:%s", sha1, kind)
		}
		return fmt.Sprintf("zsync:%s:%s:%s", normalizeCheckKeyValue(app.Update.Zsync.UpdateInfo), normalizeCheckKeyValue(app.Update.Zsync.Transport), sha1)
	case models.UpdateGitHubRelease:
		if app.Update.GitHubRelease == nil {
			return fmt.Sprintf("github:missing:%s", version)
		}
		return fmt.Sprintf("github:%s:%s:%s", normalizeCheckKeyValue(app.Update.GitHubRelease.Repo), normalizeCheckKeyValue(app.Update.GitHubRelease.Asset), version)
	case models.UpdateGitLabRelease:
		if app.Update.GitLabRelease == nil {
			return fmt.Sprintf("gitlab:missing:%s", version)
		}
		return fmt.Sprintf("gitlab:%s:%s:%s", normalizeCheckKeyValue(app.Update.GitLabRelease.Project), normalizeCheckKeyValue(app.Update.GitLabRelease.Asset), version)
	default:
		return fmt.Sprintf("kind:%s:%s:%d", kind, version, fallbackIdx)
	}
}

func normalizeCheckKeyValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func clonePendingManagedUpdateForApp(update *pendingManagedUpdate, app *models.App) *pendingManagedUpdate {
	if update == nil {
		return nil
	}

	clone := *update
	clone.App = app
	return &clone
}

func managedCheckWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}

	const maxWorkers = 4
	if total < maxWorkers {
		return total
	}

	return maxWorkers
}

func flushManagedCheckMetadata(updates []repo.CheckMetadataUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return repo.UpdateCheckMetadataBatch(updates)
}

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

func checkAppUpdate(app *models.App) (*pendingManagedUpdate, error) {
	if app == nil || app.Update == nil || app.Update.Kind == models.UpdateNone {
		return nil, nil
	}

	switch app.Update.Kind {
	case models.UpdateZsync:
		update, err := runZsyncUpdateCheck(app.Update, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateZsync,
			}, nil
		}
		latest := strings.TrimSpace(update.NormalizedVersion)
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateZsync,
			}, nil
		}
		label := "Update available"
		if update.PreRelease {
			label = "Pre-release update available"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       latest,
			ExpectedSHA1: strings.TrimSpace(update.RemoteSHA1),
			FromKind:     models.UpdateZsync,
		}, nil
	case models.UpdateGitHubRelease:
		update, err := runGitHubReleaseUpdateCheck(app.Update, app.Version, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}

		latest := strings.TrimSpace(update.NormalizedVersion)
		if latest == "" {
			latest = strings.TrimSpace(update.TagName)
		}

		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitHubRelease,
			}, nil
		}
		label := "Update available"
		if update.PreRelease {
			label = "Pre-release update available"
		}
		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadUrl,
			Asset:        update.AssetName,
			Label:        label,
			Available:    true,
			Latest:       latest,
			Transport:    update.Transport,
			ZsyncURL:     update.ZsyncURL,
			ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
			FromKind:     models.UpdateGitHubRelease,
		}, nil
	case models.UpdateGitLabRelease:
		update, err := runGitLabReleaseUpdateCheck(app.Update, app.Version, app.SHA1)
		if err != nil {
			return nil, err
		}
		if update == nil {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		latest := strings.TrimSpace(update.NormalizedVersion)
		if latest == "" {
			latest = strings.TrimSpace(update.TagName)
		}
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    latest,
				FromKind:  models.UpdateGitLabRelease,
			}, nil
		}

		return &pendingManagedUpdate{
			App:          app,
			URL:          update.DownloadURL,
			Asset:        update.AssetName,
			Label:        "Update available",
			Available:    true,
			Latest:       latest,
			Transport:    update.Transport,
			ZsyncURL:     update.ZsyncURL,
			ExpectedSHA1: strings.TrimSpace(update.ExpectedSHA1),
			FromKind:     models.UpdateGitLabRelease,
		}, nil
	default:
		return nil, softwareError(fmt.Errorf("unsupported update source for %s: %q. Reconfigure with `aim update --set %s ...`", app.ID, app.Update.Kind, app.ID))
	}
}

func updateCheckMetadata(app *models.App, checked, available bool, latest string) error {
	if app == nil {
		return nil
	}

	lastCheckedAt := util.NowISO()

	if err := repo.UpdateCheckMetadataBatch([]repo.CheckMetadataUpdate{{
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

func displayVersion(value string) string {
	v := strings.TrimSpace(strings.Trim(value, `"'`))
	if v == "" {
		return "unknown"
	}

	if strings.EqualFold(v, "dev") {
		return "dev"
	}

	if normalized := core.NormalizeComparableVersion(v); normalized != "" {
		v = normalized
	}

	v = strings.TrimSpace(v)
	lower := strings.ToLower(v)
	if lower == "" || lower == "n/a" || lower == "na" || lower == "none" || lower == "unknown" || lower == "-" {
		return "unknown"
	}
	if strings.HasPrefix(lower, "v") {
		return v
	}
	return "v" + v
}

func updateDownloadFilename(assetName, downloadURL string) string {
	name := strings.TrimSpace(filepath.Base(assetName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = strings.TrimSpace(filepath.Base(downloadURL))
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "update.AppImage"
	}
	if !util.HasExtension(name, ".AppImage") {
		name = name + ".AppImage"
	}
	return name
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

func isHTTPSURL(value string) bool {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	if strings.TrimSpace(u.Host) == "" {
		return false
	}
	return true
}

func colorize(enabled bool, code, value string) string {
	if !enabled {
		return value
	}

	return code + value + "\033[0m"
}

func printSuccess(cmd *cobra.Command, text string) {
	if runtimeOptionsFrom(cmd).Quiet || !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;32m", text))
}

func printWarning(cmd *cobra.Command, text string) {
	if !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;33m", text))
}

func printError(cmd *cobra.Command, text string) {
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;31m", text))
}

func printInfo(cmd *cobra.Command, text string) {
	if runtimeOptionsFrom(cmd).Quiet || !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;36m", text))
}

func printSection(cmd *cobra.Command, text string) {
	if shouldUseStructuredOutput(cmd) {
		return
	}
	writeDataf(cmd, "%s\n", colorize(shouldColorStdout(cmd), "\033[1m", text))
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
	case models.UpdateGitLabRelease:
		if update.GitLabRelease == nil {
			return "gitlab: <missing>"
		}
		return fmt.Sprintf("gitlab: %s, asset: %s", update.GitLabRelease.Project, update.GitLabRelease.Asset)
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

const maxListNameColumnWidth = 28

func listIDColumnWidth(groups ...[]*models.App) int {
	width := len("ID")
	for _, group := range groups {
		for _, app := range group {
			if app == nil {
				continue
			}
			if l := len(app.ID); l > width {
				width = l
			}
		}
	}

	return width
}

func listNameDisplayWidth(groups ...[]*models.App) int {
	width := len("App Name")
	for _, group := range groups {
		for _, app := range group {
			if app == nil {
				continue
			}
			if l := len([]rune(strings.TrimSpace(app.Name))); l > width {
				width = l
			}
		}
	}
	if width > maxListNameColumnWidth {
		return maxListNameColumnWidth
	}
	return width
}

func formatListRow(app *models.App, idWidth, nameWidth int) string {
	if app == nil {
		return ""
	}

	return fmt.Sprintf(
		"%-*s %-*s %s",
		idWidth,
		app.ID,
		nameWidth,
		truncateForDisplay(app.Name, nameWidth),
		app.Version,
	)
}

func truncateForDisplay(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= width {
		return string(runes)
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}

	return string(runes[:width-3]) + "..."
}
