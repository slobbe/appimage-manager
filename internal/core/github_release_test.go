package core

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	models "github.com/slobbe/appimage-manager/internal/types"
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

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "keeps plain semver", input: "1.2.3", expect: "1.2.3"},
		{name: "trims spaces and v prefix", input: "  v1.2.3  ", expect: "1.2.3"},
		{name: "normalizes version prefix", input: "Version 2.0.0", expect: "2.0.0"},
		{name: "extracts decorated package tag", input: "@standardnotes/desktop@3.201.19", expect: "3.201.19"},
		{name: "extracts release prefix version", input: "release-3.2.1", expect: "3.2.1"},
		{name: "extracts embedded v version", input: "desktop-v1.2.3", expect: "1.2.3"},
		{name: "strips linux arch suffix", input: "1.17.0-linux-x86-64", expect: "1.17.0"},
		{name: "strips arch suffix", input: "1.17.0-x86_64", expect: "1.17.0"},
		{name: "keeps prerelease", input: "v1.2.3-rc1", expect: "1.2.3-rc1"},
		{name: "keeps prerelease before packaging suffix", input: "1.17.0-rc1-linux-x86_64", expect: "1.17.0-rc1"},
		{name: "keeps dotted prerelease", input: "foo@1.2.3-beta.1", expect: "1.2.3-beta.1"},
		{name: "clears unknown", input: "unknown", expect: ""},
		{name: "handles empty", input: "", expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeVersion(tt.input)
			if got != tt.expect {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestNormalizeVersionUsesLastMatchingToken(t *testing.T) {
	got := normalizeVersion("pkg-2024.1@1.2.3")
	if got != "1.2.3" {
		t.Fatalf("normalizeVersion returned %q, want %q", got, "1.2.3")
	}
}

func TestSelectRelease(t *testing.T) {
	releases := []gitHubReleaseResponse{
		{TagName: "v3.0.0", Draft: true},
		{TagName: "v2.0.0-rc1", Prerelease: true},
		{TagName: "v1.0.0", Prerelease: false},
	}

	gotStable, okStable := selectRelease(releases, false)
	if !okStable {
		t.Fatal("selectRelease returned no result for stable selection")
	}
	if gotStable.TagName != "v1.0.0" {
		t.Fatalf("stable selectRelease picked %q, want %q", gotStable.TagName, "v1.0.0")
	}

	gotPre, okPre := selectRelease(releases, true)
	if !okPre {
		t.Fatal("selectRelease returned no result for prerelease selection")
	}
	if gotPre.TagName != "v2.0.0-rc1" {
		t.Fatalf("prerelease selectRelease picked %q, want %q", gotPre.TagName, "v2.0.0-rc1")
	}

	_, okNone := selectRelease([]gitHubReleaseResponse{{TagName: "v1", Draft: true}}, false)
	if okNone {
		t.Fatal("selectRelease should return no match when only drafts are present")
	}
}

func TestMatchAssetArchPreference(t *testing.T) {
	assets := []releaseAsset{
		{Name: "MyApp-arm64.AppImage", BrowserDownloadURL: "https://example.com/arm64"},
		{Name: "MyApp.AppImage", BrowserDownloadURL: "https://example.com/generic"},
		{Name: "MyApp-x86_64.AppImage", BrowserDownloadURL: "https://example.com/amd64"},
	}

	nameAMD64, urlAMD64 := matchAsset(assets, "*.AppImage", "amd64")
	if nameAMD64 != "MyApp-x86_64.AppImage" || urlAMD64 != "https://example.com/amd64" {
		t.Fatalf("amd64 selection got (%q, %q), want (%q, %q)", nameAMD64, urlAMD64, "MyApp-x86_64.AppImage", "https://example.com/amd64")
	}

	nameARM64, urlARM64 := matchAsset(assets, "*.AppImage", "arm64")
	if nameARM64 != "MyApp-arm64.AppImage" || urlARM64 != "https://example.com/arm64" {
		t.Fatalf("arm64 selection got (%q, %q), want (%q, %q)", nameARM64, urlARM64, "MyApp-arm64.AppImage", "https://example.com/arm64")
	}

	nameUnknown, urlUnknown := matchAsset(assets, "*.AppImage", "riscv64")
	if nameUnknown != "MyApp.AppImage" || urlUnknown != "https://example.com/generic" {
		t.Fatalf("unknown-arch selection got (%q, %q), want (%q, %q)", nameUnknown, urlUnknown, "MyApp.AppImage", "https://example.com/generic")
	}
}

func TestMatchAssetNoMatch(t *testing.T) {
	assets := []releaseAsset{
		{Name: "MyApp-x86_64.AppImage", BrowserDownloadURL: "https://example.com/amd64"},
	}

	name, url := matchAsset(assets, "*.zsync", "amd64")
	if name != "" || url != "" {
		t.Fatalf("matchAsset should return empty result for non-matching pattern, got (%q, %q)", name, url)
	}
}

func TestGitHubReleaseUpdateCheckNormalizesDecoratedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"@standardnotes/desktop@3.201.19","prerelease":false,"draft":false,"assets":[{"name":"StandardNotes-x86_64.AppImage","browser_download_url":"https://example.com/StandardNotes-x86_64.AppImage"}]}
		]`))
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
	})
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}, "3.201.19", "")
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if update.Available {
		t.Fatal("expected decorated matching tag not to be treated as update")
	}
	if update.NormalizedVersion != "3.201.19" {
		t.Fatalf("NormalizedVersion = %q", update.NormalizedVersion)
	}
}

func TestGitHubReleaseUpdateCheckDetectsRealUpdateFromDecoratedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"@standardnotes/desktop@3.202.0","prerelease":false,"draft":false,"assets":[{"name":"StandardNotes-x86_64.AppImage","browser_download_url":"https://example.com/StandardNotes-x86_64.AppImage"}]}
		]`))
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
	})
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}, "3.201.19", "")
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.NormalizedVersion != "3.202.0" {
		t.Fatalf("NormalizedVersion = %q", update.NormalizedVersion)
	}
}

