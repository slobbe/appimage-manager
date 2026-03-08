package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/slobbe/appimage-manager/internal/core"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/urfave/cli/v3"
)

func RootCmd(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	return cli.ShowRootCommandHelp(cmd)
}

func UpgradeCmd(ctx context.Context, cmd *cli.Command) error {
	result, err := runSelfUpgrade(ctx, version)
	if err != nil {
		return err
	}

	if result.Updated {
		printSuccess(cmd, fmt.Sprintf("Updated aim %s -> %s", displayVersion(result.CurrentVersion), displayVersion(result.LatestVersion)))
		return nil
	}

	printSuccess(cmd, fmt.Sprintf("aim is up to date (%s)", displayVersion(result.CurrentVersion)))
	return nil
}

func AddCmd(ctx context.Context, cmd *cli.Command) error {
	input := cmd.StringArg("app")
	target, err := resolveAddTarget(input)
	if err != nil {
		return err
	}
	if err := validateAddTargetFlags(cmd, target); err != nil {
		return err
	}

	var app *models.App

	switch target.Kind {
	case addTargetIntegrated:
		app = target.App
		printSuccess(cmd, fmt.Sprintf("Already integrated: %s", formatAppRef(app)))
	case addTargetUnlinked:
		appData, err := integrateExistingApp(ctx, target.App.ID)
		if err != nil {
			return err
		}
		app = appData
		printSuccess(cmd, fmt.Sprintf("Reintegrated: %s", formatAppRef(app)))
	case addTargetLocalFile:
		inputLabel := strings.TrimSpace(filepath.Base(target.LocalPath))
		if inputLabel == "" || inputLabel == "." || inputLabel == string(filepath.Separator) {
			inputLabel = strings.TrimSpace(target.LocalPath)
		}

		printInfo(cmd, fmt.Sprintf("Integrating %s", inputLabel))

		appData, err := integrateLocalApp(ctx, target.LocalPath, func(existing, incoming *models.UpdateSource) (bool, error) {
			fmt.Println("Current update source:")
			fmt.Println("  " + updateSummary(existing))
			fmt.Println("Incoming AppImage update info:")
			fmt.Println("  " + updateSummary(incoming))
			prompt := fmt.Sprintf("Replace update source %s with AppImage update info? [y/N]: ", existing.Kind)
			return confirmOverwrite(prompt)
		})
		if err != nil {
			return err
		}
		app = appData
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppRef(app)))
	case addTargetDirectURL:
		appData, err := integrateFromDirectURL(ctx, cmd, target, strings.TrimSpace(cmd.String("sha256")))
		if err != nil {
			return err
		}
		app = appData
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppRef(app)))
	case addTargetGitHub:
		appData, err := integrateFromGitHubRelease(ctx, cmd, target, strings.TrimSpace(cmd.String("asset")))
		if err != nil {
			return err
		}
		app = appData
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppRef(app)))
	case addTargetGitLab:
		appData, err := integrateFromGitLabRelease(ctx, cmd, target, strings.TrimSpace(cmd.String("asset")))
		if err != nil {
			return err
		}
		app = appData
		printSuccess(cmd, fmt.Sprintf("Integrated: %s", formatAppRef(app)))
	default:
		return fmt.Errorf("unknown argument %s", input)
	}

	if app.Update != nil && app.Update.Kind == models.UpdateZsync && cmd.Bool("post-check") {
		update, err := runZsyncUpdateCheck(app.Update, app.SHA1)

		if update != nil && update.Available {
			latest := ""
			if update.PreRelease {
				printWarning(cmd, fmt.Sprintf("Pre-release update available: %s %s", app.Name, formatVersionTransition(app.Version, latest)))
			} else {
				printWarning(cmd, fmt.Sprintf("Update available: %s %s", app.Name, formatVersionTransition(app.Version, latest)))
			}
			if strings.TrimSpace(update.DownloadUrl) != "" {
				fmt.Printf("  Download: %s\n", strings.TrimSpace(update.DownloadUrl))
			}
			fmt.Printf("  Reintegrate with: %s\n", integrationHint(update.AssetName))
		}

		return err
	}

	return nil
}

