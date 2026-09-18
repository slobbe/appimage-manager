package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

// service implements application workflows using app-defined ports. It owns
// orchestration only; all filesystem work is delegated to infra adapters.
type service struct {
	config                      Config
	currentVersion              string
	appImages                   AppImageExtractor
	desktopEntries              DesktopEntryDiscoverer
	icons                       IconDiscoverer
	appImageInstaller           AppImageInstaller
	iconInstaller               IconInstaller
	desktopEntryInstaller       DesktopEntryInstaller
	artifactRemover             ArtifactRemover
	artifactBackups             ArtifactBackupManager
	artifactPaths               ArtifactPathInspector
	desktopIntegrationRefresher DesktopIntegrationRefresher
	githubReleases              GitHubReleaseFinder
	downloads                   AssetDownloader
	selfUpdater                 SelfUpdater
	apps                        AppRepository
	mutations                   MutationLocker
}

type ServiceDeps struct {
	Config                      Config
	AppImages                   AppImageExtractor
	DesktopEntries              DesktopEntryDiscoverer
	Icons                       IconDiscoverer
	AppImageInstaller           AppImageInstaller
	IconInstaller               IconInstaller
	DesktopEntryInstaller       DesktopEntryInstaller
	ArtifactRemover             ArtifactRemover
	ArtifactBackups             ArtifactBackupManager
	ArtifactPaths               ArtifactPathInspector
	DesktopIntegrationRefresher DesktopIntegrationRefresher
	GitHubReleases              GitHubReleaseFinder
	Downloads                   AssetDownloader
	SelfUpdater                 SelfUpdater
	CurrentVersion              string
	Apps                        AppRepository
	Mutations                   MutationLocker
}

func NewService(deps ServiceDeps) (Service, error) {
	service := &service{
		config:                      deps.Config,
		currentVersion:              deps.CurrentVersion,
		appImages:                   deps.AppImages,
		desktopEntries:              deps.DesktopEntries,
		icons:                       deps.Icons,
		appImageInstaller:           deps.AppImageInstaller,
		iconInstaller:               deps.IconInstaller,
		desktopEntryInstaller:       deps.DesktopEntryInstaller,
		artifactRemover:             deps.ArtifactRemover,
		artifactBackups:             deps.ArtifactBackups,
		artifactPaths:               deps.ArtifactPaths,
		desktopIntegrationRefresher: deps.DesktopIntegrationRefresher,
		githubReleases:              deps.GitHubReleases,
		downloads:                   deps.Downloads,
		selfUpdater:                 deps.SelfUpdater,
		apps:                        deps.Apps,
		mutations:                   deps.Mutations,
	}
	if err := service.validate(); err != nil {
		return nil, err
	}

	return service, nil
}

var _ Service = (*service)(nil)

func createWorkspace(ctx context.Context) (string, func(), error) {
	if err := ctx.Err(); err != nil {
		return "", func() {}, err
	}

	path, err := os.MkdirTemp("", "aim-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create workspace: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = os.RemoveAll(path)
		return "", func() {}, err
	}

	return path, func() { _ = os.RemoveAll(path) }, nil
}

func (s *service) Add(ctx context.Context, req AddRequest) (AddResult, error) {
	if err := ctx.Err(); err != nil {
		return AddResult{}, err
	}
	unlock, err := s.mutations.Lock(ctx)
	if err != nil {
		return AddResult{}, err
	}
	defer unlock()

	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	if strings.TrimSpace(req.AssetPattern) != "" && strings.TrimSpace(req.GitHubRepo) == "" {
		return AddResult{}, errors.New("asset pattern requires github repo")
	}
	if strings.TrimSpace(req.GitHubRepo) != "" {
		return s.addFromGitHub(ctx, req, activity)
	}
	if req.Path == "" {
		return AddResult{}, errors.New("appimage path is required")
	}

	return s.addLocal(ctx, req, activity)
}

type addLocalOptions struct {
	source          domain.Source
	fallbackVersion string
	appID           string
	saveApp         bool
}

func (s *service) addLocal(ctx context.Context, req AddRequest, activity ActivityReporter) (AddResult, error) {
	return s.addLocalWithOptions(ctx, req, activity, addLocalOptions{
		source:          domain.NewLocalSource(req.Path, time.Now()),
		fallbackVersion: filepath.Base(req.Path),
		saveApp:         true,
	})
}

func (s *service) addLocalWithOptions(ctx context.Context, req AddRequest, activity ActivityReporter, options addLocalOptions) (AddResult, error) {
	task := activity.Start(ctx, Activity{Kind: ActivityKindIntegrating, Path: req.Path, AppID: options.appID})
	result, err := s.integrateLocal(ctx, req, options.source, options.fallbackVersion, options.appID, options.saveApp)
	if err != nil {
		task.Fail(err)
		return AddResult{}, err
	}
	task.Done("Integrated " + result.App.Name)

	return result, nil
}

