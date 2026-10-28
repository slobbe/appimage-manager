package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
)

func TestClientLatestReleaseMatchingTagSelection(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name              string
		pattern           string
		includePrerelease bool
		body              string
		wantTag           string
	}{
		{
			name: "stable skips drafts prereleases and unmatched releases", pattern: "v1.*",
			body:    `[{"tag_name":"v1.9","draft":true},{"tag_name":"v1.8","prerelease":true},{"tag_name":"v2.0"},{"tag_name":"v1.2"},{"tag_name":"v1.7"}]`,
			wantTag: "v1.2",
		},
		{
			name: "prereleases allowed but drafts excluded", pattern: "v1.*", includePrerelease: true,
			body:    `[{"tag_name":"v1.9","draft":true,"prerelease":true},{"tag_name":"v1.2-beta","prerelease":true},{"tag_name":"v1.7"}]`,
			wantTag: "v1.2-beta",
		},
		{
			name: "including prereleases still allows stable", pattern: "v*", includePrerelease: true,
			body: `[{"tag_name":"v1.0"},{"tag_name":"v2.0-beta","prerelease":true}]`, wantTag: "v1.0",
		},
		{
			name: "case sensitive and tag not title", pattern: "v1.*",
			body: `[{"tag_name":"V1.9"},{"tag_name":"v2.0","name":"v1.8"},{"tag_name":"v1.2","name":"Different title"}]`, wantTag: "v1.2",
		},
		{
			name: "character class and question mark", pattern: "v[12].?",
			body: `[{"tag_name":"v3.0"},{"tag_name":"v2.10"},{"tag_name":"v2.4"}]`, wantTag: "v2.4",
		},
		{
			name: "whole tag not substring", pattern: "v1",
			body: `[{"tag_name":"prefix-v1"},{"tag_name":"v1-suffix"},{"tag_name":"v1"}]`, wantTag: "v1",
		},
		{
			name: "wildcards do not cross separators", pattern: "release/*",
			body: `[{"tag_name":"release/nested/v1"},{"tag_name":"release/v1"}]`, wantTag: "release/v1",
		},
		{
			name: "escaped star and star in class", pattern: `v\*[*]`,
			body: `[{"tag_name":"v12"},{"tag_name":"v**"}]`, wantTag: "v**",
		},
		{
			name: "pattern whitespace is significant", pattern: " v* ",
			body: `[{"tag_name":"v1"},{"tag_name":" v1 "}]`, wantTag: " v1 ",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/owner/repo/releases" {
					t.Errorf("path = %q, want releases list", r.URL.Path)
				}
				for key, want := range map[string]string{"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "aim"} {
					if got := r.Header.Get(key); got != want {
						t.Errorf("%s = %q, want %q", key, got, want)
					}
				}
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestReleaseMatchingTag(context.Background(), " owner/repo ", tt.includePrerelease, tt.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if release.TagName != tt.wantTag || release.Repo != "owner/repo" || release.Draft {
				t.Fatalf("release = %+v, want non-draft %s in owner/repo", release, tt.wantTag)
			}
		})
	}
}

func TestClientLatestReleaseMatchingTagPagination(t *testing.T) {
	t.Parallel()
	for _, match := range []bool{true, false} {
		t.Run(fmt.Sprintf("match=%t", match), func(t *testing.T) {
			t.Parallel()
			var pages []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/owner/repo/releases" {
					t.Errorf("unexpected path %q (no fallback allowed)", r.URL.Path)
				}
				page := r.URL.Query().Get("page")
				pages = append(pages, page)
				switch page {
				case "":
					w.Header().Set("Link", `<?page=3>; rel="last", <?page=2>; rel="next"`)
					fmt.Fprint(w, `[{"tag_name":"other","name":"v1.0"}]`)
				case "2":
					w.Header().Add("Link", `<?page=1>; rel="prev"`)
					w.Header().Add("Link", fmt.Sprintf(`<http://%s/repos/owner/repo/releases?page=3>; rel="next"`, r.Host))
					fmt.Fprint(w, `[{"tag_name":"v1.9","draft":true},{"tag_name":"v1.8","prerelease":true}]`)
				case "3":
					if match {
						w.Header().Set("Link", `<?page=4>; rel="next"`)
						fmt.Fprint(w, `[{"tag_name":"v1.2"},{"tag_name":"v1.7"}]`)
					} else {
						w.Header().Set("Link", `<?page=1>; rel="first", <?page=2>; rel="prev"`)
						fmt.Fprint(w, `[]`)
					}
				default:
					t.Errorf("unexpected page %q: must stop at first match", page)
					fmt.Fprint(w, `[]`)
				}
			}))
			defer server.Close()
			release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestReleaseMatchingTag(context.Background(), "owner/repo", false, "v1.*")
			if match {
				if err != nil || release.TagName != "v1.2" {
					t.Fatalf("release = %+v, error = %v", release, err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "no eligible releases") || !strings.Contains(err.Error(), "owner/repo") || !strings.Contains(err.Error(), "v1.*") {
					t.Fatalf("error = %v, want contextual no-match error", err)
				}
				if !reflect.DeepEqual(release, app.GitHubRelease{}) {
					t.Fatalf("release = %+v, want zero value", release)
				}
			}
			if !reflect.DeepEqual(pages, []string{"", "2", "3"}) {
				t.Fatalf("pages = %v, want first three pages only", pages)
			}
		})
	}
}

