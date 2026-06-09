package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appremove "github.com/slobbe/appimage-manager/internal/app/remove"
	appselfupdate "github.com/slobbe/appimage-manager/internal/app/selfupdate"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	repo "github.com/slobbe/appimage-manager/internal/infra/repository"
	selfupdateinfra "github.com/slobbe/appimage-manager/internal/infra/selfupdate"
	"github.com/slobbe/appimage-manager/internal/infra/zsync"
)

type RuntimePaths struct {
	AimDir       string
	DesktopDir   string
	TempDir      string
	IconThemeDir string
}

type DefaultWorkflowServices struct {
	Add        AddService
	List       ListService
	Info       InfoService
	Remove     RemoveService
	Update     UpdateService
	SelfUpdate SelfUpdateService
	Discovery  DiscoveryService
	Locker     StateLocker
}

type DefaultWorkflowOptions struct {
	DBPath                       string
	Paths                        RuntimePaths
	APIClient                    *http.Client
	NowISO                       func() string
	Locker                       StateLocker
	RemoteInstallDownload        func(context.Context, string, string) error
	BeforeRemoteInstallIntegrate func(context.Context)
	CheckManagedUpdate           ManagedUpdateChecker
	RefreshCaches                func(context.Context)
}

type ManagedUpdateCheckCacheStore struct {
	Path    string
	TempDir string
}

func NewDefaultManagedAppCompletions(dbPath string, prefix string) ([]ManagedAppCompletion, error) {
	apps, err := repo.NewStore(dbPath).GetAllApps()
	if err != nil {
		return nil, err
	}
	prefix = strings.TrimSpace(prefix)
	rows := make([]ManagedAppCompletion, 0, len(apps))
	seen := make(map[string]bool, len(apps))
	for key, app := range apps {
		if app == nil {
			continue
		}
		id := strings.TrimSpace(app.ID)
		if id == "" {
			id = strings.TrimSpace(key)
		}
		if id == "" || seen[id] {
			continue
		}
		if prefix != "" && !strings.HasPrefix(id, prefix) {
			continue
		}
		seen[id] = true
		rows = append(rows, ManagedAppCompletion{ID: id, Name: strings.TrimSpace(app.Name)})
	}
	return rows, nil
}