func (s *service) addFromGitHub(ctx context.Context, req AddRequest, activity ActivityReporter) (AddResult, error) {
	repo := strings.TrimSpace(req.GitHubRepo)
	if !validGitHubRepo(repo) {
		return AddResult{}, errors.New("github repo must be in owner/repo format")
	}
	if strings.TrimSpace(req.Path) != "" {
		return AddResult{}, errors.New("provide either appimage path or github repo, not both")
	}
	if s.githubReleases == nil {
		return AddResult{}, errors.New("github release finder is required")
	}
	if s.downloads == nil {
		return AddResult{}, errors.New("asset downloader is required")
	}

	check := activity.Start(ctx, Activity{Kind: ActivityKindCheckingGitHub, Repo: repo})
	release, err := s.githubReleases.LatestRelease(ctx, repo, req.Prerelease)
	if err != nil {
		check.Fail(err)
		return AddResult{}, err
	}
	check.Done("Checked " + repo)

	var asset GitHubReleaseAsset
	if strings.TrimSpace(req.AssetPattern) != "" {
		asset, err = selectGitHubAppImageAssetMatchingPattern(release, req.AssetPattern)
	} else {
		asset, err = selectGitHubAppImageAsset(release)
	}
	if err != nil {
		return AddResult{}, err
	}

	workspacePath, cleanup, err := createWorkspace(ctx)
	if err != nil {
		return AddResult{}, err
	}
	defer cleanup()

	downloadPath := filepath.Join(workspacePath, filepath.Base(asset.Name))
	download := activity.Start(ctx, Activity{
		Kind:      ActivityKindDownloading,
		Repo:      repo,
		AssetName: asset.Name,
		Total:     asset.SizeBytes,
		Unit:      ActivityUnitBytes,
	})
	downloaded, err := s.downloads.Download(ctx, DownloadSource{
		URL:       asset.DownloadURL,
		FileName:  asset.Name,
		SizeBytes: asset.SizeBytes,
	}, downloadPath, download)
	if err != nil {
		download.Fail(err)
		return AddResult{}, err
	}
	download.Done("Downloaded " + asset.Name)

	integratePath := downloaded.Path
	if integratePath == "" {
		integratePath = downloadPath
	}

	source := domain.NewGitHubReleaseSource(repo, release.TagName, asset.Name, asset.DownloadURL, asset.SizeBytes, time.Now())
	return s.addLocalWithOptions(ctx, AddRequest{
		Path:         integratePath,
		GitHubRepo:   repo,
		AssetPattern: req.AssetPattern,
		Prerelease:   req.Prerelease,
		Activity:     activity,
	}, activity, addLocalOptions{
		source:          source,
		fallbackVersion: release.TagName,
		saveApp:         true,
	})
}

func (s *service) integrateLocal(ctx context.Context, req AddRequest, source domain.Source, fallbackVersion string, appID string, saveApp bool) (AddResult, error) {
	var rollback rollbackStack
	committed := false
	defer func() {
		if !committed {
			rollback.run(ctx)
		}
	}()

	workspacePath, cleanup, err := createWorkspace(ctx)
	if err != nil {
		return AddResult{}, err
	}
	defer cleanup()

	metadata, err := s.inspectLocalAppImageInWorkspace(ctx, req, source, fallbackVersion, appID, integrationSource, workspacePath)
	if err != nil {
		return AddResult{}, err
	}

	provisionalApp := metadata.app
	installedAppImagePath, err := s.appImageInstaller.Install(ctx, req.Path, provisionalApp.ID)
	if err != nil {
		return AddResult{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return s.artifactRemover(ctx, installedAppImagePath)
	})

	installedIconPath, err := s.iconInstaller.Install(ctx, metadata.iconFile.Path, provisionalApp.ID)
	if err != nil {
		return AddResult{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return s.artifactRemover(ctx, installedIconPath)
	})

	updatedDesktopEntry := metadata.desktopEntry.
		WithExec(installedAppImagePath).
		WithIcon(installedIconPath)
	installedDesktopEntryPath, err := s.desktopEntryInstaller.Install(ctx, provisionalApp.ID, updatedDesktopEntry.Bytes())
	if err != nil {
		return AddResult{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return s.artifactRemover(ctx, installedDesktopEntryPath)
	})

	if s.desktopIntegrationRefresher != nil {
		_ = s.desktopIntegrationRefresher.Refresh(ctx)
	}

	finalApp := domain.NewAppFromDesktopEntry(metadata.desktopEntry, domain.AppInput{
		ID:               provisionalApp.ID,
		AppImagePath:     installedAppImagePath,
		DesktopEntryPath: installedDesktopEntryPath,
		IconPath:         installedIconPath,
		Source:           source,
		UpdateSource:     metadata.updateSource,
	})
	if saveApp {
		if err := s.apps.Save(ctx, finalApp); err != nil {
			return AddResult{}, err
		}
	}
	committed = true

	return AddResult{App: finalApp}, nil
}

type localAppImageMetadata struct {
	app          domain.App
	desktopEntry domain.DesktopEntry
	iconFile     IconFile
	updateSource domain.UpdateSource
}

func (s *service) inspectLocalAppImage(ctx context.Context, req AddRequest, source domain.Source, fallbackVersion string, appID string, sourceFunc func(AddRequest, string) domain.UpdateSource) (localAppImageMetadata, error) {
	workspacePath, cleanup, err := createWorkspace(ctx)
	if err != nil {
		return localAppImageMetadata{}, err
	}
	defer cleanup()

	return s.inspectLocalAppImageInWorkspace(ctx, req, source, fallbackVersion, appID, sourceFunc, workspacePath)
}

func (s *service) inspectLocalAppImageInWorkspace(ctx context.Context, req AddRequest, source domain.Source, fallbackVersion string, appID string, sourceFunc func(AddRequest, string) domain.UpdateSource, workspacePath string) (localAppImageMetadata, error) {
	extractionPath, err := s.extractionPath(ctx, req.Path, workspacePath)
	if err != nil {
		return localAppImageMetadata{}, err
	}

	extraction, err := s.appImages.Extract(ctx, extractionPath, filepath.Join(workspacePath, "extract"))
	if err != nil {
		return localAppImageMetadata{}, err
	}

	desktopFile, err := s.desktopEntries.Discover(ctx, extraction.RootDir)
	if err != nil {
		return localAppImageMetadata{}, err
	}

	desktopEntry, err := domain.ParseDesktopEntry(desktopFile.Content)
	if err != nil {
		return localAppImageMetadata{}, err
	}
	desktopEntry = withFallbackVersion(desktopEntry, fallbackVersion)

	iconFile, err := s.icons.Discover(ctx, extraction.RootDir, desktopEntry.Icon)
	if err != nil {
		return localAppImageMetadata{}, err
	}

	updateSource := sourceFunc(req, extraction.UpdateInfo)
	app := domain.NewAppFromDesktopEntry(desktopEntry, domain.AppInput{
		ID:           appID,
		AppImagePath: req.Path,
		Source:       source,
		UpdateSource: updateSource,
	})

	return localAppImageMetadata{
		app:          app,
		desktopEntry: desktopEntry,
		iconFile:     iconFile,
		updateSource: updateSource,
	}, nil
}

