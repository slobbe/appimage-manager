package core

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		_, _ = w.Write([]byte(`{"tag_name":"v0.4.1","assets":[{"name":"aim-v0.4.1-linux-amd64.tar.gz","browser_download_url":"https://example.com/aim-v0.4.1-linux-amd64.tar.gz"}]}`))
	}))
	defer server.Close()

	originalClient := selfUpdateHTTPClient
	originalArch := selfUpdateGOARCH
	originalRepo := selfUpdateRepoSlug
	originalURL := selfUpdateLatestReleaseURL
	originalInstall := selfUpdateInstall
	originalRunInstaller := selfUpdateRunInstaller
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalClient
		selfUpdateGOARCH = originalArch
		selfUpdateRepoSlug = originalRepo
		selfUpdateLatestReleaseURL = originalURL
		selfUpdateInstall = originalInstall
		selfUpdateRunInstaller = originalRunInstaller
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
	runInstallerCalled := false
	selfUpdateRunInstaller = func(context.Context, string) error {
		runInstallerCalled = true
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
	if runInstallerCalled {
		t.Fatal("installer fallback should not run when already up to date")
	}
}

func TestSelfUpgradeInstallsWhenNewer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.4.2","assets":[{"name":"aim-v0.4.2-linux-amd64.tar.gz","browser_download_url":"https://example.com/aim-v0.4.2-linux-amd64.tar.gz"}]}`))
	}))
	defer server.Close()

	originalClient := selfUpdateHTTPClient
	originalArch := selfUpdateGOARCH
	originalRepo := selfUpdateRepoSlug
	originalURL := selfUpdateLatestReleaseURL
	originalInstall := selfUpdateInstall
	originalRunInstaller := selfUpdateRunInstaller
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalClient
		selfUpdateGOARCH = originalArch
		selfUpdateRepoSlug = originalRepo
		selfUpdateLatestReleaseURL = originalURL
		selfUpdateInstall = originalInstall
		selfUpdateRunInstaller = originalRunInstaller
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
		if assetURL != "https://example.com/aim-v0.4.2-linux-amd64.tar.gz" {
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
	runInstallerCalled := false
	selfUpdateRunInstaller = func(context.Context, string) error {
		runInstallerCalled = true
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
	if runInstallerCalled {
		t.Fatal("installer fallback should not run when built-in install succeeds")
	}
}

func TestSelfUpgradeFallsBackToInstallerScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.12.1","assets":[{"name":"aim-v0.12.1-linux-amd64.tar.gz","browser_download_url":"https://example.com/aim-v0.12.1-linux-amd64.tar.gz"}]}`))
	}))
	defer server.Close()

	originalClient := selfUpdateHTTPClient
	originalArch := selfUpdateGOARCH
	originalRepo := selfUpdateRepoSlug
	originalURL := selfUpdateLatestReleaseURL
	originalInstall := selfUpdateInstall
	originalRunInstaller := selfUpdateRunInstaller
	originalScriptURL := selfUpdateInstallScriptURL
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalClient
		selfUpdateGOARCH = originalArch
		selfUpdateRepoSlug = originalRepo
		selfUpdateLatestReleaseURL = originalURL
		selfUpdateInstall = originalInstall
		selfUpdateRunInstaller = originalRunInstaller
		selfUpdateInstallScriptURL = originalScriptURL
	})

	selfUpdateHTTPClient = server.Client()
	selfUpdateGOARCH = "amd64"
	selfUpdateRepoSlug = "test/repo"
	selfUpdateLatestReleaseURL = func(string) string {
		return server.URL + "/releases/latest"
	}

	selfUpdateInstall = func(context.Context, string, string, string) error {
		return fmt.Errorf("no release binary found in archive")
	}

	called := false
	selfUpdateInstallScriptURL = func(repoSlug string) string {
		if repoSlug != "test/repo" {
			t.Fatalf("unexpected repo slug %q", repoSlug)
		}
		return "https://raw.githubusercontent.com/test/repo/main/scripts/install.sh"
	}
	selfUpdateRunInstaller = func(_ context.Context, scriptURL string) error {
		called = true
		if scriptURL != "https://raw.githubusercontent.com/test/repo/main/scripts/install.sh" {
			return fmt.Errorf("unexpected script URL %q", scriptURL)
		}
		return nil
	}

	result, err := SelfUpgrade(context.Background(), "0.11.0")
	if err != nil {
		t.Fatalf("SelfUpgrade returned error: %v", err)
	}
	if !called {
		t.Fatal("expected installer fallback to run")
	}
	if result == nil || !result.Updated {
		t.Fatal("expected successful update result")
	}
	if !result.UsedInstallerFallback {
		t.Fatal("expected installer fallback flag to be set")
	}
}