func NewDefaultWorkflowServices(opts DefaultWorkflowOptions) DefaultWorkflowServices {
	store := repo.NewStore(opts.DBPath)
	apiClient := opts.APIClient
	discoveryService := NewDefaultDiscoveryWorkflowService(apiClient)
	appImageService := NewDefaultAppImageService(opts.Paths)
	updateInfoService := NewDefaultUpdateInfoService(apiClient)
	integrateService := NewDefaultIntegrateService(store, opts.Paths, updateInfoService.GetUpdateInfo)
	removeService := NewDefaultRemoveService(store, opts.Paths)
	managedUpdateService := NewDefaultManagedUpdateService(opts.Paths, apiClient, opts.NowISO, func(ctx context.Context, src string, existingApp *domain.App, confirm func(existing, incoming *domain.UpdateSource) (bool, error)) (*domain.App, error) {
		return integrateService.IntegrateManagedUpdateWithoutCacheRefreshOrPersist(ctx, src, existingApp, confirm)
	})
	updateCheckCacheStore := NewDefaultManagedUpdateCheckCacheStore(opts.Paths)

	remoteInstallService := NewDefaultRemoteInstallService(
		store,
		opts.Paths,
		opts.RemoteInstallDownload,
		func(ctx context.Context, path string, confirm appintegrate.UpdateOverwritePrompt) (*domain.App, error) {
			if opts.BeforeRemoteInstallIntegrate != nil {
				opts.BeforeRemoteInstallIntegrate(ctx)
			}
			return integrateService.IntegrateLocal(ctx, path, confirm)
		},
		store.AddApp,
		removeService.Remove,
	)

	checkManagedUpdate := opts.CheckManagedUpdate
	if checkManagedUpdate == nil {
		checker := NewDefaultManagedUpdateChecker(apiClient)
		checkManagedUpdate = appupdate.NewManagedUpdateChecker(checker).Check
	}
	refreshCaches := opts.RefreshCaches
	if refreshCaches == nil {
		refreshCaches = func(ctx context.Context) {
			(defaultIntegrationCacheRefresherAdapter{}).RefreshIntegrationCaches(ctx, opts.Paths.DesktopDir, opts.Paths.IconThemeDir)
		}
	}

	return DefaultWorkflowServices{
		Add: NewAddWorkflowService(AddWorkflowService{
			Store:             store,
			Discovery:         discoveryService,
			Installer:         remoteInstallService,
			HasExtension:      defaultFilesystemAdapter{}.HasExtension,
			IntegrateLocalApp: integrateService.IntegrateLocal,
			ReintegrateApp:    integrateService.Reintegrate,
			AppImageInfo:      AppImageInfoReaderFunc(appImageService.ReadAppImageInfo),
			AimDir:            opts.Paths.AimDir,
			DesktopDir:        opts.Paths.DesktopDir,
		}),
		List: NewStoreListService(store),
		Info: NewStoreInfoService(StoreInfoService{
			Store:      store,
			AppImage:   AppImageInfoReaderFunc(appImageService.ReadAppImageInfo),
			UpdateInfo: UpdateInfoReaderFunc(updateInfoService.GetUpdateInfo),
			Discovery:  discoveryService,
		}),
		Remove: NewRemoveWorkflowService(RemoveWorkflowService{Store: store, RemoveFunc: removeService.Remove}),
		Update: NewSourceUpdateWorkflowService(SourceUpdateService{
			Store:                store,
			Locker:               opts.Locker,
			UpdateInfo:           UpdateInfoReaderFunc(updateInfoService.GetUpdateInfo),
			CheckManagedUpdate:   checkManagedUpdate,
			LoadCheckCache:       updateCheckCacheStore.Load,
			SaveCheckCache:       updateCheckCacheStore.Save,
			PersistCheckMetadata: persistCheckMetadataWithStore(store),
			InvalidateCheckCache: updateCheckCacheStore.Invalidate,
			ApplyManagedUpdate:   managedUpdateService.ApplyManagedUpdate,
			PersistApps:          store.AddAppsBatch,
			PersistApp:           store.AddApp,
			RemoveApp:            removeService.Remove,
			NowISO:               opts.NowISO,
			RefreshCaches:        refreshCaches,
		}),
		SelfUpdate: NewDefaultSelfUpdateWorkflowService(opts.Paths.TempDir, apiClient),
		Discovery:  discoveryService,
		Locker:     opts.Locker,
	}
}

func persistCheckMetadataWithStore(store *repo.Store) func([]CheckMetadataUpdate) error {
	return func(updates []CheckMetadataUpdate) error {
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
		return store.UpdateCheckMetadataBatch(repositoryUpdates)
	}
}

func NewDefaultManagedUpdateCheckCacheStore(paths RuntimePaths) ManagedUpdateCheckCacheStore {
	return ManagedUpdateCheckCacheStore{
		Path:    filepath.Join(paths.TempDir, "update-check-cache.json"),
		TempDir: paths.TempDir,
	}
}

