package core

import (
	"net/http"
	"net/http/httptest"
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
	}, "v1.0.0")
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
}