type addTargetKind string

const (
	addTargetLocalFile  addTargetKind = "local_file"
	addTargetUnlinked   addTargetKind = "unlinked"
	addTargetIntegrated addTargetKind = "integrated"
	addTargetDirectURL  addTargetKind = "direct_url"
	addTargetGitHub     addTargetKind = "github_release"
	addTargetGitLab     addTargetKind = "gitlab_release"
)

type addTarget struct {
	Kind      addTargetKind
	App       *models.App
	LocalPath string
	URL       string
	Repo      string
	Project   string
}

func resolveAddTarget(input string) (*addTarget, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, fmt.Errorf("missing required argument <app>")
	}

	if strings.HasPrefix(trimmed, "github:") {
		repoSlug := strings.TrimSpace(strings.TrimPrefix(trimmed, "github:"))
		if repoSlug == "" || strings.Count(repoSlug, "/") != 1 {
			return nil, fmt.Errorf("github add source must be in the form github:owner/repo")
		}
		return &addTarget{Kind: addTargetGitHub, Repo: repoSlug}, nil
	}

	if strings.HasPrefix(trimmed, "gitlab:") {
		project := strings.TrimSpace(strings.TrimPrefix(trimmed, "gitlab:"))
		if project == "" || strings.Count(project, "/") < 1 || strings.HasPrefix(project, "/") || strings.HasSuffix(project, "/") {
			return nil, fmt.Errorf("gitlab add source must be in the form gitlab:namespace/project")
		}
		return &addTarget{Kind: addTargetGitLab, Project: project}, nil
	}

	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, fmt.Errorf("direct URLs must use https")
	}

	if isHTTPSURL(trimmed) {
		return &addTarget{Kind: addTargetDirectURL, URL: trimmed}, nil
	}

	if app, err := repo.GetApp(trimmed); err == nil {
		kind := addTargetIntegrated
		if strings.TrimSpace(app.DesktopEntryLink) == "" {
			kind = addTargetUnlinked
		}
		return &addTarget{Kind: kind, App: app}, nil
	}

	if util.HasExtension(trimmed, ".AppImage") {
		return &addTarget{Kind: addTargetLocalFile, LocalPath: trimmed}, nil
	}

	return nil, fmt.Errorf("unknown argument %s", input)
}

func validateAddTargetFlags(cmd *cli.Command, target *addTarget) error {
	if target == nil {
		return fmt.Errorf("missing add target")
	}

	assetPattern := strings.TrimSpace(cmd.String("asset"))
	sha256 := strings.TrimSpace(cmd.String("sha256"))

	switch target.Kind {
	case addTargetGitHub, addTargetGitLab:
		if sha256 != "" {
			return fmt.Errorf("--sha256 is only supported with direct https URLs")
		}
	case addTargetDirectURL:
		if assetPattern != "" {
			return fmt.Errorf("--asset is only supported with github: or gitlab: add sources")
		}
		if sha256 != "" && !isSHA256Hex(sha256) {
			return fmt.Errorf("--sha256 must be a valid 64-character hexadecimal SHA-256")
		}
	default:
		if assetPattern != "" {
			return fmt.Errorf("--asset is only supported with github: or gitlab: add sources")
		}
		if sha256 != "" {
			return fmt.Errorf("--sha256 is only supported with direct https URLs")
		}
	}

	return nil
}

