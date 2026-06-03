package update

import (
	"context"
	"net/http"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/zsync"
)

var sharedHTTPClient = &http.Client{Timeout: 30 * time.Second}

func NewHTTPClient(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
func SharedHTTPClient() *http.Client                   { return sharedHTTPClient }

type testGitHubReleaseResolver struct{}

func (testGitHubReleaseResolver) ResolveReleaseAsset(repoSlug, assetPattern string) (*GitHubReleaseAsset, error) {
	asset, err := (github.Client{HTTPClient: SharedHTTPClient()}).ResolveReleaseAsset(repoSlug, assetPattern)
	if err != nil {
		return nil, err
	}
	return &GitHubReleaseAsset{
		DownloadURL:       asset.DownloadURL,
		TagName:           asset.TagName,
		NormalizedVersion: asset.NormalizedVersion,
		AssetName:         asset.AssetName,
		PreRelease:        asset.PreRelease,
	}, nil
}

type testZsyncMetadataFetcher struct{}

func (testZsyncMetadataFetcher) FetchMetadata(url string) (*domain.ZsyncMetadata, error) {
	return (zsync.Client{HTTPClient: SharedHTTPClient()}).FetchMetadata(url)
}

type testStagedDownloadService struct{}

func (testStagedDownloadService) AppImageFilename(assetName, downloadURL string) string {
	return download.AppImageFilename(assetName, downloadURL)
}

func (testStagedDownloadService) Download(ctx context.Context, assetURL, destination string, onProgress func(DownloadProgress)) error {
	return (download.StagedDownloader{Client: SharedHTTPClient()}).Download(ctx, assetURL, destination, func(event download.Progress) {
		if onProgress != nil {
			onProgress(DownloadProgress{Downloaded: event.Downloaded, Total: event.Total})
		}
	})
}

func (testStagedDownloadService) RemoveStaged(downloadPath string) {
	download.RemoveStaged(downloadPath)
}

func (testStagedDownloadService) StableDestination(dir, assetURL, nameHint string) (string, error) {
	return download.StableDestination(dir, assetURL, nameHint)
}

type testHashVerifier struct{}

func (testHashVerifier) VerifyHashes(path, expectedSHA256, expectedSHA1 string) error {
	return fsys.VerifyHashes(path, expectedSHA256, expectedSHA1)
}

type testUpdateInfoExtractor struct{}

func (testUpdateInfoExtractor) ExtractUpdateInfo(path string) (string, error) {
	return appimage.ExtractUpdateInfo(path)
}

func (testGitHubReleaseResolver) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*GitHubReleaseAssetSelection, error) {
	selection, err := (github.Client{HTTPClient: SharedHTTPClient()}).ResolveReleaseAssetSelection(repoSlug, assetPattern, arch)
	if err != nil {
		return nil, err
	}
	result := &GitHubReleaseAssetSelection{
		Ambiguous: selection.Ambiguous,
		Reason:    selection.Reason,
	}
	if selection.Release != nil {
		result.Release = &GitHubReleaseAsset{
			DownloadURL:       selection.Release.DownloadURL,
			TagName:           selection.Release.TagName,
			NormalizedVersion: selection.Release.NormalizedVersion,
			AssetName:         selection.Release.AssetName,
			PreRelease:        selection.Release.PreRelease,
		}
	}
	for _, candidate := range selection.Candidates {
		result.Candidates = append(result.Candidates, GitHubReleaseAssetCandidate{
			Name:        candidate.Name,
			DownloadURL: candidate.DownloadURL,
			Arch:        candidate.Arch,
			ArchLabel:   candidate.ArchLabel,
		})
	}
	return result, nil
}

func (testGitHubReleaseResolver) ResolveLatestReleaseTag(owner, repo string) (string, error) {
	return (github.Client{HTTPClient: SharedHTTPClient()}).ResolveLatestReleaseTag(owner, repo)
}