func TestSelfUpgradeInstallerFallbackFailureReturnsCombinedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.12.1","assets":[{"name":"aim-v0.12.1-linux-amd64.tar.gz","browser_download_url":"https://example.com/aim-v0.12.1-linux-amd64.tar.gz"}]}`))
	}))
	defer server.Close()

	originalClient := selfUpdateHTTPClient
	originalArch := selfUpdateGOARCH
	originalRepo := selfUpdateRepoSlug
	originalURL := selfUpdateLatestReleaseURL
	originalInstall := selfUpdateInstall
	originalRunInstaller := selfUpdateRunInstaller
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalClient
		selfUpdateGOARCH = originalArch
		selfUpdateRepoSlug = originalRepo
		selfUpdateLatestReleaseURL = originalURL
		selfUpdateInstall = originalInstall
		selfUpdateRunInstaller = originalRunInstaller
	})

	selfUpdateHTTPClient = server.Client()
	selfUpdateGOARCH = "amd64"
	selfUpdateRepoSlug = "test/repo"
	selfUpdateLatestReleaseURL = func(string) string {
		return server.URL + "/releases/latest"
	}

	selfUpdateInstall = func(context.Context, string, string, string) error {
		return fmt.Errorf("no release binary found in archive")
	}
	selfUpdateRunInstaller = func(context.Context, string) error {
		return fmt.Errorf("permission denied")
	}

	_, err := SelfUpgrade(context.Background(), "0.11.0")
	if err == nil {
		t.Fatal("expected combined error")
	}
	if !strings.Contains(err.Error(), "built-in self-upgrade failed: no release binary found in archive") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "installer fallback failed: permission denied") {
		t.Fatalf("unexpected error: %v", err)
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

func TestReleaseVersionedAssetPatternForArch(t *testing.T) {
	tests := []struct {
		arch   string
		expect string
		err    bool
	}{
		{arch: "amd64", expect: "aim-?*-linux-amd64.tar.gz"},
		{arch: "arm64", expect: "aim-?*-linux-arm64.tar.gz"},
		{arch: "s390x", err: true},
	}

	for _, tt := range tests {
		name, err := releaseVersionedAssetPatternForArch(tt.arch)
		if tt.err {
			if err == nil {
				t.Fatalf("releaseVersionedAssetPatternForArch(%q) expected error", tt.arch)
			}
			continue
		}
		if err != nil {
			t.Fatalf("releaseVersionedAssetPatternForArch(%q) returned error: %v", tt.arch, err)
		}
		if name != tt.expect {
			t.Fatalf("releaseVersionedAssetPatternForArch(%q) = %q, want %q", tt.arch, name, tt.expect)
		}
	}
}

func TestFindReleaseAssetURLPrefersVersionedThenLegacy(t *testing.T) {
	assets := []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		{Name: "aim-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/legacy"},
		{Name: "aim-v1.2.3-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/versioned"},
	}

	url, err := findReleaseAssetURL(assets, "aim-?*-linux-amd64.tar.gz", "aim-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("findReleaseAssetURL returned error: %v", err)
	}
	if url != "https://example.com/versioned" {
		t.Fatalf("url = %q, want %q", url, "https://example.com/versioned")
	}

	onlyLegacy := []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		{Name: "aim-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.com/legacy"},
	}

	url, err = findReleaseAssetURL(onlyLegacy, "aim-?*-linux-amd64.tar.gz", "aim-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("findReleaseAssetURL legacy fallback returned error: %v", err)
	}
	if url != "https://example.com/legacy" {
		t.Fatalf("legacy url = %q, want %q", url, "https://example.com/legacy")
	}
}