func (store ManagedUpdateCheckCacheStore) Load() (*appupdate.CheckCacheFile, error) {
	data, ok, err := fsys.ReadFileIfExists(store.Path)
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

func (store ManagedUpdateCheckCacheStore) Save(cache *appupdate.CheckCacheFile) error {
	if cache == nil {
		return nil
	}
	if err := fsys.EnsureDir(store.TempDir); err != nil {
		return err
	}
	appupdate.NormalizeCheckCache(cache)

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return fsys.WriteAtomicFile(store.Path, data, 0o644)
}

func (store ManagedUpdateCheckCacheStore) Invalidate(appIDs []string) error {
	cache, err := store.Load()
	if err != nil {
		return err
	}
	appupdate.InvalidateCachedManagedUpdates(cache, appIDs...)
	return store.Save(cache)
}

func NewDefaultAppImageService(paths RuntimePaths) appimageapp.Service {
	return appimageapp.NewService(appimageapp.Service{
		Paths: appimageapp.Paths{
			AimDir:  paths.AimDir,
			TempDir: paths.TempDir,
		},
		Filesystem:           defaultFilesystemAdapter{},
		Extractor:            defaultAppImageExtractorAdapter{},
		DesktopEntryRewriter: defaultDesktopEntryRewriterAdapter{},
	})
}

func NewDefaultUpdateInfoService(apiClient *http.Client) appupdate.Service {
	return appupdate.NewService(appupdate.Service{
		UpdateInfoExtractor:   defaultUpdateInfoExtractorAdapter{},
		GitHubReleaseResolver: defaultGitHubReleaseAdapter{client: github.Client{HTTPClient: apiClient}},
	})
}

func NewDefaultManagedUpdateService(paths RuntimePaths, _ *http.Client, nowISO func() string, integrate appupdate.IntegrateFunc) appupdate.Service {
	return appupdate.NewService(appupdate.Service{
		TempDir:        paths.TempDir,
		NowISO:         nowISO,
		Zsync:          zsync.Runner{},
		StagedDownload: defaultStagedDownloadAdapter{client: download.SharedHTTPClient(), nowISO: nowISO},
		HashVerifier:   defaultHashVerifierAdapter{},
		Integrate:      integrate,
	})
}

func NewDefaultManagedUpdateChecker(apiClient *http.Client) appupdate.ManagedUpdateChecker {
	githubReleaseResolver := defaultGitHubReleaseAdapter{client: github.Client{HTTPClient: apiClient}}
	zsyncFetcher := defaultZsyncMetadataFetcherAdapter{client: apiClient}
	return appupdate.ManagedUpdateChecker{
		ZsyncCheck: func(update *domain.UpdateSource, localSHA1 string) (*appupdate.UpdateData, error) {
			return appupdate.ZsyncUpdateCheckWithResolver(update, localSHA1, zsyncFetcher, githubReleaseResolver)
		},
		GitHubReleaseCheck: func(update *domain.UpdateSource, currentVersion, localSHA1 string) (*appupdate.GitHubReleaseUpdate, error) {
			return appupdate.GitHubReleaseUpdateCheckWithResolver(update, currentVersion, localSHA1, githubReleaseResolver, zsyncFetcher)
		},
	}
}

func NewDefaultRemoteInstallService(store AppStore, paths RuntimePaths, downloadFn func(context.Context, string, string) error, integrateFn IntegrateFunc, persistApp func(*domain.App, bool) error, removeApp func(context.Context, string, bool) (*domain.App, error)) RemoteInstallService {
	return NewRemoteInstallService(RemoteInstallService{
		Store:    store,
		Filename: appupdate.ManagedUpdateDownloadFilename,
		StableDestination: func(assetURL, nameHint string) (string, error) {
			return download.StableDestination(paths.TempDir, assetURL, nameHint)
		},
		Download:          downloadFn,
		VerifySHA256:      defaultVerifySHA256,
		IntegrateLocalApp: integrateFn,
		PersistApp:        persistApp,
		RemoveApp:         removeApp,
		RemoveStaged:      download.RemoveStaged,
	})
}

func defaultVerifySHA256(path, expectedSHA256 string) error {
	return fsys.VerifyHashes(path, expectedSHA256, "")
}

func NewDefaultDiscoveryWorkflowService(apiClient *http.Client) DiscoveryWorkflowService {
	return NewDiscoveryWorkflowService(DiscoveryWorkflowService{
		Backends: []discovery.DiscoveryBackend{
			discovery.NewGitHubBackend(discovery.NewGitHubClientResolver(github.Client{HTTPClient: apiClient})),
		},
	})
}

func NewDefaultSelfUpdateWorkflowService(tempDir string, apiClient *http.Client) SelfUpdateWorkflowService {
	selfUpdateService := appselfupdate.NewService(appselfupdate.Service{
		TempDir:     tempDir,
		SelfUpdater: defaultSelfUpdaterAdapter{client: apiClient},
	})
	return NewSelfUpdateWorkflowService(SelfUpdateWorkflowService{
		CheckFunc:      selfUpdateService.Check,
		SelfUpdateFunc: selfUpdateService.SelfUpdate,
	})
}

func NewDefaultIntegrateService(store appintegrate.AppStore, paths RuntimePaths, embeddedUpdateInfo func(string) (*appupdate.UpdateInfo, error)) appintegrate.Service {
	return appintegrate.NewService(appintegrate.Service{
		Store:                            store,
		Filesystem:                       defaultFilesystemAdapter{},
		DesktopLinkResolver:              defaultDesktopLinkResolverAdapter{},
		DesktopEntryValidator:            defaultDesktopEntryValidatorAdapter{},
		DesktopIntegrationCacheRefresher: defaultDesktopIntegrationCacheRefresherAdapter{},
		Paths: appintegrate.Paths{
			AimDir:       paths.AimDir,
			DesktopDir:   paths.DesktopDir,
			TempDir:      paths.TempDir,
			IconThemeDir: paths.IconThemeDir,
		},
		AppImage:           NewDefaultAppImageService(paths),
		EmbeddedUpdateInfo: embeddedUpdateInfo,
	})
}

func NewDefaultRemoveService(store appremove.AppStore, paths RuntimePaths) appremove.Service {
	return appremove.NewService(appremove.Service{
		Store:                     store,
		Filesystem:                defaultFilesystemAdapter{},
		IntegrationCacheRefresher: defaultIntegrationCacheRefresherAdapter{},
		Paths: appremove.Paths{
			AimDir:       paths.AimDir,
			DesktopDir:   paths.DesktopDir,
			IconThemeDir: paths.IconThemeDir,
		},
	})
}

type defaultFilesystemAdapter struct{}

func (defaultFilesystemAdapter) Copy(src string, dst string) (string, error) {
	return fsys.Copy(src, dst)
}

func (defaultFilesystemAdapter) EnsureDir(path string) error {
	return fsys.EnsureDir(path)
}

func (defaultFilesystemAdapter) HasExtension(src string, ext string) bool {
	return fsys.HasExtension(src, ext)
}

func (defaultFilesystemAdapter) LocateDesktopEntry(root string) (*appimageapp.DesktopEntryCandidate, error) {
	candidate, err := fsys.LocateDesktopEntry(root)
	if err != nil {
		return nil, err
	}
	return &appimageapp.DesktopEntryCandidate{Path: candidate.Path, Stem: candidate.Stem}, nil
}

func (defaultFilesystemAdapter) LocateIcon(root string) (string, error) {
	return fsys.LocateIcon(root)
}

func (defaultFilesystemAdapter) MakeAbsolute(path string) (string, error) {
	return fsys.MakeAbsolute(path)
}

func (defaultFilesystemAdapter) Move(src string, dst string) (string, error) {
	return fsys.Move(src, dst)
}

func (defaultFilesystemAdapter) MakeExecutable(path string) error {
	return fsys.MakeExecutable(path)
}

func (defaultFilesystemAdapter) RemoveAll(path string) error {
	return fsys.RemoveAll(path)
}

func (defaultFilesystemAdapter) RemoveFileIfExists(path string) error {
	return fsys.RemoveFileIfExists(path)
}

func (defaultFilesystemAdapter) ReplaceSymlink(src string, linkPath string) error {
	return fsys.ReplaceSymlink(src, linkPath)
}

func (defaultFilesystemAdapter) Sha256AndSha1(path string) (string, string, error) {
	return fsys.Sha256AndSha1(path)
}

func (defaultFilesystemAdapter) ReadTextFile(path string) (string, error) {
	return fsys.ReadTextFile(path)
}

func (defaultFilesystemAdapter) RequireRegularFile(path string, subject string) (os.FileInfo, error) {
	return fsys.RequireRegularFile(path, subject)
}

type defaultAppImageExtractorAdapter struct{}

func (defaultAppImageExtractorAdapter) Extract(ctx context.Context, src string, tempBaseDir string) (*appimageapp.Extraction, appimageapp.CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Extract(ctx, src, tempBaseDir)
	if err != nil {
		return nil, nil, err
	}
	return &appimageapp.Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, appimageapp.CleanupFunc(cleanup), nil
}

func (defaultAppImageExtractorAdapter) Inspect(ctx context.Context, src string) (*appimageapp.Extraction, appimageapp.CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Inspect(ctx, src)
	if err != nil {
		return nil, nil, err
	}
	return &appimageapp.Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, appimageapp.CleanupFunc(cleanup), nil
}

type defaultDesktopEntryRewriterAdapter struct{}

func (defaultDesktopEntryRewriterAdapter) RewriteDesktopEntryFile(path, execPath, iconValue string) error {
	return desktop.RewriteDesktopEntryFile(path, execPath, iconValue)
}

func (defaultDesktopEntryRewriterAdapter) SanitizeDesktopStem(value string) string {
	return desktop.SanitizeDesktopStem(value)
}

func (defaultDesktopEntryRewriterAdapter) DesktopStemFromPath(path string) string {
	return desktop.DesktopStemFromPath(path)
}

type defaultDesktopLinkResolverAdapter struct{}

func (defaultDesktopLinkResolverAdapter) ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName string) (string, error) {
	return desktop.ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName)
}