func (s *service) Remove(ctx context.Context, req RemoveRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := s.mutations.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if req.Name == "" {
		return errors.New("app name is required")
	}

	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	installedApp, err := s.apps.Find(ctx, req.Name)
	if err != nil {
		return err
	}

	task := activity.Start(ctx, Activity{Kind: ActivityKindRemoving, AppID: installedApp.ID})
	if err := s.removeInstalledApp(ctx, installedApp); err != nil {
		task.Fail(err)
		return err
	}
	task.Done("Removed " + installedApp.Name)

	return nil
}

func (s *service) removeInstalledApp(ctx context.Context, installedApp domain.App) error {
	if err := removeInstalledArtifact(ctx, installedApp.DesktopEntryPath, s.artifactRemover); err != nil {
		return err
	}
	if err := removeInstalledArtifact(ctx, installedApp.IconPath, s.artifactRemover); err != nil {
		return err
	}
	if err := removeInstalledArtifact(ctx, installedApp.AppImagePath, s.artifactRemover); err != nil {
		return err
	}

	return s.apps.Delete(ctx, installedApp.ID)
}

func removeInstalledArtifact(ctx context.Context, path string, remove func(context.Context, string) error) error {
	if path == "" {
		return nil
	}

	return remove(ctx, path)
}

func (s *service) Update(ctx context.Context, req UpdateRequest) (UpdateResult, error) {
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, err
	}
	unlock, err := s.mutations.Lock(ctx)
	if err != nil {
		return UpdateResult{}, err
	}
	defer unlock()

	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	plans, candidates, failures, skipped, checked, err := s.planGitHubUpdates(ctx, req.Target, activity)
	if err != nil {
		return UpdateResult{}, err
	}
	if req.CheckOnly {
		return UpdateResult{
			Applied:  false,
			Checked:  checked,
			Updates:  candidates,
			Failures: failures,
			Skipped:  skipped,
		}, nil
	}
	if len(plans) == 0 {
		return UpdateResult{
			Applied:  false,
			Checked:  checked,
			Failures: failures,
			Skipped:  skipped,
		}, nil
	}

	if req.Confirmation != nil {
		confirmed, err := req.Confirmation.ConfirmUpdates(ctx, candidates)
		if err != nil {
			return UpdateResult{}, err
		}
		if !confirmed {
			return UpdateResult{
				Applied:  false,
				Checked:  checked,
				Updates:  candidates,
				Failures: failures,
				Skipped:  skipped,
			}, nil
		}
	}

	var warnings []OperationWarning
	appliedCount := 0
	for _, plan := range plans {
		applyWarnings, err := s.applyGitHubUpdate(ctx, activity, plan)
		warnings = append(warnings, applyWarnings...)
		if err != nil {
			if ctx.Err() != nil {
				return UpdateResult{}, err
			}
			failures = append(failures, updateFailure(plan.app.ID, err))
			continue
		}
		appliedCount++
	}

	return UpdateResult{
		Applied:      appliedCount > 0,
		Checked:      checked,
		AppliedCount: appliedCount,
		Updates:      candidates,
		Failures:     failures,
		Skipped:      skipped,
		Warnings:     warnings,
	}, nil
}

type githubUpdatePlan struct {
	app     domain.App
	release GitHubRelease
	asset   GitHubReleaseAsset
	version domain.Version
}

func (s *service) planGitHubUpdates(ctx context.Context, target string, activity ActivityReporter) ([]githubUpdatePlan, []UpdateCandidate, []UpdateFailure, []UpdateSkip, int, error) {
	task := activity.Start(ctx, Activity{Kind: ActivityKindCheckingUpdates})
	apps, err := s.updateScope(ctx, target)
	if err != nil {
		task.Fail(err)
		return nil, nil, nil, nil, 0, err
	}

	plans := make([]githubUpdatePlan, 0)
	candidates := make([]UpdateCandidate, 0)
	failures := make([]UpdateFailure, 0)
	skipped := make([]UpdateSkip, 0)
	for _, installedApp := range apps {
		if err := ctx.Err(); err != nil {
			task.Fail(err)
			return nil, nil, nil, nil, 0, err
		}
		if installedApp.UpdateSource.Kind != domain.UpdateSourceKindGitHub {
			skipped = append(skipped, updateSkip(installedApp))
			continue
		}
		if strings.TrimSpace(installedApp.UpdateSource.Repo) == "" {
			skipped = append(skipped, UpdateSkip{
				AppID:      installedApp.ID,
				Reason:     UpdateSkipReasonInvalidSource,
				SourceKind: string(installedApp.UpdateSource.Kind),
			})
			continue
		}
		if s.githubReleases == nil {
			err := errors.New("github release finder is required")
			task.Fail(err)
			return nil, nil, nil, nil, 0, err
		}

		release, err := s.githubReleaseForUpdateSource(ctx, installedApp.UpdateSource)
		if err != nil {
			if ctx.Err() != nil {
				task.Fail(err)
				return nil, nil, nil, nil, 0, err
			}
			failures = append(failures, updateFailure(installedApp.ID, err))
			continue
		}
		asset, err := selectGitHubUpdateAsset(release, installedApp.UpdateSource)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				task.Fail(ctxErr)
				return nil, nil, nil, nil, 0, ctxErr
			}
			failures = append(failures, updateFailure(installedApp.ID, err))
			continue
		}
		version, ok := updateVersion(release, asset)
		if !ok || !installedApp.HasUpdate(version) {
			continue
		}

		plans = append(plans, githubUpdatePlan{app: installedApp, release: release, asset: asset, version: version})
		candidates = append(candidates, UpdateCandidate{
			ID:             installedApp.ID,
			CurrentVersion: installedApp.Version.String(),
			NewVersion:     version.String(),
		})
	}
	task.Done("Checked integrated apps")

	return plans, candidates, failures, skipped, len(apps), nil
}

