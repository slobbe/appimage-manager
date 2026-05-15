package cli

import (
	"context"
	"errors"
	"fmt"
	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appremove "github.com/slobbe/appimage-manager/internal/app/remove"
	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	appupgrade "github.com/slobbe/appimage-manager/internal/app/upgrade"
	"github.com/slobbe/appimage-manager/internal/domain"
	models "github.com/slobbe/appimage-manager/internal/domain"
	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/config"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
	repo "github.com/slobbe/appimage-manager/internal/infra/repository"
	"github.com/slobbe/appimage-manager/internal/infra/selfupdate"
	"github.com/slobbe/appimage-manager/internal/infra/zsync"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type filesystemAdapter struct{}

func (filesystemAdapter) Copy(src string, dst string) (string, error) {
	return fsys.Copy(src, dst)
}

func (filesystemAdapter) EnsureDir(path string) error {
	return fsys.EnsureDir(path)
}

func (filesystemAdapter) HasExtension(src string, ext string) bool {
	return fsys.HasExtension(src, ext)
}

func (filesystemAdapter) LocateDesktopEntry(root string) (*appimageapp.DesktopEntryCandidate, error) {
	candidate, err := fsys.LocateDesktopEntry(root)
	if err != nil {
		return nil, err
	}
	return &appimageapp.DesktopEntryCandidate{Path: candidate.Path, Stem: candidate.Stem}, nil
}

func (filesystemAdapter) LocateIcon(root string) (string, error) {
	return fsys.LocateIcon(root)
}

func (filesystemAdapter) MakeAbsolute(path string) (string, error) {
	return fsys.MakeAbsolute(path)
}

func (filesystemAdapter) Move(src string, dst string) (string, error) {
	return fsys.Move(src, dst)
}

func (filesystemAdapter) MakeExecutable(path string) error {
	return fsys.MakeExecutable(path)
}

func (filesystemAdapter) RemoveAll(path string) error {
	return fsys.RemoveAll(path)
}

func (filesystemAdapter) RemoveFileIfExists(path string) error {
	return fsys.RemoveFileIfExists(path)
}

func (filesystemAdapter) ReplaceSymlink(src string, linkPath string) error {
	return fsys.ReplaceSymlink(src, linkPath)
}

func (filesystemAdapter) Sha256AndSha1(path string) (string, string, error) {
	return fsys.Sha256AndSha1(path)
}

func (filesystemAdapter) ReadTextFile(path string) (string, error) {
	return fsys.ReadTextFile(path)
}

func (filesystemAdapter) RequireRegularFile(path string, subject string) (os.FileInfo, error) {
	return fsys.RequireRegularFile(path, subject)
}

type appimageExtractorAdapter struct{}

func (appimageExtractorAdapter) Extract(ctx context.Context, src string, tempBaseDir string) (*appimageapp.Extraction, appimageapp.CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Extract(ctx, src, tempBaseDir)
	if err != nil {
		return nil, nil, err
	}
	return &appimageapp.Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, appimageapp.CleanupFunc(cleanup), nil
}

func (appimageExtractorAdapter) Inspect(ctx context.Context, src string) (*appimageapp.Extraction, appimageapp.CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Inspect(ctx, src)
	if err != nil {
		return nil, nil, err
	}
	return &appimageapp.Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, appimageapp.CleanupFunc(cleanup), nil
}

type desktopEntryRewriterAdapter struct{}

func (desktopEntryRewriterAdapter) RewriteDesktopEntryFile(path, execPath, iconValue string) error {
	return desktop.RewriteDesktopEntryFile(path, execPath, iconValue)
}

func (desktopEntryRewriterAdapter) SanitizeDesktopStem(value string) string {
	return desktop.SanitizeDesktopStem(value)
}

func (desktopEntryRewriterAdapter) DesktopStemFromPath(path string) string {
	return desktop.DesktopStemFromPath(path)
}

type desktopLinkResolverAdapter struct{}

func (desktopLinkResolverAdapter) ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName string) (string, error) {
	return desktop.ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName)
}

type desktopEntryValidatorAdapter struct{}

func (desktopEntryValidatorAdapter) ValidateDesktopEntry(ctx context.Context, desktopPath string) error {
	binary, err := exec.LookPath("desktop-file-validate")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Warning: desktop-file-validate not found; skipping desktop entry validation for %s\n", desktopPath)
			return nil
		}
		return fmt.Errorf("failed to find desktop-file-validate: %w", err)
	}

	out, err := exec.CommandContext(ctx, binary, desktopPath).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			return fmt.Errorf("desktop entry validation failed for %s: %w", desktopPath, err)
		}
		return fmt.Errorf("desktop entry validation failed for %s: %s", desktopPath, message)
	}
	return nil
}