func TestClientLatestReleaseMatchingTagMalformedPattern(t *testing.T) {
	t.Parallel()
	for _, pattern := range []string{"[", "[]", "[^]", "[a-]", "\\", "v*later*[", "v*later*\\", "v*[abc", "v*[a-]", `v\**later*[`, `v[*]*later*[`, "v[é-]"} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("invalid pattern must be rejected before HTTP request")
				fmt.Fprint(w, `[]`)
			}))
			defer server.Close()
			_, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestReleaseMatchingTag(context.Background(), "owner/repo", false, pattern)
			if !errors.Is(err, filepath.ErrBadPattern) || !strings.Contains(err.Error(), "tag pattern") {
				t.Fatalf("error = %v, want invalid tag pattern", err)
			}
		})
	}
}

func TestClientLatestReleaseMatchingTagErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "empty list", status: 200, body: `[]`, want: "no eligible releases"},
		{name: "title only", status: 200, body: `[{"tag_name":"other","name":"v1.0"}]`, want: "no eligible releases"},
		{name: "empty pattern is not a fallback", status: 200, body: `[{"tag_name":"v1.0"}]`, want: "no eligible releases"},
		{name: "http", status: 404, body: `missing`, want: "404"},
		{name: "decode", status: 200, body: `{`, want: "decode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			pattern := "v*"
			if tt.name == "empty pattern is not a fallback" {
				pattern = ""
			}
			_, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestReleaseMatchingTag(context.Background(), "owner/repo", false, pattern)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestClientLatestReleaseMatchingTagLaterPageErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "http", status: http.StatusForbidden, body: "forbidden", want: "403"},
		{name: "decode", status: http.StatusOK, body: `{`, want: "decode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "" {
					w.Header().Set("Link", `<?page=2>; rel="next"`)
					fmt.Fprint(w, `[]`)
					return
				}
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			_, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestReleaseMatchingTag(context.Background(), "owner/repo", false, "v*")
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), "no eligible releases") {
				t.Fatalf("error = %v, want page error containing %q, not no-match", err, tt.want)
			}
		})
	}
}

func TestClientLatestReleaseMatchingTagMapsResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v1.2","name":"Title","html_url":"https://example.com/release","prerelease":true,"published_at":"2026-10-26T12:34:56Z","assets":[{"name":"app.AppImage","browser_download_url":"https://example.com/app.AppImage","content_type":"application/octet-stream","size":123}]}]`)
	}))
	defer server.Close()
	release, err := (Client{BaseURL: server.URL, HTTPClient: server.Client()}).LatestReleaseMatchingTag(context.Background(), "owner/repo", true, "v*")
	if err != nil {
		t.Fatal(err)
	}
	want := app.GitHubRelease{
		Repo: "owner/repo", TagName: "v1.2", Name: "Title", URL: "https://example.com/release", Prerelease: true,
		PublishedAt: time.Date(2026, time.October, 26, 12, 34, 56, 0, time.UTC),
		Assets:      []app.GitHubReleaseAsset{{Name: "app.AppImage", DownloadURL: "https://example.com/app.AppImage", ContentType: "application/octet-stream", SizeBytes: 123}},
	}
	if !reflect.DeepEqual(release, want) {
		t.Fatalf("release = %+v, want %+v", release, want)
	}
}

func TestClientLatestReleaseMatchingTagValidatesRepoAndContext(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected HTTP request")
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := client.LatestReleaseMatchingTag(context.Background(), "owner/repo/extra", false, "*"); err == nil || !strings.Contains(err.Error(), "owner/repo") {
		t.Fatalf("error = %v, want invalid repo", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.LatestReleaseMatchingTag(ctx, "owner/repo", false, "*"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
