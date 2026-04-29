package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/slobbe/appimage-manager/internal/core"
)

type rewriteHostTransport struct {
	base *url.URL
	next http.RoundTripper
}

func (t *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.base.Scheme
	clone.URL.Host = t.base.Host
	clone.Host = t.base.Host
	return t.next.RoundTrip(clone)
}

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"obsidian-releases","full_name":"obsidianmd/obsidian-releases","description":"Releases of Obsidian","html_url":"https://github.com/obsidianmd/obsidian-releases","stargazers_count":10}`))
	}))
	defer server.Close()

	originalClient := githubDiscoveryHTTPClient
	originalResolve := resolveGitHubReleaseAssetFn
	t.Cleanup(func() {
		githubDiscoveryHTTPClient = originalClient
		resolveGitHubReleaseAssetFn = originalResolve
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubDiscoveryHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}
	resolveGitHubReleaseAssetFn = func(repoSlug, assetPattern string) (*core.GitHubReleaseAsset, error) {
		if repoSlug != "obsidianmd/obsidian-releases" || assetPattern != "*.AppImage" {
			t.Fatalf("unexpected resolve input: %s %s", repoSlug, assetPattern)
		}
		return &core.GitHubReleaseAsset{
			DownloadURL: "https://example.com/Obsidian.AppImage",
			TagName:     "v1.12.4",
			AssetName:   "Obsidian-1.12.4.AppImage",
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