type desktopIntegrationCacheRefresherAdapter struct{}

func (desktopIntegrationCacheRefresherAdapter) RefreshDesktopIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string) {
	desktop.RefreshIntegrationCaches(ctx, desktop.CachePaths{
		DesktopDir:   desktopDir,
		IconThemeDir: iconThemeDir,
	})
}

type integrationCacheRefresherAdapter struct{}

func (integrationCacheRefresherAdapter) RefreshIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string) {
	desktop.RefreshIntegrationCaches(ctx, desktop.CachePaths{
		DesktopDir:   desktopDir,
		IconThemeDir: iconThemeDir,
	})
}

type zsyncMetadataFetcherAdapter struct{}

func (zsyncMetadataFetcherAdapter) FetchMetadata(url string) ([]byte, error) {
	return (zsync.Client{HTTPClient: appupdate.SharedHTTPClient()}).FetchMetadata(url)
}

type stagedDownloadAdapter struct {
	client func() *http.Client
	nowISO func() string
}

func (adapter stagedDownloadAdapter) AppImageFilename(assetName, downloadURL string) string {
	return download.AppImageFilename(assetName, downloadURL)
}

func (adapter stagedDownloadAdapter) Download(ctx context.Context, assetURL, destination string, onProgress func(appupdate.DownloadProgress)) error {
	return (download.StagedDownloader{
		Client: adapter.client(),
		NowISO: adapter.nowISO,
	}).Download(ctx, assetURL, destination, func(event download.Progress) {
		if onProgress != nil {
			onProgress(appupdate.DownloadProgress{Downloaded: event.Downloaded, Total: event.Total})
		}
	})
}

func (adapter stagedDownloadAdapter) RemoveStaged(downloadPath string) {
	download.RemoveStaged(downloadPath)
}

func (adapter stagedDownloadAdapter) StableDestination(dir, assetURL, nameHint string) (string, error) {
	return download.StableDestination(dir, assetURL, nameHint)
}

type hashVerifierAdapter struct{}

func (hashVerifierAdapter) VerifyHashes(path, expectedSHA256, expectedSHA1 string) error {
	return fsys.VerifyHashes(path, expectedSHA256, expectedSHA1)
}

type pathResolverAdapter struct{}

func (pathResolverAdapter) MakeAbsolute(path string) (string, error) {
	return fsys.MakeAbsolute(path)
}

var (
	zsyncLookPath       = exec.LookPath
	zsyncCommandContext = exec.CommandContext
)

type runtimeDownloadMetadata struct {
	URL          string
	ETag         string
	LastModified string
	TotalBytes   int64
}

type runtimeDownloadRequest struct {
	URL         string
	Destination string
	Metadata    *runtimeDownloadMetadata
}

type runtimeDownloadProgress struct {
	Downloaded int64
	Total      int64
	Metadata   runtimeDownloadMetadata
}

type runtimeDownloadStatusError struct {
	Status string
	Code   int
}

func (err *runtimeDownloadStatusError) Error() string {
	return fmt.Sprintf("download failed with status %s", err.Status)
}

func setRuntimeDownloadTimeout(timeout time.Duration) {
	download.SetHTTPClientTimeout(timeout)
}

func runtimeDownloadHTTPClient() *http.Client {
	return download.SharedHTTPClient()
}

func runtimeDownload(ctx context.Context, req runtimeDownloadRequest, onProgress func(runtimeDownloadProgress)) (*runtimeDownloadMetadata, error) {
	result, err := (download.Downloader{Client: download.SharedHTTPClient()}).Download(ctx, download.Request{
		URL:         req.URL,
		Destination: req.Destination,
		Metadata:    downloadMetadataFromRuntime(req.Metadata),
	}, func(event download.Progress) {
		if onProgress != nil {
			metadata := runtimeMetadataFromDownload(&event.Metadata)
			onProgress(runtimeDownloadProgress{
				Downloaded: event.Downloaded,
				Total:      event.Total,
				Metadata:   *metadata,
			})
		}
	})
	if err != nil {
		var statusErr *download.StatusError
		if errors.As(err, &statusErr) {
			return nil, &runtimeDownloadStatusError{Status: statusErr.Status, Code: statusErr.Code}
		}
		return nil, err
	}
	return runtimeMetadataFromDownload(result), nil
}

