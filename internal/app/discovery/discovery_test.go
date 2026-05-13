package discovery

import (
	"context"
	"testing"

	"github.com/slobbe/appimage-manager/internal/infra/github"
)

func TestParseGitHubRepoValue(t *testing.T) {
	tests := []struct {
		input     string
		expect    PackageRef
		wantError bool
	}{
		{input: "owner/repo", expect: PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "owner", wantError: true},
		{input: "/owner/repo", wantError: true},
	}

	for _, tt := range tests {
		got, err := ParseGitHubRepoValue(tt.input)
		if tt.wantError {
			if err == nil {
				t.Fatalf("ParseGitHubRepoValue(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseGitHubRepoValue(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expect {
			t.Fatalf("ParseGitHubRepoValue(%q) = %#v, want %#v", tt.input, got, tt.expect)
		}
	}
}

func TestParsePackageRefURL(t *testing.T) {
	tests := []struct {
		input     string
		expect    PackageRef
		wantError bool
	}{
		{input: "https://github.com/owner/repo", expect: PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://www.github.com/owner/repo/releases", expect: PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://github.com/owner/repo/releases/tag/v1.2.3?tab=readme#fragment", expect: PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://github.com/owner/repo/blob/main/README.md", expect: PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "https://github.com/owner", wantError: true},
		{input: "https://github.com/owner/repo/issues/1", wantError: true},
		{input: "https://github.com/owner/repo/releases/download/v1/App.AppImage", wantError: true},
		{input: "https://example.com/owner/repo", wantError: true},
		{input: "http://github.com/owner/repo", wantError: true},
	}

	for _, tt := range tests {
		got, err := ParsePackageRefURL(tt.input)
		if tt.wantError {
			if err == nil {
				t.Fatalf("ParsePackageRefURL(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParsePackageRefURL(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expect {
			t.Fatalf("ParsePackageRefURL(%q) = %#v, want %#v", tt.input, got, tt.expect)
		}
	}
}

func TestGitHubBackendResolveUsesRepoMetadataAndRelease(t *testing.T) {
	originalResolveSelection := resolveGitHubReleaseAssetSelectionFn
	originalFetchRepository := fetchGitHubRepositoryFn
	t.Cleanup(func() {
		resolveGitHubReleaseAssetSelectionFn = originalResolveSelection
		fetchGitHubRepositoryFn = originalFetchRepository
	})

	fetchGitHubRepositoryFn = func(ctx context.Context, repoSlug string) (*github.Repository, error) {
		if repoSlug != "obsidianmd/obsidian-releases" {
			t.Fatalf("unexpected repo metadata input: %s", repoSlug)
		}
		return &github.Repository{
			Name:            "obsidian-releases",
			FullName:        "obsidianmd/obsidian-releases",
			Description:     "Releases of Obsidian",
			HTMLURL:         "https://github.com/obsidianmd/obsidian-releases",
			StargazersCount: 10,
		}, nil
	}
	resolveGitHubReleaseAssetSelectionFn = func(repoSlug, assetPattern, arch string) (*github.ReleaseAssetSelection, error) {
		if repoSlug != "obsidianmd/obsidian-releases" || assetPattern != "*.AppImage" {
			t.Fatalf("unexpected resolve input: %s %s", repoSlug, assetPattern)
		}
		return &github.ReleaseAssetSelection{
			Release: &github.ReleaseAsset{
				DownloadURL: "https://example.com/Obsidian.AppImage",
				TagName:     "v1.12.4",
				AssetName:   "Obsidian-1.12.4.AppImage",
			},
		}, nil
	}

	metadata, err := (GitHubBackend{}).Resolve(context.Background(), PackageRef{Kind: ProviderGitHub, ProviderRef: "obsidianmd/obsidian-releases"}, "")
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
	originalResolveSelection := resolveGitHubReleaseAssetSelectionFn
	originalFetchRepository := fetchGitHubRepositoryFn
	t.Cleanup(func() {
		resolveGitHubReleaseAssetSelectionFn = originalResolveSelection
		fetchGitHubRepositoryFn = originalFetchRepository
	})

	fetchGitHubRepositoryFn = func(ctx context.Context, repoSlug string) (*github.Repository, error) {
		return &github.Repository{
			Name:        "example",
			FullName:    "owner/repo",
			Description: "Example",
			HTMLURL:     "https://github.com/owner/repo",
		}, nil
	}
	resolveGitHubReleaseAssetSelectionFn = func(repoSlug, assetPattern, arch string) (*github.ReleaseAssetSelection, error) {
		return &github.ReleaseAssetSelection{
			Ambiguous: true,
			Reason:    "multiple generic assets match",
			Candidates: []github.ReleaseAssetCandidate{
				{Name: "Example.AppImage", DownloadURL: "https://example.com/one", ArchLabel: "generic"},
				{Name: "Example-portable.AppImage", DownloadURL: "https://example.com/two", ArchLabel: "generic"},
			},
		}, nil
	}

	metadata, err := (GitHubBackend{}).Resolve(context.Background(), PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}, "")
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