type defaultDesktopEntryValidatorAdapter struct{}

func (defaultDesktopEntryValidatorAdapter) ValidateDesktopEntry(ctx context.Context, desktopPath string) error {
	binary, err := exec.LookPath("desktop-file-validate")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
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

type defaultDesktopIntegrationCacheRefresherAdapter struct{}

func (defaultDesktopIntegrationCacheRefresherAdapter) RefreshDesktopIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string) {
	desktop.RefreshIntegrationCaches(ctx, desktop.CachePaths{
		DesktopDir:   desktopDir,
		IconThemeDir: iconThemeDir,
	})
}

type defaultIntegrationCacheRefresherAdapter struct{}

func (defaultIntegrationCacheRefresherAdapter) RefreshIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string) {
	desktop.RefreshIntegrationCaches(ctx, desktop.CachePaths{
		DesktopDir:   desktopDir,
		IconThemeDir: iconThemeDir,
	})
}

type defaultUpdateInfoExtractorAdapter struct{}

func (defaultUpdateInfoExtractorAdapter) ExtractUpdateInfo(path string) (string, error) {
	return appimageinfra.ExtractUpdateInfo(path)
}

type defaultZsyncMetadataFetcherAdapter struct {
	client *http.Client
}

func (adapter defaultZsyncMetadataFetcherAdapter) FetchMetadata(url string) (*domain.ZsyncMetadata, error) {
	return (zsync.Client{HTTPClient: adapter.client}).FetchMetadata(url)
}