func downloadMetadataFromRuntime(meta *runtimeDownloadMetadata) *download.Metadata {
	if meta == nil {
		return nil
	}
	return &download.Metadata{
		URL:          meta.URL,
		ETag:         meta.ETag,
		LastModified: meta.LastModified,
		TotalBytes:   meta.TotalBytes,
	}
}

func runtimeMetadataFromDownload(meta *download.Metadata) *runtimeDownloadMetadata {
	if meta == nil {
		return nil
	}
	return &runtimeDownloadMetadata{
		URL:          meta.URL,
		ETag:         meta.ETag,
		LastModified: meta.LastModified,
		TotalBytes:   meta.TotalBytes,
	}
}

func runtimeEnsureDir(path string) error {
	return fsys.EnsureDir(path)
}

func runtimeReadFileIfExists(path string) ([]byte, bool, error) {
	return fsys.ReadFileIfExists(path)
}

func runtimeWriteAtomicFile(path string, data []byte, perm os.FileMode) error {
	return fsys.WriteAtomicFile(path, data, perm)
}

func runtimeRemoveFileIfExists(path string) error {
	return fsys.RemoveFileIfExists(path)
}

func runtimeHasExtension(src string, ext string) bool {
	return fsys.HasExtension(src, ext)
}

func runtimeZsyncRunner() appupdate.ZsyncRunner {
	return zsync.Runner{
		LookPath:       zsyncLookPath,
		CommandContext: zsyncCommandContext,
	}
}

type selfUpdaterAdapter struct{}

func (selfUpdaterAdapter) FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error) {
	return (selfupdate.Client{HTTPClient: appupgrade.SharedHTTPClient()}).FetchLatestReleaseTag(ctx, releaseURL)
}

func (selfUpdaterAdapter) ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	return (selfupdate.Client{HTTPClient: appupgrade.SharedHTTPClient()}).ReadInstalledVersion(ctx, binaryPath)
}

func (selfUpdaterAdapter) ResolveInstalledPath() (string, error) {
	return (selfupdate.Client{HTTPClient: appupgrade.SharedHTTPClient()}).ResolveInstalledPath()
}

func (selfUpdaterAdapter) RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error)) error {
	return (selfupdate.Client{HTTPClient: appupgrade.SharedHTTPClient()}).RunInstallerScript(ctx, scriptURL, tempDir)
}

type gitHubReleaseAdapter struct {
	client github.Client
}

func (adapter gitHubReleaseAdapter) ResolveReleaseAsset(repoSlug, assetPattern string) (*appupdate.GitHubReleaseAsset, error) {
	asset, err := adapter.client.ResolveReleaseAsset(repoSlug, assetPattern)
	if err != nil {
		return nil, err
	}
	return &appupdate.GitHubReleaseAsset{
		DownloadURL:       asset.DownloadURL,
		TagName:           asset.TagName,
		NormalizedVersion: asset.NormalizedVersion,
		AssetName:         asset.AssetName,
		PreRelease:        asset.PreRelease,
	}, nil
}

