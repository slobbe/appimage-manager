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
		if got, want := r.URL.Path, "/repos/owner/repo/releases"; got != want {
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

		fmt.Fprint(w, `[
				{
					"tag_name": "v1.2.3",
					"name": "Release 1.2.3",
					"html_url": "https://github.example/owner/repo/releases/v1.2.3",
					"prerelease": false,
					"draft": false,
					"assets": [
						{
							"name": "Example-x86_64.AppImage",
							"browser_download_url": "https://downloads.example/Example-x86_64.AppImage",
							"content_type": "application/octet-stream",
							"size": 12345
						}
					]
				}
			]`)
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
	if release.Prerelease {
		t.Fatal("Prerelease = true, want false")
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

func TestClientLatestReleaseSelectsNewestVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		includePrerelease bool
		response          string
		wantTag           string
		wantPrerelease    bool
	}{
		{
			name:              "stable only ignores newer prerelease",
			includePrerelease: false,
			response: `[
				{"tag_name": "v0.0.29-nightly", "name": "Nightly", "prerelease": true, "draft": false, "assets": []},
				{"tag_name": "v0.0.28", "name": "Stable", "prerelease": false, "draft": false, "assets": []}
			]`,
			wantTag: "v0.0.28",
		},
		{
			name:              "prerelease allowed prefers stable over same core nightly",
			includePrerelease: true,
			response: `[
				{"tag_name": "v0.0.28-nightly", "name": "Nightly", "prerelease": true, "draft": false, "assets": []},
				{"tag_name": "v0.0.28", "name": "Stable", "prerelease": false, "draft": false, "assets": []}
			]`,
			wantTag: "v0.0.28",
		},
		{
			name:              "prerelease allowed selects genuinely newer prerelease",
			includePrerelease: true,
			response: `[
				{"tag_name": "v0.0.29-nightly", "name": "Nightly", "prerelease": true, "draft": false, "assets": []},
				{"tag_name": "v0.0.28", "name": "Stable", "prerelease": false, "draft": false, "assets": []}
			]`,
			wantTag:        "v0.0.29-nightly",
			wantPrerelease: true,
		},
		{
			name:              "ignores drafts",
			includePrerelease: true,
			response: `[
				{"tag_name": "v9.0.0", "name": "Draft", "prerelease": false, "draft": true, "assets": []},
				{"tag_name": "v1.0.0", "name": "Stable", "prerelease": false, "draft": false, "assets": []}
			]`,
			wantTag: "v1.0.0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.URL.Path, "/repos/owner/repo/releases"; got != want {
					t.Fatalf("request path = %q, want %q", got, want)
				}
				fmt.Fprint(w, tt.response)
			}))
			defer server.Close()

			release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestRelease(context.Background(), "owner/repo", tt.includePrerelease)
			if err != nil {
				t.Fatalf("LatestRelease() error = %v", err)
			}
			if got := release.TagName; got != tt.wantTag {
				t.Fatalf("TagName = %q, want %q", got, tt.wantTag)
			}
			if got := release.Prerelease; got != tt.wantPrerelease {
				t.Fatalf("Prerelease = %v, want %v", got, tt.wantPrerelease)
			}
		})
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
