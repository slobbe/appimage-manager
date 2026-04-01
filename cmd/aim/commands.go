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
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
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

func maybeRunRootUpgradeFlag(ctx context.Context, cmd *cobra.Command, args []string) (bool, error) {
	if cmd == nil || cmd.Name() != "aim" || len(args) == 0 {
		return false, nil
	}

	switch args[0] {
	case "--upgrade", "-U":
		if len(args) != 1 {
			return true, usageError(fmt.Errorf("--upgrade does not accept positional arguments"))
		}
		if err := prepareRuntime(cmd); err != nil {
			return true, err
		}
		if err := mustEnsureRuntimeDirs(); err != nil {
			return true, err
		}
		return true, runUpgrade(ctx, cmd)
	default:
		return false, nil
	}
}

func UpgradeCmd(cmd *cobra.Command, args []string) error {
	_ = args
	return runUpgrade(cmd.Context(), cmd)
}

func runUpgrade(ctx context.Context, cmd *cobra.Command) error {
	if ctx == nil {
		ctx = context.Background()
	}

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

func printAimMetadata(cmd *cobra.Command) error {
	result := map[string]string{
		"version":    version,
		"repository": rootCommandRepositoryURL,
		"license":    rootCommandLicense,
		"issues":     rootCommandIssuesURL,
		"author":     rootCommandAuthor,
		"copyright":  rootCommandCopyright,
	}

	if runtimeOptionsFrom(cmd).JSON {
		return printJSONSuccess(cmd, result)
	}
	if runtimeOptionsFrom(cmd).CSV {
		return usageError(fmt.Errorf("--csv is not supported for `aim`"))
	}

	writeDataf(cmd, "Version: %s\n", version)
	writeDataf(cmd, "Repository: %s\n", rootCommandRepositoryURL)
	writeDataf(cmd, "License: %s\n", rootCommandLicense)
	writeDataf(cmd, "Issues: %s\n", rootCommandIssuesURL)
	writeDataf(cmd, "Author: %s\n", rootCommandAuthor)
	writeDataf(cmd, "Copyright: %s\n", rootCommandCopyright)
	return nil
}

func MigrateCmd(cmd *cobra.Command, args []string) error {
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
		changed, err := runWithBusyIndicator(cmd, progressMigrateApps(), func() (bool, error) {
			return migrateAllApps()
		})
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
			return err
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

	changed, err := runWithBusyIndicator(cmd, progressMigrateApp(id), func() (bool, error) {
		return migrateSingleApp(id)
	})
	if err != nil {
		return err
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
	if addCommandNeedsInput(cmd, args) {
		return printConciseHelpError(cmd, "missing required argument <https-url|github-url|gitlab-url|id|Path/To.AppImage>")
	}

	if ref, ok, err := resolveAddProviderRef(cmd, args); err != nil {
		return err
	} else if ok {
		return runInstallPackageRef(cmd.Context(), cmd, ref)
	}

	input, err := commandSingleArg(args, "<https-url|github-url|gitlab-url|id|Path/To.AppImage>")
	if err != nil {
		return err
	}

	if isRemoteAddInput(input) {
		return runInstallTarget(cmd.Context(), cmd, input)
	}

	if _, err := resolveIntegrateTarget(input); err == nil {
		if err := validateAddIntegrateFlags(cmd); err != nil {
			return err
		}
		return runIntegrateTarget(cmd.Context(), cmd, input)
	}

	return usageError(fmt.Errorf("unknown add target %q; expected https://..., GitHub/GitLab repo URL, <id>, or <Path/To.AppImage>", input))
}

func resolveAddProviderRef(cmd *cobra.Command, args []string) (discovery.PackageRef, bool, error) {
	return resolveProviderFlagRef(cmd, args, "add")
}

func resolveInfoProviderRef(cmd *cobra.Command, args []string) (discovery.PackageRef, bool, error) {
	return resolveProviderFlagRef(cmd, args, "info")
}

func resolveProviderFlagRef(cmd *cobra.Command, args []string, cmdName string) (discovery.PackageRef, bool, error) {
	githubValue, err := flagString(cmd, "github")
	if err != nil {
		return discovery.PackageRef{}, false, err
	}
	gitlabValue, err := flagString(cmd, "gitlab")
	if err != nil {
		return discovery.PackageRef{}, false, err
	}

	hasGitHub := strings.TrimSpace(githubValue) != ""
	hasGitLab := strings.TrimSpace(gitlabValue) != ""

	if !hasGitHub && !hasGitLab {
		return discovery.PackageRef{}, false, nil
	}
	if hasGitHub && hasGitLab {
		return discovery.PackageRef{}, false, usageError(fmt.Errorf("--github and --gitlab are mutually exclusive"))
	}
	if len(args) > 0 {
		return discovery.PackageRef{}, false, usageError(fmt.Errorf("when using --github or --gitlab, do not pass a positional target"))
	}

	if hasGitHub {
		ref, err := discovery.ParseGitHubRepoValue(githubValue)
		return ref, true, usageError(err)
	}

	ref, err := discovery.ParseGitLabProjectValue(gitlabValue)
	return ref, true, usageError(err)
}

func isLegacyPackageRef(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "github:") || strings.HasPrefix(trimmed, "gitlab:")
}

func legacyProviderRefGuidance(cmdName, input string) error {
	trimmed := strings.TrimSpace(input)
	switch {
	case strings.HasPrefix(trimmed, "github:"):
		return usageError(fmt.Errorf("github:... refs are no longer accepted; use 'aim %s --github owner/repo' or a GitHub repo URL", cmdName))
	case strings.HasPrefix(trimmed, "gitlab:"):
		return usageError(fmt.Errorf("gitlab:... refs are no longer accepted; use 'aim %s --gitlab namespace/project' or a GitLab project URL", cmdName))
	default:
		return usageError(fmt.Errorf("unsupported provider ref %q", input))
	}
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
		return nil, usageError(fmt.Errorf("direct URLs must use https; use 'aim add https://...'"))
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
		return nil, usageError(fmt.Errorf("missing required argument <ref>"))
	}

	if isLegacyPackageRef(trimmed) {
		return nil, legacyProviderRefGuidance("add", trimmed)
	}

	if ref, err := discovery.ParsePackageRefURL(trimmed); err == nil {
		switch ref.Kind {
		case discovery.ProviderGitHub:
			return &installTarget{Kind: installTargetGitHub, Repo: ref.ProviderRef}, nil
		case discovery.ProviderGitLab:
			return &installTarget{Kind: installTargetGitLab, Project: ref.ProviderRef}, nil
		}
	}

	if isHTTPSURL(trimmed) {
		return &installTarget{Kind: installTargetDirectURL, URL: trimmed}, nil
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, usageError(fmt.Errorf("direct URLs must use https"))
	}

	if app, err := repo.GetApp(trimmed); err == nil && app != nil {
		return nil, usageError(fmt.Errorf("managed app IDs are added with 'aim add <id>'"))
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return nil, usageError(fmt.Errorf("local AppImages are added with 'aim add <Path/To.AppImage>'"))
	}

	return nil, usageError(fmt.Errorf("unknown add target %s", input))
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

func integrateFromGitHubRelease(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string) (*models.App, error) {
	if strings.TrimSpace(assetPattern) == "" {
		assetPattern = defaultReleaseAssetPattern
	}

	release, err := runWithBusyIndicator(cmd, fmt.Sprintf("Resolving GitHub release for %s", target.Repo), func() (*core.GitHubReleaseAsset, error) {
		return resolveGitHubReleaseAsset(target.Repo, assetPattern)
	})
	if err != nil {
		return nil, err
	}

	return integrateGitHubReleaseAsset(ctx, cmd, target, assetPattern, release)
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

func integrateFromGitLabRelease(ctx context.Context, cmd *cobra.Command, target *installTarget, assetPattern string) (*models.App, error) {
	if strings.TrimSpace(assetPattern) == "" {
		assetPattern = defaultReleaseAssetPattern
	}

	release, err := runWithBusyIndicator(cmd, fmt.Sprintf("Resolving GitLab release for %s", target.Project), func() (*core.GitLabReleaseAsset, error) {
		return resolveGitLabReleaseAsset(target.Project, assetPattern)
	})
	if err != nil {
		return nil, err
	}

	return integrateGitLabReleaseAsset(ctx, cmd, target, assetPattern, release)
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
	tempDir, err := os.MkdirTemp(config.TempDir, "aim-install-*")
	if err != nil {
		return nil, wrapWriteError(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(req.AssetName, req.DownloadURL)
	downloadPath := filepath.Join(tempDir, fileName)

	printInfo(cmd, fmt.Sprintf("Downloading %s", strings.TrimSpace(req.DisplayLabel)))
	if err := downloadRemoteAsset(ctx, req.DownloadURL, downloadPath, isTerminalStderr()); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.ExpectedSHA256) != "" {
		printInfo(cmd, fmt.Sprintf("Verifying %s", fileName))
		if err := verifyDownloadedUpdate(downloadPath, pendingManagedUpdate{ExpectedSHA256: req.ExpectedSHA256}); err != nil {
			return nil, err
		}
	}

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

	if err := addSingleApp(app, true); err != nil {
		return nil, wrapWriteError(err)
	}

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
	if len(nonFlagCommandTokens(args)) == 0 {
		return printConciseHelpError(cmd, "missing required argument <id>")
	}

	id, err := commandSingleArg(args, "<id>")
	if err != nil {
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
			return wrapDatabaseReadError(err)
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

	app, err := removeManagedApp(cmd.Context(), id, unlink)

	if err == nil {
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
	}

	return err
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
	if infoCommandNeedsInput(cmd, args) {
		return printConciseHelpError(cmd, "missing required argument <target>")
	}

	if ref, ok, err := resolveInfoProviderRef(cmd, args); err != nil {
		return err
	} else if ok {
		return runShowPackageRef(cmd.Context(), cmd, ref)
	}

	input, err := commandSingleArg(args, "<target>")
	if err != nil {
		return err
	}

	if ref, err := resolvePackageRefInput(input); err == nil {
		return runShowPackageRef(cmd.Context(), cmd, ref)
	}

	if _, err := resolveInspectTarget(input); err == nil {
		return runInspectTarget(cmd.Context(), cmd, input)
	}

	return usageError(fmt.Errorf("unknown info target %q; expected GitHub/GitLab repo URL, <id>, or <Path/To.AppImage>", input))
}

func resolvePackageMetadataFromRef(ctx context.Context, ref discovery.PackageRef, assetOverride string) (*discovery.PackageMetadata, error) {
	backend, err := backendForRef(ref)
	if err != nil {
		return nil, err
	}

	metadata, err := backend.Resolve(ctx, ref, assetOverride)
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, unavailableError(fmt.Errorf("failed to resolve package metadata for %s", discovery.FormatPackageRef(ref)))
	}

	return metadata, nil
}

func resolvePackageMetadataFromInput(ctx context.Context, input, assetOverride string) (*discovery.PackageMetadata, error) {
	ref, err := resolvePackageRefInput(input)
	if err != nil {
		return nil, err
	}

	return resolvePackageMetadataFromRef(ctx, ref, assetOverride)
}

func resolvePackageRefInput(input string) (discovery.PackageRef, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return discovery.PackageRef{}, usageError(fmt.Errorf("missing package ref"))
	}

	if isLegacyPackageRef(trimmed) {
		return discovery.PackageRef{}, legacyProviderRefGuidance("info", trimmed)
	}

	return discovery.ParsePackageRefURL(trimmed)
}

func backendForRef(ref discovery.PackageRef) (discovery.DiscoveryBackend, error) {
	for _, backend := range discoveryBackends() {
		switch {
		case ref.Kind == discovery.ProviderGitHub && strings.EqualFold(backend.Name(), "GitHub"):
			return backend, nil
		case ref.Kind == discovery.ProviderGitLab && strings.EqualFold(backend.Name(), "GitLab"):
			return backend, nil
		}
	}

	return nil, unavailableError(fmt.Errorf("no discovery backend available for %s", discovery.FormatPackageRef(ref)))
}

func installPackageMetadata(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	if metadata == nil {
		return nil, softwareError(fmt.Errorf("package metadata cannot be empty"))
	}

	switch metadata.Ref.Kind {
	case discovery.ProviderGitHub:
		return installResolvedGitHubPackage(ctx, cmd, metadata)
	case discovery.ProviderGitLab:
		return installResolvedGitLabPackage(ctx, cmd, metadata)
	default:
		return nil, softwareError(fmt.Errorf("unsupported add provider %q", metadata.Ref.Kind))
	}
}

func installResolvedGitHubPackage(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	release := &core.GitHubReleaseAsset{
		DownloadURL:       strings.TrimSpace(metadata.DownloadURL),
		TagName:           strings.TrimSpace(metadata.ReleaseTag),
		NormalizedVersion: strings.TrimSpace(strings.TrimPrefix(metadata.LatestVersion, "v")),
		AssetName:         strings.TrimSpace(metadata.AssetName),
	}
	target := &installTarget{Kind: installTargetGitHub, Repo: metadata.Ref.ProviderRef}
	return integrateGitHubReleaseAsset(ctx, cmd, target, metadata.AssetPattern, release)
}

func installResolvedGitLabPackage(ctx context.Context, cmd *cobra.Command, metadata *discovery.PackageMetadata) (*models.App, error) {
	release := &core.GitLabReleaseAsset{
		DownloadURL:       strings.TrimSpace(metadata.DownloadURL),
		TagName:           strings.TrimSpace(metadata.ReleaseTag),
		NormalizedVersion: strings.TrimSpace(strings.TrimPrefix(metadata.LatestVersion, "v")),
		AssetName:         strings.TrimSpace(metadata.AssetName),
	}
	target := &installTarget{Kind: installTargetGitLab, Project: metadata.Ref.ProviderRef}
	return integrateGitLabReleaseAsset(ctx, cmd, target, metadata.AssetPattern, release)
}

func printPackageMetadata(cmd *cobra.Command, metadata *discovery.PackageMetadata) {
	printSection(cmd, metadata.Name)
	writeDataf(cmd, "Provider: %s\n", strings.TrimSpace(metadata.Provider))
	if providerRef := formatProviderRef(metadata.Ref); providerRef != "" {
		writeDataf(cmd, "Provider ref: %s\n", providerRef)
	}
	if strings.TrimSpace(metadata.RepoURL) != "" {
		writeDataf(cmd, "Source URL: %s\n", strings.TrimSpace(metadata.RepoURL))
	}
	if strings.TrimSpace(metadata.Summary) != "" {
		writeDataf(cmd, "Summary: %s\n", strings.TrimSpace(metadata.Summary))
	}

	installable := strings.TrimSpace(metadata.InstallReason) == "" && metadata.Installable
	writeDataf(cmd, "Installable: %s\n", yesNo(installable))

	if !installable && strings.TrimSpace(metadata.InstallReason) != "" {
		writeDataf(cmd, "Reason: %s\n", strings.TrimSpace(metadata.InstallReason))
		return
	}

	if strings.TrimSpace(metadata.LatestVersion) != "" {
		writeDataf(cmd, "Latest release: %s\n", displayVersion(metadata.LatestVersion))
	}
	if strings.TrimSpace(metadata.AssetName) != "" {
		writeDataf(cmd, "Selected asset: %s\n", strings.TrimSpace(metadata.AssetName))
	}
	writeDataf(cmd, "Managed updates: yes\n")

	printSection(cmd, "Install Command")
	writeDataf(cmd, "  %s\n", formatAddProviderCommand(metadata.Ref))
}

func formatProviderRef(ref discovery.PackageRef) string {
	value := strings.TrimSpace(ref.ProviderRef)
	if value == "" {
		return ""
	}

	switch ref.Kind {
	case discovery.ProviderGitHub:
		return "GitHub " + value
	case discovery.ProviderGitLab:
		return "GitLab " + value
	default:
		return value
	}
}

func installTargetLabel(target *installTarget) string {
	if target == nil {
		return "package"
	}

	switch target.Kind {
	case installTargetGitHub:
		if value := strings.TrimSpace(target.Repo); value != "" {
			return "GitHub " + value
		}
	case installTargetGitLab:
		if value := strings.TrimSpace(target.Project); value != "" {
			return "GitLab " + value
		}
	case installTargetDirectURL:
		if value := strings.TrimSpace(target.URL); value != "" {
			return value
		}
	}

	return "package"
}

func formatAddProviderCommand(ref discovery.PackageRef) string {
	value := strings.TrimSpace(ref.ProviderRef)
	if value == "" {
		return "aim add"
	}

	switch ref.Kind {
	case discovery.ProviderGitHub:
		return "aim add --github " + value
	case discovery.ProviderGitLab:
		return "aim add --gitlab " + value
	default:
		return "aim add"
	}
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

func runShowTarget(ctx context.Context, cmd *cobra.Command, refArg string) error {
	ref, err := resolvePackageRefInput(refArg)
	if err != nil {
		return err
	}

	return runShowPackageRef(ctx, cmd, ref)
}

func runShowPackageRef(ctx context.Context, cmd *cobra.Command, ref discovery.PackageRef) error {
	metadata, err := runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", formatProviderRef(ref)), func() (*discovery.PackageMetadata, error) {
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
		metadata, err = runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", installTargetLabel(target)), func() (*discovery.PackageMetadata, error) {
			return resolvePackageMetadataFromInput(ctx, refArg, assetPattern)
		})
		if err == nil && !metadata.Installable {
			err = usageError(fmt.Errorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason)))
		}
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
		metadata, err := runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", formatProviderRef(ref)), func() (*discovery.PackageMetadata, error) {
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

	metadata, err := runWithBusyIndicator(cmd, fmt.Sprintf("Resolving package metadata for %s", formatProviderRef(ref)), func() (*discovery.PackageMetadata, error) {
		return resolvePackageMetadataFromRef(ctx, ref, assetPattern)
	})
	if err == nil && !metadata.Installable {
		err = usageError(fmt.Errorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason)))
	}
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