type defaultStagedDownloadAdapter struct {
	client *http.Client
	nowISO func() string
}

func (adapter defaultStagedDownloadAdapter) AppImageFilename(assetName, downloadURL string) string {
	return download.AppImageFilename(assetName, downloadURL)
}

func (adapter defaultStagedDownloadAdapter) Download(ctx context.Context, assetURL, destination string, onProgress func(appupdate.DownloadProgress)) error {
	return (download.StagedDownloader{
		Client: adapter.client,
		NowISO: adapter.nowISO,
	}).Download(ctx, assetURL, destination, func(event download.Progress) {
		if onProgress != nil {
			onProgress(appupdate.DownloadProgress{Downloaded: event.Downloaded, Total: event.Total})
		}
	})
}

func (defaultStagedDownloadAdapter) RemoveStaged(downloadPath string) {
	download.RemoveStaged(downloadPath)
}

func (defaultStagedDownloadAdapter) StableDestination(dir, assetURL, nameHint string) (string, error) {
	return download.StableDestination(dir, assetURL, nameHint)
}

type defaultHashVerifierAdapter struct{}

func (defaultHashVerifierAdapter) VerifyHashes(path, expectedSHA256, expectedSHA1 string) error {
	return fsys.VerifyHashes(path, expectedSHA256, expectedSHA1)
}

type defaultSelfUpdaterAdapter struct {
	client *http.Client
}

func (adapter defaultSelfUpdaterAdapter) clientAdapter() selfupdateinfra.Client {
	return selfupdateinfra.Client{HTTPClient: adapter.client}
}

func (adapter defaultSelfUpdaterAdapter) FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error) {
	return adapter.clientAdapter().FetchLatestReleaseTag(ctx, releaseURL)
}

func (adapter defaultSelfUpdaterAdapter) ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	return adapter.clientAdapter().ReadInstalledVersion(ctx, binaryPath)
}

func (adapter defaultSelfUpdaterAdapter) ResolveInstalledPath() (string, error) {
	return adapter.clientAdapter().ResolveInstalledPath()
}

func (adapter defaultSelfUpdaterAdapter) RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error), env map[string]string) error {
	return adapter.clientAdapter().RunInstallerScript(ctx, scriptURL, tempDir, env)
}

type defaultGitHubReleaseAdapter struct {
	client github.Client
}

func (adapter defaultGitHubReleaseAdapter) ResolveReleaseAsset(repoSlug, assetPattern string) (*appupdate.GitHubReleaseAsset, error) {
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

func (adapter defaultGitHubReleaseAdapter) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*appupdate.GitHubReleaseAssetSelection, error) {
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

func (adapter defaultGitHubReleaseAdapter) ResolveLatestReleaseTag(owner, repo string) (string, error) {
	return adapter.client.ResolveLatestReleaseTag(owner, repo)
}
