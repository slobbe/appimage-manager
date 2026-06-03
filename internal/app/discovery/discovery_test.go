package discovery

import (
	"context"
	"fmt"
	"testing"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestGitHubBackendResolveUsesRepoMetadataAndRelease(t *testing.T) {
	resolver := fakeGitHubResolver{
		fetchRepository: func(ctx context.Context, repoSlug string) (*Repository, error) {
			if repoSlug != "obsidianmd/obsidian-releases" {
				t.Fatalf("unexpected repo metadata input: %s", repoSlug)
			}
			return &Repository{
				Name:        "obsidian-releases",
				Description: "Releases of Obsidian",
				HTMLURL:     "https://github.com/obsidianmd/obsidian-releases",
			}, nil
		},
		resolveSelection: func(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
			if repoSlug != "obsidianmd/obsidian-releases" || assetPattern != "*.AppImage" {
				t.Fatalf("unexpected resolve input: %s %s", repoSlug, assetPattern)
			}
			return &ReleaseAssetSelection{
				Release: &ReleaseAsset{
					DownloadURL: "https://example.com/Obsidian.AppImage",
					TagName:     "v1.12.4",
					AssetName:   "Obsidian-1.12.4.AppImage",
				},
			}, nil
		},
	}

	metadata, err := (GitHubBackend{Resolver: resolver}).Resolve(context.Background(), domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "obsidianmd/obsidian-releases"}, "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !metadata.Installable {
		t.Fatal("expected metadata to be installable")
	}
	if metadata.Name != "obsidian-releases" {
		t.Fatalf("metadata.Name = %q", metadata.Name)
	}
	if metadata.AssetName != "Obsidian-1.12.4.AppImage" {
		t.Fatalf("metadata.AssetName = %q", metadata.AssetName)
	}
}

func TestGitHubBackendResolvePreservesAmbiguousAssetCandidates(t *testing.T) {
	resolver := fakeGitHubResolver{
		fetchRepository: func(ctx context.Context, repoSlug string) (*Repository, error) {
			return &Repository{
				Name:        "example",
				Description: "Example",
				HTMLURL:     "https://github.com/owner/repo",
			}, nil
		},
		resolveSelection: func(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
			return &ReleaseAssetSelection{
				Ambiguous: true,
				Reason:    "multiple generic assets match",
				Candidates: []ReleaseAssetCandidate{
					{Name: "Example.AppImage", DownloadURL: "https://example.com/one", ArchLabel: "generic"},
					{Name: "Example-portable.AppImage", DownloadURL: "https://example.com/two", ArchLabel: "generic"},
				},
			}, nil
		},
	}

	metadata, err := (GitHubBackend{Resolver: resolver}).Resolve(context.Background(), domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}, "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !metadata.AssetAmbiguous {
		t.Fatal("expected ambiguous metadata")
	}
	if metadata.AssetReason != "multiple generic assets match" {
		t.Fatalf("AssetReason = %q", metadata.AssetReason)
	}
	if len(metadata.AssetCandidates) != 2 || metadata.AssetCandidates[1].Name != "Example-portable.AppImage" {
		t.Fatalf("unexpected candidates: %#v", metadata.AssetCandidates)
	}
}

func TestGitHubBackendResolveReturnsUnavailableMetadataForReleaseErrors(t *testing.T) {
	resolver := fakeGitHubResolver{
		resolveSelection: func(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
			return nil, fmt.Errorf("no assets match pattern")
		},
	}

	metadata, err := (GitHubBackend{Resolver: resolver}).Resolve(context.Background(), domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}, "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if metadata.Installable {
		t.Fatal("expected unavailable metadata")
	}
	if metadata.Provider != "GitHub" {
		t.Fatalf("Provider = %q", metadata.Provider)
	}
	if metadata.InstallReason != "no assets match pattern" {
		t.Fatalf("InstallReason = %q", metadata.InstallReason)
	}
	if metadata.RepoURL != "https://github.com/owner/repo" {
		t.Fatalf("RepoURL = %q", metadata.RepoURL)
	}
}

type fakeGitHubResolver struct {
	resolveSelection func(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error)
	fetchRepository  func(ctx context.Context, repoSlug string) (*Repository, error)
}

func (resolver fakeGitHubResolver) ResolveReleaseAssetSelection(repoSlug, assetPattern, arch string) (*ReleaseAssetSelection, error) {
	return resolver.resolveSelection(repoSlug, assetPattern, arch)
}

func (resolver fakeGitHubResolver) FetchRepository(ctx context.Context, repoSlug string) (*Repository, error) {
	return resolver.fetchRepository(ctx, repoSlug)
}
