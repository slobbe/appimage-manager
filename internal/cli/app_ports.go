package cli

import (
	"context"
	"net/http"
	"os"
	"time"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appremove "github.com/slobbe/appimage-manager/internal/app/remove"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	appupgrade "github.com/slobbe/appimage-manager/internal/app/upgrade"
	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/httpclient"
	"github.com/slobbe/appimage-manager/internal/infra/selfupdate"
	"github.com/slobbe/appimage-manager/internal/infra/zsync"
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

type gitHubDiscoveryAdapter struct {
	client github.Client
}

func (adapter gitHubDiscoveryAdapter) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*discovery.ReleaseAssetSelection, error) {
	selection, err := adapter.client.ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
	if err != nil {
		return nil, err
	}
	result := &discovery.ReleaseAssetSelection{
		Ambiguous: selection.Ambiguous,
		Reason:    selection.Reason,
	}
	if selection.Release != nil {
		result.Release = &discovery.ReleaseAsset{
			DownloadURL:       selection.Release.DownloadURL,
			TagName:           selection.Release.TagName,
			NormalizedVersion: selection.Release.NormalizedVersion,
			AssetName:         selection.Release.AssetName,
			PreRelease:        selection.Release.PreRelease,
		}
	}
	for _, candidate := range selection.Candidates {
		result.Candidates = append(result.Candidates, discovery.ReleaseAssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result, nil
}

func (adapter gitHubDiscoveryAdapter) FetchRepository(ctx context.Context, repoSlug string) (*discovery.Repository, error) {
	repository, err := adapter.client.FetchRepository(ctx, repoSlug)
	if err != nil {
		return nil, err
	}
	return &discovery.Repository{
		Name:        repository.Name,
		Description: repository.Description,
		HTMLURL:     repository.HTMLURL,
	}, nil
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

func configureAppPorts(networkTimeout time.Duration) {
	appimageapp.SetFilesystem(filesystemAdapter{})
	appimageapp.SetExtractor(appimageExtractorAdapter{})
	appimageapp.SetDesktopEntryRewriter(desktopEntryRewriterAdapter{})
	appintegrate.SetFilesystem(filesystemAdapter{})
	appintegrate.SetDesktopLinkResolver(desktopLinkResolverAdapter{})
	appremove.SetFilesystem(filesystemAdapter{})
	appremove.SetIntegrationCacheRefresher(integrationCacheRefresherAdapter{})
	appupdate.SetZsyncMetadataFetcher(zsyncMetadataFetcherAdapter{})
	appupdate.SetStagedDownloadService(stagedDownloadAdapter{client: appupdate.SharedHTTPClient})
	appupdate.SetHashVerifier(hashVerifierAdapter{})
	appupgrade.SetSelfUpdater(selfUpdaterAdapter{})
	discovery.SetGitHubResolver(gitHubDiscoveryAdapter{
		client: github.Client{HTTPClient: httpclient.New(networkTimeout)},
	})
	appupdate.SetGitHubReleaseResolver(gitHubReleaseAdapter{
		client: github.Client{HTTPClient: appupdate.SharedHTTPClient()},
	})
}