func updateFailure(appID string, err error) UpdateFailure {
	return UpdateFailure{AppID: appID, Error: err.Error()}
}

func updateSkip(installedApp domain.App) UpdateSkip {
	sourceKind := string(installedApp.UpdateSource.Kind)
	if installedApp.UpdateSource.Kind == domain.UpdateSourceKindUnknown {
		return UpdateSkip{AppID: installedApp.ID, Reason: UpdateSkipReasonNoSource}
	}
	return UpdateSkip{
		AppID:      installedApp.ID,
		Reason:     UpdateSkipReasonUnsupportedSource,
		SourceKind: sourceKind,
	}
}

func (s *service) githubReleaseForUpdateSource(ctx context.Context, source domain.UpdateSource) (GitHubRelease, error) {
	if !source.Embedded || strings.TrimSpace(source.ReleaseTag) == "" {
		return s.githubReleases.LatestRelease(ctx, source.Repo, source.Prerelease)
	}

	switch source.ReleaseTag {
	case "latest":
		return s.githubReleases.LatestRelease(ctx, source.Repo, false)
	case "latest-pre":
		return s.githubReleases.LatestPrerelease(ctx, source.Repo)
	case "latest-all":
		return s.githubReleases.LatestRelease(ctx, source.Repo, true)
	default:
		return s.githubReleases.ReleaseByTag(ctx, source.Repo, source.ReleaseTag)
	}
}

func selectGitHubUpdateAsset(release GitHubRelease, source domain.UpdateSource) (GitHubReleaseAsset, error) {
	if strings.TrimSpace(source.AssetPattern) != "" {
		return selectGitHubAppImageAssetMatchingPattern(release, source.AssetPattern)
	}
	return selectGitHubAppImageAsset(release)
}

func (s *service) updateScope(ctx context.Context, target string) ([]domain.App, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return s.apps.List(ctx)
	}

	installedApp, err := s.apps.Find(ctx, target)
	if err != nil {
		return nil, err
	}
	return []domain.App{installedApp}, nil
}

func (s *service) applyGitHubUpdate(ctx context.Context, activity ActivityReporter, plan githubUpdatePlan) (warnings []OperationWarning, resultErr error) {
	if s.downloads == nil {
		return nil, errors.New("asset downloader is required")
	}

	workspacePath, cleanup, err := createWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	downloadPath := filepath.Join(workspacePath, filepath.Base(plan.asset.Name))
	download := activity.Start(ctx, Activity{
		Kind:      ActivityKindDownloading,
		AppID:     plan.app.ID,
		Repo:      plan.app.UpdateSource.Repo,
		AssetName: plan.asset.Name,
		Total:     plan.asset.SizeBytes,
		Unit:      ActivityUnitBytes,
	})
	downloaded, err := s.downloads.Download(ctx, DownloadSource{
		URL:       plan.asset.DownloadURL,
		FileName:  plan.asset.Name,
		SizeBytes: plan.asset.SizeBytes,
	}, downloadPath, download)
	if err != nil {
		download.Fail(err)
		return nil, err
	}
	download.Done("Downloaded " + plan.asset.Name)

	integratePath := downloaded.Path
	if integratePath == "" {
		integratePath = downloadPath
	}
	source := domain.NewGitHubReleaseSource(plan.app.UpdateSource.Repo, plan.release.TagName, plan.asset.Name, plan.asset.DownloadURL, plan.asset.SizeBytes, time.Now())
	metadata, err := s.inspectLocalAppImageInWorkspace(ctx, AddRequest{
		Path:       integratePath,
		GitHubRepo: plan.app.UpdateSource.Repo,
		Prerelease: plan.app.UpdateSource.Prerelease,
		Activity:   activity,
	}, source, plan.release.TagName, plan.app.ID, integrationSource, workspacePath)
	if err != nil {
		return nil, err
	}

	var rollback rollbackStack
	committed := false
	var backup ArtifactBackup
	defer func() {
		if !committed {
			rollbackErr := rollback.run(ctx)
			resultErr = errors.Join(resultErr, rollbackErr)
			if rollbackErr != nil && backup != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("artifact backup retained at %q", backup.Location()))
				return
			}
		}
		if backup == nil {
			return
		}
		if err := backup.Close(); err != nil {
			if committed {
				warnings = append(warnings, operationWarning(plan.app.ID, "backup-cleanup", err))
				return
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("discard artifact backup: %w", err))
		}
	}()
	destinations, err := s.installationDestinations(metadata.iconFile.Path, plan.app.ID)
	if err != nil {
		return nil, err
	}
	if err := s.preflightReplacement(ctx, destinations, plan.app); err != nil {
		return nil, err
	}
	backupPaths := append([]string{
		plan.app.AppImagePath,
		plan.app.IconPath,
		plan.app.DesktopEntryPath,
	}, destinations...)
	backup, err = s.artifactBackups.Backup(ctx, backupPaths)
	if err != nil {
		return nil, fmt.Errorf("back up installed artifacts for %s: %w", plan.app.ID, err)
	}
	rollback.add(backup.Restore)

	updatedApp, err := s.installUpdate(ctx, integratePath, metadata, source, plan.app.ID, plan.app.UpdateSource, &rollback)
	if err != nil {
		return nil, err
	}

	if err := s.apps.Save(ctx, updatedApp); err != nil {
		return nil, err
	}
	committed = true

	if err := s.removeReplacedArtifacts(ctx, plan.app, updatedApp); err != nil {
		warnings = append(warnings, operationWarning(plan.app.ID, "replaced-artifact-cleanup", err))
	}
	if s.desktopIntegrationRefresher != nil {
		if err := s.desktopIntegrationRefresher.Refresh(ctx); err != nil {
			warnings = append(warnings, operationWarning(plan.app.ID, "desktop-refresh", err))
		}
	}
	return warnings, nil
}

func (s *service) extractionPath(ctx context.Context, sourcePath string, workspacePath string) (string, error) {
	if pathWithin(sourcePath, workspacePath) {
		return sourcePath, nil
	}
	return stageAppImage(ctx, sourcePath, workspacePath)
}

