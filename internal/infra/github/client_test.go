package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientLatestReleaseMapsGitHubResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/repos/owner/repo/releases/latest"; got != want {
			t.Fatalf("request path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Accept"), "application/vnd.github+json"; got != want {
			t.Fatalf("Accept = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("X-GitHub-Api-Version"), "2022-11-28"; got != want {
			t.Fatalf("X-GitHub-Api-Version = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("User-Agent"), "aim"; got != want {
			t.Fatalf("User-Agent = %q, want %q", got, want)
		}

		fmt.Fprint(w, `{
			"tag_name": "v1.2.3",
			"name": "Release 1.2.3",
			"html_url": "https://github.example/owner/repo/releases/v1.2.3",
			"prerelease": true,
			"draft": false,
			"assets": [
				{
					"name": "Example-x86_64.AppImage",
					"browser_download_url": "https://downloads.example/Example-x86_64.AppImage",
					"content_type": "application/octet-stream",
					"size": 12345
				}
			]
		}`)
	}))
	defer server.Close()

	release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestRelease(context.Background(), "owner/repo", false)
	if err != nil {
		t.Fatalf("LatestRelease() error = %v", err)
	}

	if got, want := release.Repo, "owner/repo"; got != want {
		t.Fatalf("Repo = %q, want %q", got, want)
	}
	if got, want := release.TagName, "v1.2.3"; got != want {
		t.Fatalf("TagName = %q, want %q", got, want)
	}
	if got, want := release.Name, "Release 1.2.3"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := release.URL, "https://github.example/owner/repo/releases/v1.2.3"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if !release.Prerelease {
		t.Fatal("Prerelease = false, want true")
	}
	if release.Draft {
		t.Fatal("Draft = true, want false")
	}
	if len(release.Assets) != 1 {
		t.Fatalf("Assets len = %d, want 1", len(release.Assets))
	}
	asset := release.Assets[0]
	if got, want := asset.Name, "Example-x86_64.AppImage"; got != want {
		t.Fatalf("asset.Name = %q, want %q", got, want)
	}
	if got, want := asset.DownloadURL, "https://downloads.example/Example-x86_64.AppImage"; got != want {
		t.Fatalf("asset.DownloadURL = %q, want %q", got, want)
	}
	if got, want := asset.ContentType, "application/octet-stream"; got != want {
		t.Fatalf("asset.ContentType = %q, want %q", got, want)
	}
	if got, want := asset.SizeBytes, int64(12345); got != want {
		t.Fatalf("asset.SizeBytes = %d, want %d", got, want)
	}
}

func TestClientLatestReleaseIncludesPrereleases(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/repos/owner/repo/releases"; got != want {
			t.Fatalf("request path = %q, want %q", got, want)
		}
		fmt.Fprint(w, `[
			{
				"tag_name": "v2.0.0-beta.1",
				"name": "Beta",
				"html_url": "https://github.example/owner/repo/releases/v2.0.0-beta.1",
				"prerelease": true,
				"draft": false,
				"assets": []
			},
			{
				"tag_name": "v1.0.0",
				"name": "Stable",
				"prerelease": false,
				"draft": false,
				"assets": []
			}
		]`)
	}))
	defer server.Close()

	release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestRelease(context.Background(), "owner/repo", true)
	if err != nil {
		t.Fatalf("LatestRelease() error = %v", err)
	}
	if got, want := release.TagName, "v2.0.0-beta.1"; got != want {
		t.Fatalf("TagName = %q, want %q", got, want)
	}
	if !release.Prerelease {
		t.Fatal("Prerelease = false, want true")
	}
}

func TestClientLatestReleaseReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestRelease(context.Background(), "owner/repo", false)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("LatestRelease() error = %v, want 404 error", err)
	}
}

func TestClientLatestReleaseReturnsDecodeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{`)
	}))
	defer server.Close()

	_, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestRelease(context.Background(), "owner/repo", false)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("LatestRelease() error = %v, want decode error", err)
	}
}

func TestClientLatestReleaseValidatesRepo(t *testing.T) {
	t.Parallel()

	_, err := NewClient().LatestRelease(context.Background(), "owner/repo/extra", false)
	if err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("LatestRelease() error = %v, want repo format error", err)
	}
}