func isRemoteAddInput(input string) bool {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(trimmed, "https://") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(trimmed, "github:") ||
		strings.HasPrefix(trimmed, "gitlab:")
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

	return nil, usageError(fmt.Errorf("unknown inspect target %s", input))
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
	if hasUpdateSetFlags(cmd) {
		return usageError(fmt.Errorf("update source flags can only be used with `aim update set`"))
	}

	targetID := ""
	if len(args) > 0 {
		targetID = args[0]
	}
	if len(args) > 1 {
		return usageError(fmt.Errorf("too many arguments"))
	}

	return runManagedUpdate(cmd.Context(), cmd, targetID)
}

func UpdateSetCmd(cmd *cobra.Command, args []string) error {
	if flagChanged(cmd, "check-only") {
		return usageError(fmt.Errorf("flag --check-only/-c is not supported with `aim update set`"))
	}
	if len(nonFlagCommandTokens(args)) == 0 {
		return printConciseHelpError(cmd, "missing required argument <id>")
	}
	opts := runtimeOptionsFrom(cmd)

	id, err := commandSingleArg(args, "<id>")
	if err != nil {
		return err
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return wrapDatabaseReadError(err)
	}

	var incomingSource *models.UpdateSource
	embedded, err := flagBool(cmd, "embedded")
	if err != nil {
		return err
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
	} else {
		incomingSource, err = resolveUpdateSourceFromSetFlags(cmd)
		if err != nil {
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

	app.Update = incomingSource

	if err := repo.UpdateApp(app); err != nil {
		return wrapWriteError(err)
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

func UpdateUnsetCmd(cmd *cobra.Command, args []string) error {
	if flagChanged(cmd, "check-only") {
		return usageError(fmt.Errorf("flag --check-only/-c is not supported with `aim update unset`"))
	}
	if hasUpdateSetFlags(cmd) {
		return usageError(fmt.Errorf("update source flags are not supported with `aim update unset`"))
	}
	if len(nonFlagCommandTokens(args)) == 0 {
		return printConciseHelpError(cmd, "missing required argument <id>")
	}

	id, err := commandSingleArg(args, "<id>")
	if err != nil {
		return err
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return wrapDatabaseReadError(err)
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
	_, err = unsetManagedUpdateSource(cmd, app, prompt, true)
	return err
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
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitHubRelease,
			GitHubRelease: &models.GitHubReleaseUpdateSource{
				Repo:  githubRepo,
				Asset: assetPattern,
			},
		}, nil
	}

	if gitlabProject != "" {
		if assetPattern == "" {
			assetPattern = defaultReleaseAssetPattern
		}
		return &models.UpdateSource{
			Kind: models.UpdateGitLabRelease,
			GitLabRelease: &models.GitLabReleaseUpdateSource{
				Project: gitlabProject,
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
	var pending []pendingManagedUpdate
	checkFailures := 0
	singleStatusPrinted := false
	checkResults := runManagedChecks(apps)
	metadataUpdates := make([]repo.CheckMetadataUpdate, 0, len(checkResults))
	rows := make([]updateOutputRow, 0, len(checkResults))
	rowIndexByID := map[string]int{}
	checkedAt := util.NowISO()

	for _, result := range checkResults {
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
				return tempFailError(fmt.Errorf("failed to check updates for %s: %w", app.ID, err))
			}

			checkFailures++
			printError(cmd, fmt.Sprintf("Failed to check updates for %s: %v", app.ID, err))
			continue
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
		printError(cmd, fmt.Sprintf("Failed to persist update state: %v", err))
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

	applyResults := runManagedApplies(ctx, cmd, pending)
	applyFailures := 0
	appliedApps := make([]*models.App, 0, len(applyResults))
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

	persistErr := persistManagedAppliedApps(appliedApps)

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
			return nil, wrapDatabaseReadError(err)
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
		return nil, softwareError(fmt.Errorf("unsupported update source for %s: %q. Reconfigure with `aim update set`", app.ID, app.Update.Kind))
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
		err := softwareError(fmt.Errorf("missing download URL"))
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "aim-update-*")
	if err != nil {
		err = wrapWriteError(err)
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(update.Asset, update.URL)
	downloadPath := filepath.Join(tempDir, fileName)
	usedZsync := false
	if strings.TrimSpace(update.ZsyncURL) != "" {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageZsync})
		if err := applyZsyncUpdate(ctx, update, downloadPath); err == nil {
			usedZsync = true
		}
	}

	if !usedZsync {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageDownload})
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
	if err := verifyDownloadedUpdate(downloadPath, update); err != nil {
		emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	emitManagedApplyEvent(reporter, managedApplyEvent{Stage: managedApplyStageIntegrate})
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
	return app, nil
}

func applyZsyncUpdate(ctx context.Context, update pendingManagedUpdate, destination string) error {
	if update.App == nil {
		return softwareError(fmt.Errorf("missing app"))
	}
	if strings.TrimSpace(update.App.ExecPath) == "" {
		return notFoundError(fmt.Errorf("missing app exec path"))
	}
	if strings.TrimSpace(update.ZsyncURL) == "" {
		return unavailableError(fmt.Errorf("missing zsync url"))
	}

	binary, err := zsyncLookPath("zsync")
	if err != nil {
		return unavailableError(err)
	}

	cmd := zsyncCommandContext(ctx, binary, "-q", "-i", update.App.ExecPath, "-o", destination, update.ZsyncURL)
	cmd.Dir = filepath.Dir(destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return unavailableError(err)
		}
		return unavailableError(fmt.Errorf("%w: %s", err, msg))
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
			return unavailableError(fmt.Errorf("downloaded file sha256 mismatch"))
		}
		if strings.ToLower(sha1sum) != expectedSHA1 {
			return unavailableError(fmt.Errorf("downloaded file sha1 mismatch"))
		}
		return nil
	}

	if expectedSHA256 != "" {
		sum, err := util.Sha256File(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return unavailableError(fmt.Errorf("downloaded file sha256 mismatch"))
		}
	}

	if expectedSHA1 != "" {
		sum, err := util.Sha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA1 {
			return unavailableError(fmt.Errorf("downloaded file sha1 mismatch"))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return unavailableError(fmt.Errorf("download failed with status %s", resp.Status))
	}

	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return wrapWriteError(err)
	}
	defer f.Close()

	var (
		total      = resp.ContentLength
		downloaded int64
		buffer     = make([]byte, 32*1024)
		frame      int
		lastDraw   time.Time
	)

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
		}

		if interactive {
			now := time.Now()
			if now.Sub(lastDraw) >= 120*time.Millisecond || readErr == io.EOF {
				line := buildDownloadProgressLine(downloaded, total, frame)
				writeProcessLogf("\r%s", line)
				lastDraw = now
				frame++
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	if interactive {
		line := buildDownloadProgressLine(downloaded, total, frame)
		writeProcessLogf("\r%s\n", line)
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

func buildDownloadProgressLine(downloaded, total int64, frame int) string {
	if total > 0 {
		percent := float64(downloaded) / float64(total)
		if percent < 0 {
			percent = 0
		}
		if percent > 1 {
			percent = 1
		}

		barWidth := 24
		filled := int(percent * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
		return fmt.Sprintf("  Downloading [%s] %6.2f%% (%s/%s)", bar, percent*100, formatByteSize(downloaded), formatByteSize(total))
	}

	spinnerFrames := []string{"|", "/", "-", "\\"}
	frameLabel := spinnerFrames[frame%len(spinnerFrames)]
	return fmt.Sprintf("  Downloading %s %s", frameLabel, formatByteSize(downloaded))
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