func stageAppImage(ctx context.Context, sourcePath string, workspacePath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("appimage source path is required")
	}
	if strings.TrimSpace(workspacePath) == "" {
		return "", errors.New("appimage staging workspace is required")
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open appimage %q: %w", sourcePath, err)
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return "", fmt.Errorf("stat appimage %q: %w", sourcePath, err)
	}

	destination := filepath.Join(workspacePath, filepath.Base(sourcePath))
	target, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return "", fmt.Errorf("create staged appimage %q: %w", destination, err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return "", fmt.Errorf("copy appimage %q to %q: %w", sourcePath, destination, copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close staged appimage %q: %w", destination, closeErr)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	return destination, nil
}

func pathWithin(path string, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (s *service) installUpdate(ctx context.Context, appImagePath string, metadata localAppImageMetadata, source domain.Source, targetID string, updateSource domain.UpdateSource, rollback *rollbackStack) (domain.App, error) {
	installedAppImagePath, err := s.appImageInstaller.Install(ctx, appImagePath, targetID)
	if err != nil {
		return domain.App{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedAppImagePath, s.artifactRemover)
	})
	installedIconPath, err := s.iconInstaller.Install(ctx, metadata.iconFile.Path, targetID)
	if err != nil {
		return domain.App{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedIconPath, s.artifactRemover)
	})

	updatedDesktopEntry := metadata.desktopEntry.
		WithExec(installedAppImagePath).
		WithIcon(installedIconPath)
	installedDesktopEntryPath, err := s.desktopEntryInstaller.Install(ctx, targetID, updatedDesktopEntry.Bytes())
	if err != nil {
		return domain.App{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedDesktopEntryPath, s.artifactRemover)
	})

	updatedApp := domain.NewAppFromDesktopEntry(metadata.desktopEntry, domain.AppInput{
		ID:               targetID,
		AppImagePath:     installedAppImagePath,
		DesktopEntryPath: installedDesktopEntryPath,
		IconPath:         installedIconPath,
		Source:           source,
		UpdateSource:     updateSource,
	})
	if updatedApp.Version.IsZero() {
		updatedApp.Version = metadata.app.Version
	}
	return updatedApp, nil
}

func addAppRollback(rollback *rollbackStack, s *service, installedApp domain.App) {
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedApp.AppImagePath, s.artifactRemover)
	})
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedApp.IconPath, s.artifactRemover)
	})
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedApp.DesktopEntryPath, s.artifactRemover)
	})
}

func (s *service) SetID(ctx context.Context, req SetIDRequest) (SetIDResult, error) {
	if err := ctx.Err(); err != nil {
		return SetIDResult{}, err
	}
	unlock, err := s.mutations.Lock(ctx)
	if err != nil {
		return SetIDResult{}, err
	}
	defer unlock()
	currentID := strings.TrimSpace(req.CurrentID)
	if currentID == "" {
		return SetIDResult{}, errors.New("current app id is required")
	}
	if req.Auto && strings.TrimSpace(req.NewID) != "" {
		return SetIDResult{}, errors.New("provide either --set or --auto, not both")
	}
	if !req.Auto && strings.TrimSpace(req.NewID) == "" {
		return SetIDResult{}, errors.New("new app id is required unless --auto is used")
	}

	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}
	task := activity.Start(ctx, Activity{Kind: ActivityKindIntegrating, AppID: currentID})

	result, err := s.setID(ctx, req, currentID)
	if err != nil {
		task.Fail(err)
		return SetIDResult{}, err
	}
	if result.Changed {
		task.Done("Updated app ID " + result.PreviousID + " to " + result.ID)
	} else {
		task.Done("App ID already " + result.ID)
	}
	return result, nil
}

