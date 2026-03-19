package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestParseGitLabProjectValue(t *testing.T) {
	tests := []struct {
		input     string
		expect    PackageRef
		wantError bool
	}{
		{input: "group/project", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/project"}},
		{input: "group/subgroup/project", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/subgroup/project"}},
		{input: "group", wantError: true},
		{input: "/group/project", wantError: true},
	}

	for _, tt := range tests {
		got, err := ParseGitLabProjectValue(tt.input)
		if tt.wantError {
			if err == nil {
				t.Fatalf("ParseGitLabProjectValue(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseGitLabProjectValue(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expect {
			t.Fatalf("ParseGitLabProjectValue(%q) = %#v, want %#v", tt.input, got, tt.expect)
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
		{input: "https://gitlab.com/group/project", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/project"}},
		{input: "https://www.gitlab.com/group/subgroup/project/-/releases/v1.0.0", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/subgroup/project"}},
		{input: "https://gitlab.com/group/project/-/blob/main/README.md", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/project"}},
		{input: "https://gitlab.com/group/subgroup/project", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/subgroup/project"}},
		{input: "https://github.com/owner", wantError: true},
		{input: "https://github.com/owner/repo/issues/1", wantError: true},
		{input: "https://github.com/owner/repo/releases/download/v1/App.AppImage", wantError: true},
		{input: "https://gitlab.com/group", wantError: true},
		{input: "https://gitlab.com/group/project/-/issues/1", wantError: true},
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

func TestGitLabBackendResolveUsesProjectMetadataAndRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/projects/group%2Fproject") || strings.Contains(r.URL.Path, "/projects/group/project") {
			_, _ = w.Write([]byte(`{"name":"Foo App","path_with_namespace":"group/project","description":"Great app","web_url":"https://gitlab.example/group/project","star_count":3}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	originalClient := gitLabDiscoveryHTTPClient
	originalBaseURL := gitLabDiscoveryAPIBaseURL
	originalResolve := resolveGitLabReleaseAssetFn
	t.Cleanup(func() {
		gitLabDiscoveryHTTPClient = originalClient
		gitLabDiscoveryAPIBaseURL = originalBaseURL
		resolveGitLabReleaseAssetFn = originalResolve
	})

	gitLabDiscoveryHTTPClient = server.Client()
	gitLabDiscoveryAPIBaseURL = server.URL
	resolveGitLabReleaseAssetFn = func(project, assetPattern string) (*core.GitLabReleaseAsset, error) {
		if project != "group/project" || assetPattern != "*.AppImage" {
			t.Fatalf("unexpected resolve input: %s %s", project, assetPattern)
		}
		return &core.GitLabReleaseAsset{
			DownloadURL: "https://example.com/Foo.AppImage",
			TagName:     "v2.0.0",
			AssetName:   "Foo-x86_64.AppImage",
		}, nil
	}

	metadata, err := (GitLabBackend{}).Resolve(context.Background(), PackageRef{Kind: ProviderGitLab, ProviderRef: "group/project"}, "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !metadata.Installable {
		t.Fatal("expected metadata to be installable")
	}
	if metadata.Name != "Foo App" {
		t.Fatalf("metadata.Name = %q", metadata.Name)
	}
	if metadata.AssetName != "Foo-x86_64.AppImage" {
		t.Fatalf("metadata.AssetName = %q", metadata.AssetName)
	}
}