func integrateFromDirectURL(ctx context.Context, cmd *cli.Command, target *addTarget, sha256 string) (*models.App, error) {
	if strings.TrimSpace(sha256) == "" {
		printWarning(cmd, "No SHA-256 provided; skipping checksum verification")
	}

	return integrateRemoteAdd(ctx, cmd, remoteAddRequest{
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

func integrateFromGitHubRelease(ctx context.Context, cmd *cli.Command, target *addTarget, assetPattern string) (*models.App, error) {
	if strings.TrimSpace(assetPattern) == "" {
		assetPattern = defaultReleaseAssetPattern
	}

	printInfo(cmd, fmt.Sprintf("Resolving GitHub release for %s", target.Repo))
	release, err := resolveGitHubReleaseAsset(target.Repo, assetPattern)
	if err != nil {
		return nil, err
	}

	return integrateRemoteAdd(ctx, cmd, remoteAddRequest{
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

func integrateFromGitLabRelease(ctx context.Context, cmd *cli.Command, target *addTarget, assetPattern string) (*models.App, error) {
	if strings.TrimSpace(assetPattern) == "" {
		assetPattern = defaultReleaseAssetPattern
	}

	printInfo(cmd, fmt.Sprintf("Resolving GitLab release for %s", target.Project))
	release, err := resolveGitLabReleaseAsset(target.Project, assetPattern)
	if err != nil {
		return nil, err
	}

	return integrateRemoteAdd(ctx, cmd, remoteAddRequest{
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

type remoteAddRequest struct {
	DisplayLabel   string
	DownloadURL    string
	AssetName      string
	ExpectedSHA256 string
	BuildSource    func(app *models.App) models.Source
	BuildUpdate    func(app *models.App) *models.UpdateSource
}

func integrateRemoteAdd(ctx context.Context, cmd *cli.Command, req remoteAddRequest) (*models.App, error) {
	tempDir, err := os.MkdirTemp(config.TempDir, "aim-add-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(req.AssetName, req.DownloadURL)
	downloadPath := filepath.Join(tempDir, fileName)

	printInfo(cmd, fmt.Sprintf("Downloading %s", strings.TrimSpace(req.DisplayLabel)))
	if err := downloadRemoteAsset(ctx, req.DownloadURL, downloadPath, isTerminalOutput()); err != nil {
		return nil, err
	}

	if strings.TrimSpace(req.ExpectedSHA256) != "" {
		fmt.Println("  Verifying")
		if err := verifyDownloadedUpdate(downloadPath, pendingManagedUpdate{ExpectedSHA256: req.ExpectedSHA256}); err != nil {
			return nil, err
		}
	}

	printInfo(cmd, fmt.Sprintf("Integrating %s", fileName))
	app, err := integrateLocalApp(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		_ = existing
		_ = incoming
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	incomingUpdate := req.BuildUpdate(app)
	finalUpdate, err := chooseRemoteUpdateSource(app.ID, incomingUpdate)
	if err != nil {
		return nil, err
	}

	app.Source = req.BuildSource(app)
	app.Update = finalUpdate

	if err := addSingleApp(app, true); err != nil {
		return nil, err
	}

	return app, nil
}

func chooseRemoteUpdateSource(id string, incoming *models.UpdateSource) (*models.UpdateSource, error) {
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

	fmt.Printf("Current update source for %s:\n", id)
	fmt.Println("  " + updateSummary(existing))
	fmt.Println("New update source:")
	fmt.Println("  " + updateSummary(incoming))
	prompt := fmt.Sprintf("Replace update source for %s? [y/N]: ", id)
	confirmed, err := confirmOverwrite(prompt)
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

func RemoveCmd(ctx context.Context, cmd *cli.Command) error {
	id := cmd.StringArg("id")
	unlink := cmd.Bool("unlink")

	app, err := removeManagedApp(ctx, id, unlink)

	if err == nil {
		label := "Removed"
		if unlink {
			label = "Unlinked"
		}
		printSuccess(cmd, fmt.Sprintf("%s: %s [%s]", label, app.Name, app.ID))
	}

	return err
}

func ListCmd(ctx context.Context, cmd *cli.Command) error {
	all := cmd.Bool("all")
	integrated := cmd.Bool("integrated")
	unlinked := cmd.Bool("unlinked")

	if (integrated && unlinked) || (all && (integrated || unlinked)) {
		return fmt.Errorf("flags --all, --integrated, and --unlinked are mutually exclusive")
	}

	if !all && !integrated && !unlinked {
		all = true
	}

	apps, err := repo.GetAllApps()
	if err != nil {
		return err
	}

	if len(apps) == 0 {
		printSuccess(cmd, "No managed AppImages")
		return nil
	}

	integratedRows := make([]*models.App, 0, len(apps))
	unlinkedRows := make([]*models.App, 0, len(apps))
	for _, app := range apps {
		if len(app.DesktopEntryLink) > 0 {
			integratedRows = append(integratedRows, app)
			continue
		}
		unlinkedRows = append(unlinkedRows, app)
	}

	if integrated && len(integratedRows) == 0 {
		printSuccess(cmd, "No integrated AppImages")
		return nil
	}
	if unlinked && len(unlinkedRows) == 0 {
		printSuccess(cmd, "No unlinked AppImages")
		return nil
	}

	idWidth := listIDColumnWidth(integratedRows, unlinkedRows)
	nameWidth := listNameDisplayWidth(integratedRows, unlinkedRows)
	header := fmt.Sprintf("%-*s %-*s %s", idWidth, "ID", nameWidth, "App Name", "Version")
	printSection(cmd, header)

	if all || integrated {
		for _, app := range integratedRows {
			fmt.Fprintln(os.Stdout, formatListRow(app, idWidth, nameWidth))
		}
	}

	if all || unlinked {
		for _, app := range unlinkedRows {
			row := formatListRow(app, idWidth, nameWidth)
			fmt.Println(colorize(useColor(cmd), "\033[2m\033[3m", row))
		}
	}

	return nil
}

func UpdateCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) > 0 {
		switch args[0] {
		case "set":
			return UpdateSetCmd(ctx, cmd, args[1:])
		case "check":
			return fmt.Errorf("`aim update check` has been removed; use `aim update [<id>]` for managed apps")
		}
	}

	if hasUpdateSetFlags(cmd) {
		return fmt.Errorf("update source flags can only be used with `aim update set`")
	}

	targetID := ""
	if len(args) > 0 {
		targetID = args[0]
	}
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	return runManagedUpdate(ctx, cmd, targetID)
}

func UpdateSetCmd(ctx context.Context, cmd *cli.Command, args []string) error {
	_ = ctx
	if cmd.IsSet("yes") || cmd.IsSet("check-only") {
		return fmt.Errorf("flags --yes/-y and --check-only/-c are not supported with `aim update set`")
	}

	id := ""
	if len(args) > 0 {
		id = strings.TrimSpace(args[0])
	}
	if id == "" {
		return fmt.Errorf("missing required argument <id>")
	}

	incomingSource, err := resolveUpdateSourceFromSetFlags(cmd)
	if err != nil {
		return err
	}

	app, err := repo.GetApp(id)
	if err != nil {
		return err
	}

	if app.Update != nil && app.Update.Kind != models.UpdateNone {
		fmt.Printf("Current update source for %s:\n", id)
		fmt.Println("  " + updateSummary(app.Update))
		fmt.Println("New update source:")
		fmt.Println("  " + updateSummary(incomingSource))
		prompt := fmt.Sprintf("Replace update source for %s? [y/N]: ", id)
		confirmed, err := confirmOverwrite(prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			printWarning(cmd, "Update source unchanged")
			return nil
		}
	}

	app.Update = incomingSource

	if err := repo.UpdateApp(app); err != nil {
		return err
	}

	printSuccess(cmd, fmt.Sprintf("Update source set: %s", updateSummary(incomingSource)))
	return nil
}

func hasUpdateSetFlags(cmd *cli.Command) bool {
	keys := []string{"github", "gitlab", "asset", "zsync-url", "manifest-url", "url", "sha256"}
	for _, key := range keys {
		if cmd.IsSet(key) {
			return true
		}
	}
	return false
}

func resolveUpdateSourceFromSetFlags(cmd *cli.Command) (*models.UpdateSource, error) {
	githubRepo := strings.TrimSpace(cmd.String("github"))
	gitlabProject := strings.TrimSpace(cmd.String("gitlab"))
	assetPattern := strings.TrimSpace(cmd.String("asset"))
	zsyncURL := strings.TrimSpace(cmd.String("zsync-url"))
	manifestURL := strings.TrimSpace(cmd.String("manifest-url"))
	directURL := strings.TrimSpace(cmd.String("url"))
	sha256 := strings.TrimSpace(cmd.String("sha256"))

	if manifestURL != "" {
		return nil, fmt.Errorf("--manifest-url is no longer supported; use --github, --gitlab, or --zsync-url")
	}
	if directURL != "" {
		return nil, fmt.Errorf("--url is no longer supported; use --github, --gitlab, or --zsync-url")
	}
	if sha256 != "" {
		return nil, fmt.Errorf("--sha256 is no longer supported; use --github, --gitlab, or --zsync-url")
	}

	selectorCount := 0
	for _, value := range []string{githubRepo, gitlabProject, zsyncURL} {
		if value != "" {
			selectorCount++
		}
	}

	if selectorCount == 0 {
		return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, or --zsync-url")
	}
	if selectorCount > 1 {
		return nil, fmt.Errorf("update source flags are mutually exclusive")
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
			return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
		}
		if !isHTTPSURL(zsyncURL) {
			return nil, fmt.Errorf("--zsync-url must be a valid https URL")
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
		return nil, fmt.Errorf("--asset is only supported with --github or --gitlab")
	}
	return nil, fmt.Errorf("missing update source; set one of --github, --gitlab, or --zsync-url")
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
var resolveGitHubReleaseAsset = core.ResolveGitHubReleaseAsset
var resolveGitLabReleaseAsset = core.ResolveGitLabReleaseAsset
var downloadRemoteAsset = downloadUpdateAsset
var runSelfUpgrade = core.SelfUpgrade
var integrateExistingApp = core.IntegrateExisting
var integrateLocalApp = core.IntegrateFromLocalFile
var removeManagedApp = core.Remove
var addAppsBatch = repo.AddAppsBatch
var addSingleApp = repo.AddApp

const defaultReleaseAssetPattern = "*.AppImage"

func runManagedUpdate(ctx context.Context, cmd *cli.Command, targetID string) error {
	apps, err := collectManagedUpdateTargets(targetID)
	if err != nil {
		return err
	}

	autoApply := cmd.Bool("yes")
	checkOnly := cmd.Bool("check-only")

	var pending []pendingManagedUpdate
	checkFailures := 0
	singleStatusPrinted := false
	checkResults := runManagedChecks(apps)
	metadataUpdates := make([]repo.CheckMetadataUpdate, 0, len(checkResults))

	for _, result := range checkResults {
		app := result.app
		update := result.update
		err := result.err
		if err != nil {
			metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
				ID:            app.ID,
				Checked:       false,
				Available:     app.UpdateAvailable,
				Latest:        app.LatestVersion,
				LastCheckedAt: util.NowISO(),
			})

			if targetID != "" {
				if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
					return metaErr
				}
				metadataUpdates = metadataUpdates[:0]
				return fmt.Errorf("failed to check updates for %s: %w", app.ID, err)
			}

			checkFailures++
			printError(cmd, fmt.Sprintf("Failed to check updates for %s: %v", app.ID, err))
			continue
		}

		if update == nil {
			if targetID != "" {
				if app.Update == nil || app.Update.Kind == models.UpdateNone {
					printSuccess(cmd, fmt.Sprintf("No update source configured for %s", app.ID))
				} else {
					printSuccess(cmd, fmt.Sprintf("No update information for %s", app.ID))
				}
				singleStatusPrinted = true
			}
			continue
		}

		metadataUpdates = append(metadataUpdates, repo.CheckMetadataUpdate{
			ID:            app.ID,
			Checked:       true,
			Available:     update.Available,
			Latest:        update.Latest,
			LastCheckedAt: util.NowISO(),
		})

		if targetID != "" {
			if metaErr := flushManagedCheckMetadata(metadataUpdates); metaErr != nil {
				return metaErr
			}
			metadataUpdates = metadataUpdates[:0]
		}

		if update.URL == "" {
			if targetID != "" {
				printSuccess(cmd, fmt.Sprintf("Up to date: %s %s", app.ID, displayVersion(app.Version)))
				singleStatusPrinted = true
			}
			continue
		}

		msg := buildManagedUpdateMessage(*update, checkOnly)
		if targetID == "" {
			header := fmt.Sprintf("[%s]", app.ID)
			printSection(cmd, header)
		}
		printWarning(cmd, msg)
		if checkOnly {
			fmt.Printf("  Download: %s\n", update.URL)
			if showManagedUpdateAsset(update.Asset) {
				fmt.Printf("  Asset: %s\n", strings.TrimSpace(update.Asset))
			}
		}

		pending = append(pending, *update)
	}

	if err := flushManagedCheckMetadata(metadataUpdates); err != nil {
		if targetID != "" {
			return err
		}
		checkFailures++
		printError(cmd, fmt.Sprintf("Failed to persist update state: %v", err))
	}

	if len(pending) == 0 {
		if targetID != "" && singleStatusPrinted {
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
		return nil
	}

	if !autoApply {
		prompt := fmt.Sprintf("Apply %d updates? [y/N]: ", len(pending))
		if targetID != "" {
			prompt = fmt.Sprintf("Apply update for %s? [y/N]: ", targetID)
		}

		confirmed, err := confirmOverwrite(prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			printWarning(cmd, "No updates applied")
			return nil
		}
	}

	applyFailures := 0
	applySuccesses := 0
	totalPending := len(pending)
	appliedApps := make([]*models.App, 0, totalPending)
	interactiveProgress := isTerminalOutput()
	for i, item := range pending {
		progress := fmt.Sprintf("Updating %s (%d/%d)", item.App.ID, i+1, totalPending)
		if transition := updateVersionTransition(item); transition != "" {
			progress = fmt.Sprintf("%s %s", progress, transition)
		}
		printInfo(cmd, progress)

		updatedApp, err := applyManagedUpdate(ctx, item, interactiveProgress)
		if err != nil {
			applyFailures++
			printError(cmd, fmt.Sprintf("Failed to update %s: %v", item.App.ID, err))
			continue
		}

		printSuccess(cmd, fmt.Sprintf("Updated: %s %s", updatedApp.ID, displayVersion(updatedApp.Version)))
		applySuccesses++
		appliedApps = append(appliedApps, updatedApp)
	}

	if applySuccesses > 0 {
		core.RefreshDesktopIntegrationCaches(ctx)
	}

	persistErr := persistManagedAppliedApps(appliedApps)

	if applyFailures > 0 {
		if persistErr != nil {
			return fmt.Errorf("%d update(s) failed; failed to persist applied updates: %w", applyFailures, persistErr)
		}
		return fmt.Errorf("%d update(s) failed", applyFailures)
	}

	if persistErr != nil {
		return persistErr
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
		return fmt.Errorf("failed to persist applied updates: %s", strings.Join(fallbackErrors, "; "))
	}

	return nil
}

func collectManagedUpdateTargets(targetID string) ([]*models.App, error) {
	if strings.TrimSpace(targetID) != "" {
		app, err := repo.GetApp(targetID)
		if err != nil {
			return nil, err
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
		if !update.Available {
			return &pendingManagedUpdate{
				App:       app,
				Available: false,
				Latest:    "",
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
			Latest:       "",
			ExpectedSHA1: strings.TrimSpace(update.RemoteSHA1),
			FromKind:     models.UpdateZsync,
		}, nil
	case models.UpdateGitHubRelease:
		update, err := runGitHubReleaseUpdateCheck(app.Update, app.Version)
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
			App:       app,
			URL:       update.DownloadUrl,
			Asset:     update.AssetName,
			Label:     label,
			Available: true,
			Latest:    latest,
			FromKind:  models.UpdateGitHubRelease,
		}, nil
	case models.UpdateGitLabRelease:
		update, err := runGitLabReleaseUpdateCheck(app.Update, app.Version)
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
			App:       app,
			URL:       update.DownloadURL,
			Asset:     update.AssetName,
			Label:     "Update available",
			Available: true,
			Latest:    latest,
			FromKind:  models.UpdateGitLabRelease,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported update source for %s: %q. Reconfigure with `aim update set`", app.ID, app.Update.Kind)
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
		return err
	}

	if checked {
		app.UpdateAvailable = available
		app.LatestVersion = strings.TrimSpace(latest)
	}
	app.LastCheckedAt = lastCheckedAt

	return nil
}

func applyManagedUpdate(ctx context.Context, update pendingManagedUpdate, interactiveProgress bool) (*models.App, error) {
	if strings.TrimSpace(update.URL) == "" {
		return nil, fmt.Errorf("missing download URL")
	}

	tempDir, err := os.MkdirTemp("", "aim-update-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileName := updateDownloadFilename(update.Asset, update.URL)
	downloadPath := filepath.Join(tempDir, fileName)
	fmt.Printf("  Downloading %s\n", fileName)
	if err := downloadUpdateAsset(ctx, update.URL, downloadPath, interactiveProgress); err != nil {
		return nil, err
	}

	fmt.Println("  Verifying")
	if err := verifyDownloadedUpdate(downloadPath, update); err != nil {
		return nil, err
	}

	fmt.Println("  Integrating")
	app, err := core.IntegrateFromLocalFileWithoutCacheRefreshOrPersist(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	return app, nil
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
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
		if strings.ToLower(sha1sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
		return nil
	}

	if expectedSHA256 != "" {
		sum, err := util.Sha256File(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
	}

	if expectedSHA1 != "" {
		sum, err := util.Sha1(downloadPath)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
	}

	return nil
}

func buildManagedUpdateMessage(update pendingManagedUpdate, checkOnly bool) string {
	base := update.Label
	if update.App != nil {
		base = fmt.Sprintf("%s: %s", base, update.App.ID)
		if transition := updateVersionTransition(update); transition != "" {
			base = fmt.Sprintf("%s %s", base, transition)
		}
	} else if transition := updateVersionTransition(update); transition != "" {
		base = fmt.Sprintf("%s %s", base, transition)
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

	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "version") {
		v = strings.TrimSpace(v[len("version"):])
		v = strings.TrimLeft(v, " :=-")
	}

	v = strings.TrimSpace(v)
	lower = strings.ToLower(v)
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
		return fmt.Errorf("download failed with status %s", resp.Status)
	}

	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
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
				return err
			}
			downloaded += int64(n)
		}

		if interactive {
			now := time.Now()
			if now.Sub(lastDraw) >= 120*time.Millisecond || readErr == io.EOF {
				line := buildDownloadProgressLine(downloaded, total, frame)
				fmt.Printf("\r%s", line)
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
		fmt.Printf("\r%s\n", line)
	} else {
		fmt.Printf("  Downloaded %s\n", formatByteSize(downloaded))
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

func useColor(cmd *cli.Command) bool {
	_ = cmd
	return isTerminalOutput()
}

func isTerminalOutput() bool {

	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) != 0
}

func colorize(enabled bool, code, value string) string {
	if !enabled {
		return value
	}

	return code + value + "\033[0m"
}

func printSuccess(cmd *cli.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;32m", text))
}

func printWarning(cmd *cli.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;33m", text))
}

func printError(cmd *cli.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;31m", text))
}

func printInfo(cmd *cli.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[0;36m", text))
}

func printSection(cmd *cli.Command, text string) {
	fmt.Println(colorize(useColor(cmd), "\033[1m", text))
}

func confirmOverwrite(prompt string) (bool, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func integrationHint(assetName string) string {
	if strings.TrimSpace(assetName) == "" {
		return "aim add path/to/new.AppImage"
	}
	return fmt.Sprintf("aim add path/to/%s", assetName)
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