func (s *service) setID(ctx context.Context, req SetIDRequest, currentID string) (result SetIDResult, resultErr error) {
	installedApp, err := s.apps.Find(ctx, currentID)
	if err != nil {
		return SetIDResult{}, err
	}
	if strings.TrimSpace(installedApp.AppImagePath) == "" {
		return SetIDResult{}, errors.New("installed appimage path is required")
	}

	metadata, err := s.inspectInstalledAppImageForID(ctx, installedApp.AppImagePath)
	if err != nil {
		return SetIDResult{}, err
	}

	targetID := strings.TrimSpace(req.NewID)
	if req.Auto {
		targetID = metadata.app.ID
	}
	targetID = domain.Slugify(targetID)
	if targetID == "" {
		return SetIDResult{}, errors.New("new app id is required")
	}
	if targetID == installedApp.ID {
		return SetIDResult{PreviousID: installedApp.ID, ID: installedApp.ID, App: installedApp, Changed: false}, nil
	}
	if _, err := s.apps.Find(ctx, targetID); err == nil {
		return SetIDResult{}, fmt.Errorf("app id %q already exists", targetID)
	} else if !errors.Is(err, ErrAppNotFound) {
		return SetIDResult{}, err
	}

	destinations, err := s.installationDestinations(installedApp.IconPath, targetID)
	if err != nil {
		return SetIDResult{}, err
	}
	existing, err := s.artifactBackups.Existing(ctx, destinations)
	if err != nil {
		return SetIDResult{}, fmt.Errorf("inspect destinations for %s: %w", targetID, err)
	}
	if len(existing) > 0 {
		return SetIDResult{}, fmt.Errorf("cannot change app id to %q: destination artifacts already exist", targetID)
	}

	var rollback rollbackStack
	committed := false
	backup, err := s.artifactBackups.Backup(ctx, destinations)
	if err != nil {
		return SetIDResult{}, fmt.Errorf("back up existing artifacts for %s: %w", targetID, err)
	}
	defer func() {
		if !committed {
			rollbackErr := rollback.run(ctx)
			resultErr = errors.Join(resultErr, rollbackErr)
			if rollbackErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("artifact backup retained at %q", backup.Location()))
				return
			}
		}
		if err := backup.Close(); err != nil {
			if committed {
				result.Warnings = append(result.Warnings, operationWarning(targetID, "backup-cleanup", err))
				return
			}
			resultErr = errors.Join(resultErr, fmt.Errorf("discard artifact backup: %w", err))
		}
	}()
	rollback.add(backup.Restore)

	installedAppImagePath, err := s.appImageInstaller.Install(ctx, installedApp.AppImagePath, targetID)
	if err != nil {
		return SetIDResult{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedAppImagePath, s.artifactRemover)
	})

	installedIconPath, err := s.iconInstaller.Install(ctx, installedApp.IconPath, targetID)
	if err != nil {
		return SetIDResult{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedIconPath, s.artifactRemover)
	})

	updatedDesktopEntry := metadata.desktopEntry.
		WithExec(installedAppImagePath).
		WithIcon(installedIconPath)
	installedDesktopEntryPath, err := s.desktopEntryInstaller.Install(ctx, targetID, updatedDesktopEntry.Bytes())
	if err != nil {
		return SetIDResult{}, err
	}
	rollback.add(func(ctx context.Context) error {
		return removeInstalledArtifact(ctx, installedDesktopEntryPath, s.artifactRemover)
	})

	updatedApp := domain.NewAppFromDesktopEntry(metadata.desktopEntry, domain.AppInput{
		ID:               targetID,
		AppImagePath:     installedAppImagePath,
		DesktopEntryPath: installedDesktopEntryPath,
		IconPath:         installedIconPath,
		Source:           installedApp.Source,
		UpdateSource:     installedApp.UpdateSource,
	})
	if err := s.apps.ReplaceID(ctx, installedApp.ID, updatedApp); err != nil {
		return SetIDResult{}, err
	}
	committed = true

	result = SetIDResult{PreviousID: installedApp.ID, ID: updatedApp.ID, App: updatedApp, Changed: true}
	if err := s.removeReplacedArtifacts(ctx, installedApp, updatedApp); err != nil {
		result.Warnings = append(result.Warnings, operationWarning(updatedApp.ID, "replaced-artifact-cleanup", err))
	}
	if s.desktopIntegrationRefresher != nil {
		if err := s.desktopIntegrationRefresher.Refresh(ctx); err != nil {
			result.Warnings = append(result.Warnings, operationWarning(updatedApp.ID, "desktop-refresh", err))
		}
	}

	return result, nil
}

func (s *service) installationDestinations(iconSource string, appID string) ([]string, error) {
	appImagePath, err := s.appImageInstaller.Destination(appID)
	if err != nil {
		return nil, err
	}
	iconPath, err := s.iconInstaller.Destination(iconSource, appID)
	if err != nil {
		return nil, err
	}
	desktopPath, err := s.desktopEntryInstaller.Destination(appID)
	if err != nil {
		return nil, err
	}
	return []string{appImagePath, iconPath, desktopPath}, nil
}

func (s *service) preflightReplacement(ctx context.Context, destinations []string, previous domain.App) error {
	managed := []string{previous.AppImagePath, previous.IconPath, previous.DesktopEntryPath}
	for _, destination := range destinations {
		exists, err := s.artifactPaths.Exists(ctx, destination)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		managedDestination := false
		for _, path := range managed {
			if path == "" {
				continue
			}
			same, err := s.artifactPaths.SameFile(ctx, destination, path)
			if err != nil {
				return err
			}
			if same {
				managedDestination = true
				break
			}
		}
		if !managedDestination {
			return fmt.Errorf("update destination %q already exists and is not the managed artifact", destination)
		}
	}
	return nil
}

func operationWarning(appID string, kind string, err error) OperationWarning {
	return OperationWarning{AppID: appID, Kind: kind, Error: err.Error()}
}

type installedAppImageIDMetadata struct {
	app          domain.App
	desktopEntry domain.DesktopEntry
}

func (s *service) inspectInstalledAppImageForID(ctx context.Context, appImagePath string) (installedAppImageIDMetadata, error) {
	workspacePath, cleanup, err := createWorkspace(ctx)
	if err != nil {
		return installedAppImageIDMetadata{}, err
	}
	defer cleanup()

	extraction, err := s.appImages.Extract(ctx, appImagePath, filepath.Join(workspacePath, "extract"))
	if err != nil {
		return installedAppImageIDMetadata{}, err
	}
	desktopFile, err := s.desktopEntries.Discover(ctx, extraction.RootDir)
	if err != nil {
		return installedAppImageIDMetadata{}, err
	}
	desktopEntry, err := domain.ParseDesktopEntry(desktopFile.Content)
	if err != nil {
		return installedAppImageIDMetadata{}, err
	}
	app := domain.NewAppFromDesktopEntry(desktopEntry, domain.AppInput{AppImagePath: appImagePath})
	return installedAppImageIDMetadata{app: app, desktopEntry: desktopEntry}, nil
}

func (s *service) removeInstalledAppArtifacts(ctx context.Context, installedApp domain.App) error {
	return errors.Join(
		removeInstalledArtifact(ctx, installedApp.DesktopEntryPath, s.artifactRemover),
		removeInstalledArtifact(ctx, installedApp.IconPath, s.artifactRemover),
		removeInstalledArtifact(ctx, installedApp.AppImagePath, s.artifactRemover),
	)
}

func (s *service) removeReplacedArtifacts(ctx context.Context, previous domain.App, next domain.App) error {
	var failures []error
	if remove, err := s.shouldRemoveReplacedArtifact(ctx, previous.DesktopEntryPath, next.DesktopEntryPath); err != nil {
		failures = append(failures, err)
	} else if remove {
		if err := s.artifactRemover(ctx, previous.DesktopEntryPath); err != nil {
			failures = append(failures, err)
		}
	}
	if remove, err := s.shouldRemoveReplacedArtifact(ctx, previous.IconPath, next.IconPath); err != nil {
		failures = append(failures, err)
	} else if remove {
		if err := s.artifactRemover(ctx, previous.IconPath); err != nil {
			failures = append(failures, err)
		}
	}
	if remove, err := s.shouldRemoveReplacedArtifact(ctx, previous.AppImagePath, next.AppImagePath); err != nil {
		failures = append(failures, err)
	} else if remove {
		if err := s.artifactRemover(ctx, previous.AppImagePath); err != nil {
			failures = append(failures, err)
		}
	}

	return errors.Join(failures...)
}

