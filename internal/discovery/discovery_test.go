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

func TestParsePackageRef(t *testing.T) {
	tests := []struct {
		input     string
		expect    PackageRef
		wantError bool
	}{
		{input: "github:owner/repo", expect: PackageRef{Kind: ProviderGitHub, ProviderRef: "owner/repo"}},
		{input: "gitlab:group/project", expect: PackageRef{Kind: ProviderGitLab, ProviderRef: "group/project"}},
		{input: "1", wantError: true},
		{input: "github:owner", wantError: true},
		{input: "gitlab:group", wantError: true},
	}

	for _, tt := range tests {
		got, err := ParsePackageRef(tt.input)
		if tt.wantError {
			if err == nil {
				t.Fatalf("ParsePackageRef(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParsePackageRef(%q) returned error: %v", tt.input, err)
		}
		if got != tt.expect {
			t.Fatalf("ParsePackageRef(%q) = %#v, want %#v", tt.input, got, tt.expect)
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
