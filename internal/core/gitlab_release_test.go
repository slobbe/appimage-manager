package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestGitLabReleaseUpdateCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/group%2Fproject/releases" && r.URL.Path != "/api/v4/projects/group/project/releases" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[
			{"tag_name":"v2.0.0","upcoming_release":false,"assets":{"links":[{"name":"MyApp-x86_64.AppImage","direct_asset_url":"https://example.com/MyApp-x86_64.AppImage"}]}},
			{"tag_name":"v2.1.0","upcoming_release":true,"assets":{"links":[]}}
		]`))
	}))
	defer server.Close()

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "v1.0.0", "")
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.AssetName != "MyApp-x86_64.AppImage" {
		t.Fatalf("AssetName = %q", update.AssetName)
	}
	if update.DownloadURL != "https://example.com/MyApp-x86_64.AppImage" {
		t.Fatalf("DownloadURL = %q", update.DownloadURL)
	}
	if update.NormalizedVersion != "2.0.0" {
		t.Fatalf("NormalizedVersion = %q", update.NormalizedVersion)
	}
}

func TestGitLabReleaseUpdateCheckNormalizesDecoratedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"@standardnotes/desktop@3.201.19","upcoming_release":false,"assets":{"links":[{"name":"StandardNotes-x86_64.AppImage","direct_asset_url":"https://example.com/StandardNotes-x86_64.AppImage"}]}}
		]`))
	}))
	defer server.Close()

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "3.201.19", "")
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
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

func TestGitLabReleaseUpdateCheckIgnoresPackagingSuffixInCurrentVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.17.0","upcoming_release":false,"assets":{"links":[{"name":"LocalSend-1.17.0-linux-x86-64.AppImage","direct_asset_url":"https://example.com/LocalSend-1.17.0-linux-x86-64.AppImage"}]}}
		]`))
	}))
	defer server.Close()

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "1.17.0-linux-x86-64", "")
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
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

func TestGitLabReleaseUpdateCheckUsesSiblingZsyncTransport(t *testing.T) {
	const expectedSHA1 = "0123456789abcdef0123456789abcdef01234567"

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/group%2Fproject/releases", "/api/v4/projects/group/project/releases":
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","upcoming_release":false,"assets":{"links":[{"name":"MyApp-x86_64.AppImage","direct_asset_url":"` + serverURL + `/downloads/MyApp-x86_64.AppImage"}]}}
			]`))
		case "/downloads/MyApp-x86_64.AppImage.zsync":
			_, _ = w.Write([]byte("Filename: MyApp-x86_64.AppImage\nSHA-1: " + expectedSHA1 + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "v1.0.0", strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil || !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Transport != "zsync" {
		t.Fatalf("Transport = %q, want %q", update.Transport, "zsync")
	}
	if update.ZsyncURL != serverURL+"/downloads/MyApp-x86_64.AppImage.zsync" {
		t.Fatalf("ZsyncURL = %q", update.ZsyncURL)
	}
	if update.ExpectedSHA1 != expectedSHA1 {
		t.Fatalf("ExpectedSHA1 = %q", update.ExpectedSHA1)
	}
}

func TestGitLabReleaseUpdateCheckIgnoresMissingSiblingZsync(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/group%2Fproject/releases", "/api/v4/projects/group/project/releases":
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","upcoming_release":false,"assets":{"links":[{"name":"MyApp-x86_64.AppImage","direct_asset_url":"` + serverURL + `/downloads/MyApp-x86_64.AppImage"}]}}
			]`))
		case "/downloads/MyApp-x86_64.AppImage.zsync":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "v1.0.0", "")
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil || !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Transport != "" || update.ZsyncURL != "" || update.ExpectedSHA1 != "" {
		t.Fatalf("expected empty zsync fields, got transport=%q url=%q sha1=%q", update.Transport, update.ZsyncURL, update.ExpectedSHA1)
	}
}

func TestGitLabReleaseUpdateCheckIgnoresMalformedSiblingZsync(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/group%2Fproject/releases", "/api/v4/projects/group/project/releases":
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","upcoming_release":false,"assets":{"links":[{"name":"MyApp-x86_64.AppImage","direct_asset_url":"` + serverURL + `/downloads/MyApp-x86_64.AppImage"}]}}
			]`))
		case "/downloads/MyApp-x86_64.AppImage.zsync":
			_, _ = w.Write([]byte("Filename: MyApp-x86_64.AppImage\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "v1.0.0", "")
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
	}
	if update == nil || !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Transport != "" || update.ZsyncURL != "" || update.ExpectedSHA1 != "" {
		t.Fatalf("expected empty zsync fields, got transport=%q url=%q sha1=%q", update.Transport, update.ZsyncURL, update.ExpectedSHA1)
	}
}

func TestGitLabReleaseUpdateCheckSkipsSiblingZsyncWhenUpToDate(t *testing.T) {
	var zsyncHits int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/group%2Fproject/releases", "/api/v4/projects/group/project/releases":
			_, _ = w.Write([]byte(`[
				{"tag_name":"v2.0.0","upcoming_release":false,"assets":{"links":[{"name":"MyApp-x86_64.AppImage","direct_asset_url":"` + serverURL + `/downloads/MyApp-x86_64.AppImage"}]}}
			]`))
		case "/downloads/MyApp-x86_64.AppImage.zsync":
			atomic.AddInt32(&zsyncHits, 1)
			_, _ = w.Write([]byte("Filename: MyApp-x86_64.AppImage\nSHA-1: 0123456789abcdef0123456789abcdef01234567\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	originalBase := gitLabReleaseAPIBaseURL
	t.Cleanup(func() {
		gitLabReleaseAPIBaseURL = originalBase
	})
	gitLabReleaseAPIBaseURL = server.URL + "/api/v4"

	update, err := GitLabReleaseUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateGitLabRelease,
		GitLabRelease: &models.GitLabReleaseUpdateSource{
			Project: "group/project",
			Asset:   "*.AppImage",
		},
	}, "2.0.0", "")
	if err != nil {
		t.Fatalf("GitLabReleaseUpdateCheck returned error: %v", err)
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