func (s *service) shouldRemoveReplacedArtifact(ctx context.Context, previous string, next string) (bool, error) {
	if previous == "" || previous == next {
		return false, nil
	}
	same, err := s.artifactPaths.SameFile(ctx, previous, next)
	if err != nil {
		return false, err
	}
	return !same, nil
}

func updateVersion(release GitHubRelease, asset GitHubReleaseAsset) (domain.Version, bool) {
	if version, ok := domain.ParseVersion(release.TagName); ok {
		return version, true
	}
	return domain.ParseVersion(asset.Name)
}

func (s *service) SetUpdateSource(ctx context.Context, req SetUpdateSourceRequest) (SetUpdateSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return SetUpdateSourceResult{}, err
	}
	unlock, err := s.mutations.Lock(ctx)
	if err != nil {
		return SetUpdateSourceResult{}, err
	}
	defer unlock()

	id := strings.TrimSpace(req.ID)
	if id == "" {
		return SetUpdateSourceResult{}, errors.New("app id is required")
	}
	if strings.TrimSpace(req.GitHubRepo) != "" && req.Embedded {
		return SetUpdateSourceResult{}, errors.New("provide either github repo or embedded update source, not both")
	}
	if strings.TrimSpace(req.GitHubRepo) == "" && !req.Embedded {
		return SetUpdateSourceResult{}, errors.New("update source is required")
	}
	if strings.TrimSpace(req.AssetPattern) != "" && strings.TrimSpace(req.GitHubRepo) == "" {
		return SetUpdateSourceResult{}, errors.New("asset pattern requires github repo")
	}

	installedApp, err := s.apps.Find(ctx, id)
	if err != nil {
		return SetUpdateSourceResult{}, err
	}

	var updateSource domain.UpdateSource
	if strings.TrimSpace(req.GitHubRepo) != "" {
		repo := strings.TrimSpace(req.GitHubRepo)
		if !validGitHubRepo(repo) {
			return SetUpdateSourceResult{}, errors.New("github repo must be in owner/repo format")
		}
		updateSource = domain.NewGitHubUpdateSource(repo, req.Prerelease)
		updateSource.AssetPattern = strings.TrimSpace(req.AssetPattern)
	} else {
		updateSource, err = s.embeddedUpdateSource(ctx, installedApp)
		if err != nil {
			return SetUpdateSourceResult{}, err
		}
	}

	installedApp.UpdateSource = updateSource
	if err := s.apps.Save(ctx, installedApp); err != nil {
		return SetUpdateSourceResult{}, err
	}

	return SetUpdateSourceResult{ID: installedApp.ID, UpdateSource: updateSource}, nil
}

func (s *service) embeddedUpdateSource(ctx context.Context, installedApp domain.App) (domain.UpdateSource, error) {
	if strings.TrimSpace(installedApp.AppImagePath) == "" {
		return domain.UpdateSource{}, errors.New("installed appimage path is required")
	}

	workspacePath, cleanup, err := createWorkspace(ctx)
	if err != nil {
		return domain.UpdateSource{}, err
	}
	defer cleanup()

	extraction, err := s.appImages.Extract(ctx, installedApp.AppImagePath, filepath.Join(workspacePath, "extract"))
	if err != nil {
		return domain.UpdateSource{}, err
	}
	if strings.TrimSpace(extraction.UpdateInfo) == "" {
		return domain.UpdateSource{}, errors.New("embedded update information not found")
	}

	return domain.NewEmbeddedUpdateSource(extraction.UpdateInfo), nil
}

func (s *service) UnsetUpdateSource(ctx context.Context, req UnsetUpdateSourceRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := s.mutations.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	id := strings.TrimSpace(req.ID)
	if id == "" {
		return errors.New("app id is required")
	}

	installedApp, err := s.apps.Find(ctx, id)
	if err != nil {
		return err
	}
	installedApp.UpdateSource = domain.UpdateSource{}
	return s.apps.Save(ctx, installedApp)
}

func (s *service) List(ctx context.Context, req ListRequest) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}

	apps, err := s.apps.List(ctx)
	if err != nil {
		return ListResult{}, err
	}

	items := make([]ListItem, 0, len(apps))
	for _, app := range apps {
		items = append(items, ListItem{
			ID:      app.ID,
			Name:    app.Name,
			Version: app.Version.String(),
		})
	}

	return ListResult{Items: items}, nil
}

func (s *service) Info(ctx context.Context, req InfoRequest) (InfoResult, error) {
	if err := ctx.Err(); err != nil {
		return InfoResult{}, err
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		return InfoResult{}, errors.New("app target is required")
	}
	if looksLikeLocalAppImagePath(target) {
		return s.infoLocal(ctx, target)
	}

	app, err := s.apps.Find(ctx, target)
	if err != nil {
		return InfoResult{}, err
	}

	return infoResultFromApp(app, true, "installed"), nil
}

func (s *service) infoLocal(ctx context.Context, path string) (InfoResult, error) {
	metadata, err := s.inspectLocalAppImage(ctx, AddRequest{Path: path}, domain.NewLocalSource(path, time.Time{}), filepath.Base(path), "", integrationSource)
	if err != nil {
		return InfoResult{}, err
	}

	return infoResultFromApp(metadata.app, false, "local_path"), nil
}

func infoResultFromApp(app domain.App, installed bool, targetKind string) InfoResult {
	return InfoResult{
		ID:           app.ID,
		Name:         app.Name,
		Version:      app.Version.String(),
		ExecPath:     app.AppImagePath,
		Installed:    installed,
		TargetKind:   targetKind,
		Source:       app.Source,
		UpdateSource: app.UpdateSource,
	}
}

