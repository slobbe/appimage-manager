package discovery

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type DiscoveryBackend interface {
	Name() string
	Resolve(ctx context.Context, ref domain.PackageRef, assetOverride string) (*domain.PackageMetadata, error)
}

type ReleaseAsset struct {
	DownloadURL       string
	TagName           string
	NormalizedVersion string
	AssetName         string
	PreRelease        bool
}

type ReleaseAssetCandidate struct {
	Name        string
	DownloadURL string
	Arch        string
	ArchLabel   string
}

type ReleaseAssetSelection struct {
	Release    *ReleaseAsset
	Candidates []ReleaseAssetCandidate
	Ambiguous  bool
	Reason     string
}

type Repository struct {
	Name        string
	Description string
	HTMLURL     string
}

type GitHubResolver interface {
	ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error)
	FetchRepository(ctx context.Context, repoSlug string) (*Repository, error)
}