func TestGitHubReleaseUpdateCheckIgnoresPackagingSuffixInCurrentVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.17.0","prerelease":false,"draft":false,"assets":[{"name":"LocalSend-1.17.0-linux-x86-64.AppImage","browser_download_url":"https://example.com/LocalSend-1.17.0-linux-x86-64.AppImage"}]}
		]`))
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
	})
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "localsend/localsend",
			Asset: "*.AppImage",
		},
	}, "1.17.0-linux-x86-64", "")
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if update.Available {
		t.Fatal("expected matching packaged version not to be treated as update")
	}
	if update.NormalizedVersion != "1.17.0" {
		t.Fatalf("NormalizedVersion = %q", update.NormalizedVersion)
	}
}

func TestGitHubReleaseUpdateCheckUsesSiblingZsyncTransport(t *testing.T) {
	const (
		assetURL      = "http://example.test/downloads/MyApp-x86_64.AppImage"
		expectedSHA1  = "0123456789abcdef0123456789abcdef01234567"
		currentSHA1   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		expectedZsync = assetURL + ".zsync"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","prerelease":false,"draft":false,"assets":[{"name":"MyApp-x86_64.AppImage","browser_download_url":"` + assetURL + `"}]}
			]`))
		case r.URL.String() == "/downloads/MyApp-x86_64.AppImage.zsync":
			_, _ = w.Write([]byte("Filename: MyApp-x86_64.AppImage\nSHA-1: " + expectedSHA1 + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
		http.DefaultTransport = originalTransport
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}
	http.DefaultTransport = &rewriteHostTransport{
		base: baseURL,
		next: server.Client().Transport,
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}, "1.0.0", currentSHA1)
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Transport != "zsync" {
		t.Fatalf("Transport = %q, want %q", update.Transport, "zsync")
	}
	if update.ZsyncURL != expectedZsync {
		t.Fatalf("ZsyncURL = %q, want %q", update.ZsyncURL, expectedZsync)
	}
	if update.ExpectedSHA1 != expectedSHA1 {
		t.Fatalf("ExpectedSHA1 = %q, want %q", update.ExpectedSHA1, expectedSHA1)
	}
}