func looksLikeLocalAppImagePath(target string) bool {
	return filepath.IsAbs(target) ||
		strings.HasPrefix(target, "."+string(filepath.Separator)) ||
		strings.HasPrefix(target, ".."+string(filepath.Separator)) ||
		strings.ContainsRune(target, filepath.Separator) ||
		strings.HasSuffix(strings.ToLower(target), ".appimage")
}

func (s *service) SelfUpdate(ctx context.Context, req SelfUpdateRequest) (SelfUpdateResult, error) {
	if err := ctx.Err(); err != nil {
		return SelfUpdateResult{}, err
	}
	if s.githubReleases == nil {
		return SelfUpdateResult{}, errors.New("github release finder is required")
	}
	if s.selfUpdater == nil {
		return SelfUpdateResult{}, errors.New("self updater is required")
	}

	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	release, err := s.selfUpdateRelease(ctx, req.Prerelease, activity)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	version := release.TagName
	currentVersion := strings.TrimSpace(s.currentVersion)
	if currentVersion == "" {
		currentVersion = "dev"
	}

	candidate := SelfUpdateCandidate{
		CurrentVersion: strings.TrimPrefix(currentVersion, "v"),
		NewVersion:     strings.TrimPrefix(version, "v"),
	}
	if asset, ok := selfUpdateArchiveAsset(release); ok {
		candidate.AssetName = asset.Name
		candidate.AssetSizeBytes = asset.SizeBytes
	}

	if !selfUpdateNeeded(currentVersion, version) {
		return SelfUpdateResult{Applied: true, Update: candidate}, nil
	}

	if req.Confirmation != nil {
		confirmed, err := req.Confirmation.ConfirmSelfUpdate(ctx, candidate)
		if err != nil {
			return SelfUpdateResult{}, err
		}
		if !confirmed {
			return SelfUpdateResult{Applied: false, Update: candidate}, nil
		}
	}

	task := activity.Start(ctx, Activity{Kind: ActivityKindWaiting, AppID: "selfupdate"})
	if err := s.selfUpdater.Install(ctx, version); err != nil {
		task.Fail(err)
		return SelfUpdateResult{}, err
	}
	task.Done("Updated aim to " + candidate.NewVersion)

	return SelfUpdateResult{Applied: true, Update: candidate}, nil
}

func (s *service) selfUpdateRelease(ctx context.Context, prerelease bool, activity ActivityReporter) (GitHubRelease, error) {
	const repo = "slobbe/appimage-manager"
	task := activity.Start(ctx, Activity{Kind: ActivityKindCheckingGitHub, Repo: repo})
	release, err := s.githubReleases.LatestRelease(ctx, repo, prerelease)
	if err != nil {
		task.Fail(err)
		return GitHubRelease{}, err
	}
	task.Done("Checked " + repo)
	return release, nil
}

func selfUpdateNeeded(current string, next string) bool {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	next = strings.TrimPrefix(strings.TrimSpace(next), "v")
	if current == "" || current == "dev" || current == "unknown" {
		return true
	}

	currentVersion, currentOK := domain.ParseVersion(current)
	nextVersion, nextOK := domain.ParseVersion(next)
	if !currentOK || !nextOK {
		return current != next
	}
	return domain.CompareVersions(nextVersion.String(), currentVersion.String()) > 0
}

func selfUpdateArchiveAsset(release GitHubRelease) (GitHubReleaseAsset, bool) {
	goarch := runtime.GOARCH
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".tar.gz") && strings.Contains(name, "linux-"+goarch) {
			return asset, true
		}
	}
	return GitHubReleaseAsset{}, false
}

func (s *service) Paths(ctx context.Context, req PathsRequest) (PathsResult, error) {
	if err := ctx.Err(); err != nil {
		return PathsResult{}, err
	}

	return PathsResult{
		ConfigFile:  s.config.ConfigFile,
		AppImageDir: s.config.AppImageDir,
		DesktopDir:  s.config.DesktopDir,
		IconDir:     s.config.IconDir,
	}, nil
}

func withFallbackVersion(entry domain.DesktopEntry, fallbackVersion string) domain.DesktopEntry {
	if !entry.Version.IsZero() {
		return entry
	}
	version, ok := domain.ParseVersion(fallbackVersion)
	if !ok {
		return entry
	}
	entry.Version = version
	return entry
}

func integrationSource(req AddRequest, embeddedUpdateInfo string) domain.UpdateSource {
	if req.GitHubRepo != "" {
		updateSource := domain.NewGitHubUpdateSource(req.GitHubRepo, req.Prerelease)
		updateSource.AssetPattern = strings.TrimSpace(req.AssetPattern)
		return updateSource
	}

	return domain.NewEmbeddedUpdateSource(embeddedUpdateInfo)
}

func validGitHubRepo(repo string) bool {
	owner, name, ok := strings.Cut(strings.TrimSpace(repo), "/")
	return ok && owner != "" && name != "" && !strings.Contains(name, "/")
}

func (s *service) validate() error {
	if s.appImages == nil {
		return fmt.Errorf("appimage extractor is required")
	}
	if s.desktopEntries == nil {
		return fmt.Errorf("desktop entry discoverer is required")
	}
	if s.icons == nil {
		return fmt.Errorf("icon discoverer is required")
	}
	if s.appImageInstaller == nil {
		return fmt.Errorf("appimage installer is required")
	}
	if s.iconInstaller == nil {
		return fmt.Errorf("icon installer is required")
	}
	if s.desktopEntryInstaller == nil {
		return fmt.Errorf("desktop entry installer is required")
	}
	if s.artifactRemover == nil {
		return fmt.Errorf("artifact remover is required")
	}
	if s.artifactBackups == nil {
		return fmt.Errorf("artifact backup manager is required")
	}
	if s.artifactPaths == nil {
		return fmt.Errorf("artifact path inspector is required")
	}
	if s.apps == nil {
		return fmt.Errorf("app repository is required")
	}
	if s.mutations == nil {
		return fmt.Errorf("mutation locker is required")
	}

	return nil
}