func (adapter gitHubReleaseAdapter) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*appupdate.GitHubReleaseAssetSelection, error) {
	selection, err := adapter.client.ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
	if err != nil {
		return nil, err
	}
	result := &appupdate.GitHubReleaseAssetSelection{
		Ambiguous: selection.Ambiguous,
		Reason:    selection.Reason,
	}
	if selection.Release != nil {
		result.Release = &appupdate.GitHubReleaseAsset{
			DownloadURL:       selection.Release.DownloadURL,
			TagName:           selection.Release.TagName,
			NormalizedVersion: selection.Release.NormalizedVersion,
			AssetName:         selection.Release.AssetName,
			PreRelease:        selection.Release.PreRelease,
		}
	}
	for _, candidate := range selection.Candidates {
		result.Candidates = append(result.Candidates, appupdate.GitHubReleaseAssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result, nil
}

func (adapter gitHubReleaseAdapter) ResolveLatestReleaseTag(owner, repo string) (string, error) {
	return adapter.client.ResolveLatestReleaseTag(owner, repo)
}

func configureAppPorts(networkTimeout time.Duration) {
	appimageapp.SetFilesystem(filesystemAdapter{})
	appimageapp.SetExtractor(appimageExtractorAdapter{})
	appimageapp.SetDesktopEntryRewriter(desktopEntryRewriterAdapter{})
	appintegrate.SetFilesystem(filesystemAdapter{})
	appintegrate.SetDesktopLinkResolver(desktopLinkResolverAdapter{})
	appintegrate.SetDesktopEntryValidator(desktopEntryValidatorAdapter{})
	appintegrate.SetDesktopIntegrationCacheRefresher(desktopIntegrationCacheRefresherAdapter{})
	appremove.SetFilesystem(filesystemAdapter{})
	appremove.SetIntegrationCacheRefresher(integrationCacheRefresherAdapter{})
	appupdate.SetZsyncMetadataFetcher(zsyncMetadataFetcherAdapter{})
	appupdate.SetStagedDownloadService(stagedDownloadAdapter{client: appupdate.SharedHTTPClient})
	appupdate.SetHashVerifier(hashVerifierAdapter{})
	appupdate.SetPathResolver(pathResolverAdapter{})
	appupgrade.SetSelfUpdater(selfUpdaterAdapter{})
	discovery.SetGitHubResolver(github.DiscoveryResolver{
		Client: github.Client{HTTPClient: httpclient.New(networkTimeout)},
	})
	appupdate.SetGitHubReleaseResolver(gitHubReleaseAdapter{
		client: github.Client{HTTPClient: appupdate.SharedHTTPClient()},
	})
}

func repositoryStore() *repo.Store {
	return repo.NewStore(config.DbSrc)
}

func configureRepositoryStores() {
	appintegrate.SetStore(repositoryStore())
	appremove.SetStore(repositoryStore())
}

type checkMetadataUpdate struct {
	ID            string
	Checked       bool
	Available     bool
	Latest        string
	LastCheckedAt string
}

func defaultAddAppsBatch(apps []*models.App, overwrite bool) error {
	return repositoryStore().AddAppsBatch(apps, overwrite)
}

func defaultAddSingleApp(app *models.App, overwrite bool) error {
	return repositoryStore().AddApp(app, overwrite)
}

func getManagedApp(id string) (*models.App, error) {
	return repositoryStore().GetApp(id)
}

func getAllManagedApps() (map[string]*models.App, error) {
	return repositoryStore().GetAllApps()
}

func updateManagedApp(app *models.App) error {
	return repositoryStore().UpdateApp(app)
}

func updateCheckMetadataBatch(updates []checkMetadataUpdate) error {
	repositoryUpdates := make([]repo.CheckMetadataUpdate, 0, len(updates))
	for _, update := range updates {
		repositoryUpdates = append(repositoryUpdates, repo.CheckMetadataUpdate{
			ID:            update.ID,
			Checked:       update.Checked,
			Available:     update.Available,
			Latest:        update.Latest,
			LastCheckedAt: update.LastCheckedAt,
		})
	}
	return repositoryStore().UpdateCheckMetadataBatch(repositoryUpdates)
}

type legacyAddService struct{}

func (legacyAddService) IntegrateLocal(ctx context.Context, req appservices.IntegrateLocalRequest) (*appservices.AddResult, error) {
	var prompt appintegrate.UpdateOverwritePrompt
	if req.ConfirmUpdateSourceReplace != nil {
		prompt = req.ConfirmUpdateSourceReplace.ConfirmUpdateSourceReplace
	}
	app, err := integrateLocalApp(ctx, req.Path, prompt)
	if err != nil {
		return nil, err
	}
	return &appservices.AddResult{Status: "integrated", App: app}, nil
}

func (legacyAddService) Reintegrate(ctx context.Context, id string) (*appservices.AddResult, error) {
	app, err := integrateExistingApp(ctx, id)
	if err != nil {
		return nil, err
	}
	return &appservices.AddResult{Status: "reintegrated", App: app}, nil
}

func (legacyAddService) InstallDirectURL(ctx context.Context, req appservices.InstallDirectURLRequest) (*appservices.AddResult, error) {
	_ = ctx
	_ = req
	return nil, softwareError(fmt.Errorf("direct URL install service is not wired yet"))
}

func (legacyAddService) InstallPackageRef(ctx context.Context, req appservices.InstallPackageRefRequest) (*appservices.AddResult, error) {
	_ = ctx
	_ = req
	return nil, softwareError(fmt.Errorf("package install service is not wired yet"))
}

func (legacyAddService) PlanLocalIntegration(ctx context.Context, path string) (*appservices.DryRunPlan, error) {
	plan, err := buildLocalIntegrateDryRunPlan(ctx, path)
	if err != nil {
		return nil, err
	}
	return &appservices.DryRunPlan{
		Action: "integrate",
		Target: path,
		Values: plan,
	}, nil
}

func (legacyAddService) PlanDirectURLInstall(ctx context.Context, req appservices.InstallDirectURLRequest) (*appservices.DryRunPlan, error) {
	_ = ctx
	return &appservices.DryRunPlan{
		Action: "install",
		Target: req.URL,
		Values: map[string]interface{}{
			"target": req.URL,
			"sha256": req.SHA256,
		},
	}, nil
}

func (legacyAddService) PlanPackageRefInstall(ctx context.Context, req appservices.InstallPackageRefRequest) (*appservices.DryRunPlan, error) {
	metadata, err := resolvePackageMetadataFromRef(ctx, req.Ref, req.AssetPattern)
	if err != nil {
		return nil, err
	}
	return &appservices.DryRunPlan{
		Action: "install",
		Target: formatProviderRef(req.Ref),
		Values: map[string]interface{}{
			"action":   "install",
			"target":   formatProviderRef(req.Ref),
			"provider": req.Ref,
			"metadata": packageMetadataOutput(metadata),
		},
	}, nil
}

type legacyInfoService struct {
	Discovery appservices.DiscoveryService
}

func (service legacyInfoService) ManagedAppInfo(ctx context.Context, id string) (*appservices.InfoResult, error) {
	_ = ctx
	app, err := getManagedApp(id)
	if err != nil {
		return nil, err
	}
	embeddedSource, _ := embeddedUpdateSourceForPath(app.ExecPath)
	return &appservices.InfoResult{
		Kind:           appservices.InfoKindManagedApp,
		App:            app,
		EmbeddedUpdate: embeddedSource,
	}, nil
}

func (service legacyInfoService) LocalAppImageInfo(ctx context.Context, path string) (*appservices.InfoResult, error) {
	info, err := readAppImageInfo(ctx, path)
	if err != nil {
		return nil, err
	}
	embeddedSource, _ := embeddedUpdateSourceForPath(path)
	return &appservices.InfoResult{
		Kind:           appservices.InfoKindLocalAppImage,
		AppImage:       info,
		EmbeddedUpdate: embeddedSource,
	}, nil
}

func (service legacyInfoService) PackageRefInfo(ctx context.Context, req appservices.PackageRefInfoRequest) (*appservices.InfoResult, error) {
	metadata, err := service.Discovery.ResolvePackage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &appservices.InfoResult{
		Kind:    appservices.InfoKindPackage,
		Package: metadata,
	}, nil
}

type legacyUpdateService struct{}

func (legacyUpdateService) Check(ctx context.Context, req appservices.UpdateCheckRequest) (*appservices.UpdateCheckResult, error) {
	_ = ctx
	_ = req
	return nil, softwareError(fmt.Errorf("update check service is not wired yet"))
}

func (legacyUpdateService) Apply(ctx context.Context, req appservices.UpdateApplyRequest) (*appservices.UpdateApplyResult, error) {
	_ = ctx
	_ = req
	return nil, softwareError(fmt.Errorf("update apply service is not wired yet"))
}

func (legacyUpdateService) SetSource(ctx context.Context, req appservices.UpdateSourceRequest) (*appservices.UpdateSourceResult, error) {
	_ = ctx
	app, err := getManagedApp(req.ID)
	if err != nil {
		return nil, err
	}
	app.Update = req.Source
	if err := updateManagedApp(app); err != nil {
		return nil, err
	}
	return &appservices.UpdateSourceResult{ID: req.ID, Source: req.Source, Changed: true}, nil
}

func (legacyUpdateService) UnsetSource(ctx context.Context, id string) (*appservices.UpdateSourceResult, error) {
	_ = ctx
	app, err := getManagedApp(id)
	if err != nil {
		return nil, err
	}
	app.Update = &domain.UpdateSource{Kind: domain.UpdateNone}
	if err := updateManagedApp(app); err != nil {
		return nil, err
	}
	return &appservices.UpdateSourceResult{ID: id, Source: app.Update, Changed: true}, nil
}

func (legacyUpdateService) PlanSetSource(ctx context.Context, req appservices.UpdateSourceRequest) (*appservices.DryRunPlan, error) {
	_ = ctx
	app, err := getManagedApp(req.ID)
	if err != nil {
		return nil, err
	}
	return &appservices.DryRunPlan{
		Action: "set_update_source",
		Target: req.ID,
		Values: buildUpdateSetDryRunResult(req.ID, app.Update, req.Source),
	}, nil
}

func (legacyUpdateService) PlanUnsetSource(ctx context.Context, id string) (*appservices.DryRunPlan, error) {
	_ = ctx
	app, err := getManagedApp(id)
	if err != nil {
		return nil, err
	}
	return &appservices.DryRunPlan{
		Action: "unset_update_source",
		Target: id,
		Values: buildUpdateUnsetDryRunResult(id, app.Update),
	}, nil
}