func TestExtractReleaseBinarySupportsRootAndBinLayouts(t *testing.T) {
	tests := []struct {
		name        string
		archivePath string
	}{
		{name: "root layout", archivePath: "aim-0.12.0-linux-amd64"},
		{name: "bin layout", archivePath: "bin/aim-0.12.0-linux-amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			archivePath := filepath.Join(tempDir, "release.tar.gz")
			outputPath := filepath.Join(tempDir, "aim-new")

			if err := writeTestReleaseArchive(archivePath, map[string]string{
				tt.archivePath:                          "binary payload",
				"share/man/man1/aim.1":                  "man page",
				"share/bash-completion/completions/aim": "completion",
			}); err != nil {
				t.Fatalf("failed to write test archive: %v", err)
			}

			if err := extractReleaseBinary(archivePath, outputPath, "amd64"); err != nil {
				t.Fatalf("extractReleaseBinary returned error: %v", err)
			}

			got, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("failed to read extracted binary: %v", err)
			}
			if string(got) != "binary payload" {
				t.Fatalf("extracted payload = %q, want %q", string(got), "binary payload")
			}
		})
	}
}

func TestLegacyRootOnlyExtractorSupportsCompatibilityArchive(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "release.tar.gz")
	outputPath := filepath.Join(tempDir, "aim-old")

	if err := writeTestReleaseArchive(archivePath, map[string]string{
		"aim-0.12.1-linux-amd64":     "root binary payload",
		"bin/aim-0.12.1-linux-amd64": "bin binary payload",
	}); err != nil {
		t.Fatalf("failed to write test archive: %v", err)
	}

	if err := extractReleaseBinaryRootOnlyForTest(archivePath, outputPath, "amd64"); err != nil {
		t.Fatalf("legacy extractor returned error: %v", err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read extracted binary: %v", err)
	}
	if string(got) != "root binary payload" {
		t.Fatalf("extracted payload = %q, want %q", string(got), "root binary payload")
	}
}

func TestExtractReleaseBinaryRejectsUnexpectedPaths(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "release.tar.gz")
	outputPath := filepath.Join(tempDir, "aim-new")

	if err := writeTestReleaseArchive(archivePath, map[string]string{
		"share/bin/aim-0.12.0-linux-amd64": "wrong place",
	}); err != nil {
		t.Fatalf("failed to write test archive: %v", err)
	}

	err := extractReleaseBinary(archivePath, outputPath, "amd64")
	if err == nil || err.Error() != "no release binary found in archive" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeTestReleaseArchive(archivePath string, files map[string]string) error {
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	gzWriter := gzip.NewWriter(f)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	for name, contents := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(contents)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := io.WriteString(tarWriter, contents); err != nil {
			return err
		}
	}

	return nil
}

func extractReleaseBinaryRootOnlyForTest(archivePath, outputPath, arch string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		name := filepath.Clean(header.Name)
		if filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
			continue
		}

		baseName := filepath.Base(name)
		if baseName != name {
			continue
		}
		if !isReleaseBinaryName(baseName, arch) {
			continue
		}

		out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}

		if _, err := io.Copy(out, tarReader); err != nil {
			_ = out.Close()
			return err
		}

		return out.Close()
	}

	return fmt.Errorf("no release binary found in archive")
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
