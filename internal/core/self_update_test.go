package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSelfUpgradeDevBuildRejected(t *testing.T) {
	_, err := SelfUpgrade(context.Background(), "dev")
	if err == nil {
		t.Fatal("expected error for dev build")
	}
}

func TestSelfUpgradeUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.4.1","assets":[{"name":"aim-linux-amd64.tar.gz","browser_download_url":"https://example.com/aim-linux-amd64.tar.gz"}]}`))
	}))
	defer server.Close()

	originalClient := selfUpdateHTTPClient
	originalArch := selfUpdateGOARCH
	originalRepo := selfUpdateRepoSlug
	originalURL := selfUpdateLatestReleaseURL
	originalInstall := selfUpdateInstall
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalClient
		selfUpdateGOARCH = originalArch
		selfUpdateRepoSlug = originalRepo
		selfUpdateLatestReleaseURL = originalURL
		selfUpdateInstall = originalInstall
	})

	selfUpdateHTTPClient = server.Client()
	selfUpdateGOARCH = "amd64"
	selfUpdateRepoSlug = "test/repo"
	selfUpdateLatestReleaseURL = func(string) string {
		return server.URL + "/releases/latest"
	}
	installCalled := false
	selfUpdateInstall = func(context.Context, string, string, string) error {
		installCalled = true
		return nil
	}

	result, err := SelfUpgrade(context.Background(), "v0.4.1")
	if err != nil {
		t.Fatalf("SelfUpgrade returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Updated {
		t.Fatal("expected no update when versions match")
	}
	if installCalled {
		t.Fatal("installer should not run when already up to date")
	}
}

func TestSelfUpgradeInstallsWhenNewer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.4.2","assets":[{"name":"aim-linux-amd64.tar.gz","browser_download_url":"https://example.com/aim-linux-amd64.tar.gz"}]}`))
	}))
	defer server.Close()

	originalClient := selfUpdateHTTPClient
	originalArch := selfUpdateGOARCH
	originalRepo := selfUpdateRepoSlug
	originalURL := selfUpdateLatestReleaseURL
	originalInstall := selfUpdateInstall
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalClient
		selfUpdateGOARCH = originalArch
		selfUpdateRepoSlug = originalRepo
		selfUpdateLatestReleaseURL = originalURL
		selfUpdateInstall = originalInstall
	})

	selfUpdateHTTPClient = server.Client()
	selfUpdateGOARCH = "amd64"
	selfUpdateRepoSlug = "test/repo"
	selfUpdateLatestReleaseURL = func(string) string {
		return server.URL + "/releases/latest"
	}

	called := false
	selfUpdateInstall = func(_ context.Context, assetURL, releaseVersion, arch string) error {
		called = true
		if assetURL != "https://example.com/aim-linux-amd64.tar.gz" {
			return fmt.Errorf("unexpected asset URL %q", assetURL)
		}
		if releaseVersion != "0.4.2" {
			return fmt.Errorf("unexpected release version %q", releaseVersion)
		}
		if arch != "amd64" {
			return fmt.Errorf("unexpected arch %q", arch)
		}
		return nil
	}

	result, err := SelfUpgrade(context.Background(), "0.4.1")
	if err != nil {
		t.Fatalf("SelfUpgrade returned error: %v", err)
	}
	if result == nil || !result.Updated {
		t.Fatal("expected successful update result")
	}
	if !called {
		t.Fatal("expected installer to run for newer version")
	}
}

func TestReleaseAssetNameForArch(t *testing.T) {
	tests := []struct {
		arch   string
		expect string
		err    bool
	}{
		{arch: "amd64", expect: "aim-linux-amd64.tar.gz"},
		{arch: "arm64", expect: "aim-linux-arm64.tar.gz"},
		{arch: "s390x", err: true},
	}

	for _, tt := range tests {
		name, err := releaseAssetNameForArch(tt.arch)
		if tt.err {
			if err == nil {
				t.Fatalf("releaseAssetNameForArch(%q) expected error", tt.arch)
			}
			continue
		}
		if err != nil {
			t.Fatalf("releaseAssetNameForArch(%q) returned error: %v", tt.arch, err)
		}
		if name != tt.expect {
			t.Fatalf("releaseAssetNameForArch(%q) = %q, want %q", tt.arch, name, tt.expect)
		}
	}
}

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		left   string
		right  string
		expect int
	}{
		{left: "0.4.2", right: "0.4.1", expect: 1},
		{left: "0.4.1", right: "0.4.1", expect: 0},
		{left: "0.4.1", right: "0.4.2", expect: -1},
		{left: "v1.0.0", right: "0.9.9", expect: 1},
	}

	for _, tt := range tests {
		got, err := compareSemanticVersions(tt.left, tt.right)
		if err != nil {
			t.Fatalf("compareSemanticVersions(%q, %q) returned error: %v", tt.left, tt.right, err)
		}
		if got != tt.expect {
			t.Fatalf("compareSemanticVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.expect)
		}
	}
}
