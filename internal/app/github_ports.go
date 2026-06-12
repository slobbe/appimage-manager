package app

import "context"

// GitHubReleaseFinder looks up GitHub release metadata for a repository.
//
// Implementations belong in infrastructure. The app layer uses this port to
// decide which release asset should be downloaded and integrated.
type GitHubReleaseFinder interface {
	LatestRelease(ctx context.Context, repo string, includePrerelease bool) (GitHubRelease, error)
	LatestPrerelease(ctx context.Context, repo string) (GitHubRelease, error)
	ReleaseByTag(ctx context.Context, repo string, tag string) (GitHubRelease, error)
}

// GitHubRelease is the app-layer representation of a GitHub release.
type GitHubRelease struct {
	Repo       string
	TagName    string
	Name       string
	URL        string
	Prerelease bool
	Draft      bool
	Assets     []GitHubReleaseAsset
}

// GitHubReleaseAsset is an asset attached to a GitHub release.
type GitHubReleaseAsset struct {
	Name        string
	DownloadURL string
	ContentType string
	SizeBytes   int64
}

// AssetDownloader downloads an external asset to a local destination path.
//
// Implementations should write atomically when possible and honor context
// cancellation. Progress is optional; implementations should tolerate nil.
type AssetDownloader interface {
	Download(ctx context.Context, source DownloadSource, destinationPath string, progress DownloadProgress) (DownloadedFile, error)
}

// DownloadSource describes a remote file to download.
type DownloadSource struct {
	URL       string
	FileName  string
	SizeBytes int64
}

// DownloadedFile describes a completed download.
type DownloadedFile struct {
	Path      string
	SizeBytes int64
}

// DownloadProgress is an app-defined progress sink for byte downloads.
type DownloadProgress interface {
	Advance(delta int64)
	Set(current int64)
}