func TestGitHubReleaseUpdateCheckIgnoresMissingSiblingZsync(t *testing.T) {
	const assetURL = "http://example.test/downloads/MyApp-x86_64.AppImage"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","prerelease":false,"draft":false,"assets":[{"name":"MyApp-x86_64.AppImage","browser_download_url":"` + assetURL + `"}]}
			]`))
		case r.URL.String() == "/downloads/MyApp-x86_64.AppImage.zsync":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
		http.DefaultTransport = originalTransport
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}
	http.DefaultTransport = &rewriteHostTransport{
		base: baseURL,
		next: server.Client().Transport,
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}, "1.0.0", "")
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil || !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Transport != "" || update.ZsyncURL != "" || update.ExpectedSHA1 != "" {
		t.Fatalf("expected empty zsync fields, got transport=%q url=%q sha1=%q", update.Transport, update.ZsyncURL, update.ExpectedSHA1)
	}
}

func TestGitHubReleaseUpdateCheckIgnoresMalformedSiblingZsync(t *testing.T) {
	const assetURL = "http://example.test/downloads/MyApp-x86_64.AppImage"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","prerelease":false,"draft":false,"assets":[{"name":"MyApp-x86_64.AppImage","browser_download_url":"` + assetURL + `"}]}
			]`))
		case r.URL.String() == "/downloads/MyApp-x86_64.AppImage.zsync":
			_, _ = w.Write([]byte("Filename: MyApp-x86_64.AppImage\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
		http.DefaultTransport = originalTransport
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}
	http.DefaultTransport = &rewriteHostTransport{
		base: baseURL,
		next: server.Client().Transport,
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}, "1.0.0", "")
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil || !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Transport != "" || update.ZsyncURL != "" || update.ExpectedSHA1 != "" {
		t.Fatalf("expected empty zsync fields, got transport=%q url=%q sha1=%q", update.Transport, update.ZsyncURL, update.ExpectedSHA1)
	}
}

func TestGitHubReleaseUpdateCheckSkipsSiblingZsyncWhenUpToDate(t *testing.T) {
	const assetURL = "http://example.test/downloads/MyApp-x86_64.AppImage"

	var zsyncHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","prerelease":false,"draft":false,"assets":[{"name":"MyApp-x86_64.AppImage","browser_download_url":"` + assetURL + `"}]}
			]`))
		case r.URL.String() == "/downloads/MyApp-x86_64.AppImage.zsync":
			atomic.AddInt32(&zsyncHits, 1)
			_, _ = w.Write([]byte("Filename: MyApp-x86_64.AppImage\nSHA-1: 0123456789abcdef0123456789abcdef01234567\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	originalClient := githubReleaseHTTPClient
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		githubReleaseHTTPClient = originalClient
		http.DefaultTransport = originalTransport
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}
	githubReleaseHTTPClient = &http.Client{
		Transport: &rewriteHostTransport{
			base: baseURL,
			next: server.Client().Transport,
		},
	}
	http.DefaultTransport = &rewriteHostTransport{
		base: baseURL,
		next: server.Client().Transport,
	}

	update, err := GitHubReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitHubRelease,
		GitHubRelease: &models.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}, "2.0.0", "")
	if err != nil {
		t.Fatalf("GitHubReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if update.Available {
		t.Fatal("expected no update")
	}
	if atomic.LoadInt32(&zsyncHits) != 0 {
		t.Fatalf("zsync hits = %d, want 0", zsyncHits)
	}
}
