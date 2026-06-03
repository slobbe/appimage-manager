package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type testLatestReleaseResponse struct {
	TagName string `json:"tag_name"`
}

type fakeSelfUpdater struct {
	fetchLatestReleaseTag func(context.Context, string) (string, error)
	runInstallerScript    func(context.Context, string, func() (string, error), map[string]string) error
	resolveInstalledPath  func() (string, error)
	readInstalledVersion  func(context.Context, string) (string, error)
}

func (f fakeSelfUpdater) FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error) {
	if f.fetchLatestReleaseTag != nil {
		return f.fetchLatestReleaseTag(ctx, releaseURL)
	}
	return "", fmt.Errorf("fetch latest release tag not configured")
}

func (f fakeSelfUpdater) RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error), env map[string]string) error {
	if f.runInstallerScript != nil {
		return f.runInstallerScript(ctx, scriptURL, tempDir, env)
	}
	return fmt.Errorf("run installer script not configured")
}

func (f fakeSelfUpdater) ResolveInstalledPath() (string, error) {
	if f.resolveInstalledPath != nil {
		return f.resolveInstalledPath()
	}
	return "", fmt.Errorf("resolve installed path not configured")
}

func (f fakeSelfUpdater) ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	if f.readInstalledVersion != nil {
		return f.readInstalledVersion(ctx, binaryPath)
	}
	return "", fmt.Errorf("read installed version not configured")
}

func TestCheckForAimSelfUpdateReturnsUpdateWhenLatestIsNewer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(testLatestReleaseResponse{TagName: "v0.12.5"})
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	t.Cleanup(func() { selfUpdateHTTPClient = originalHTTPClient })
	selfUpdateHTTPClient = server.Client()
	service := NewService(Service{SelfUpdater: testSelfUpdater{}, LatestReleaseURL: func(string) string { return server.URL + "/releases/latest" }})

	result, err := service.Check(context.Background(), "0.12.4", false)
	if err != nil {
		t.Fatalf("CheckForAimSelfUpdate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected self-update check result")
	}
	if !result.Comparable {
		t.Fatal("expected comparable result")
	}
	if !result.HasUpdate {
		t.Fatal("expected update to be available")
	}
	if result.LatestVersion != "v0.12.5" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "v0.12.5")
	}
}

func TestCheckForAimSelfUpdateDetectsNewerPrerelease(t *testing.T) {
	updater := fakeSelfUpdater{
		fetchLatestReleaseTag: func(context.Context, string) (string, error) {
			return "v0.17.1-pre.3", nil
		},
	}
	service := NewService(Service{SelfUpdater: updater})

	result, err := service.Check(context.Background(), "0.17.1-pre.2", true)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected self-update check result")
	}
	if !result.Comparable {
		t.Fatal("expected comparable result")
	}
	if !result.HasUpdate {
		t.Fatalf("HasUpdate = false, want true for %+v", result)
	}
}

func TestCheckForAimSelfUpdateReportsCurrentAheadOfLatestStable(t *testing.T) {
	updater := fakeSelfUpdater{
		fetchLatestReleaseTag: func(context.Context, string) (string, error) {
			return "v0.17.0", nil
		},
	}
	service := NewService(Service{SelfUpdater: updater})

	result, err := service.Check(context.Background(), "0.17.1-pre.4", false)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected self-update check result")
	}
	if result.HasUpdate {
		t.Fatalf("HasUpdate = true, want false for %+v", result)
	}
	if !result.CurrentAhead {
		t.Fatalf("CurrentAhead = false, want true for %+v", result)
	}
}

func TestCheckForAimSelfUpdateReturnsNoUpdateWhenVersionsMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testLatestReleaseResponse{TagName: "v0.12.5"})
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	t.Cleanup(func() { selfUpdateHTTPClient = originalHTTPClient })
	selfUpdateHTTPClient = server.Client()
	service := NewService(Service{SelfUpdater: testSelfUpdater{}, LatestReleaseURL: func(string) string { return server.URL }})

	result, err := service.Check(context.Background(), "0.12.5", false)
	if err != nil {
		t.Fatalf("CheckForAimSelfUpdate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected self-update check result")
	}
	if !result.Comparable {
		t.Fatal("expected comparable result")
	}
	if result.HasUpdate {
		t.Fatal("did not expect update to be available")
	}
}

func TestCheckForAimSelfUpdateTreatsDevAsNonComparable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testLatestReleaseResponse{TagName: "v0.12.5"})
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	t.Cleanup(func() { selfUpdateHTTPClient = originalHTTPClient })
	selfUpdateHTTPClient = server.Client()
	service := NewService(Service{SelfUpdater: testSelfUpdater{}, LatestReleaseURL: func(string) string { return server.URL }})

	result, err := service.Check(context.Background(), "dev", false)
	if err != nil {
		t.Fatalf("CheckForAimSelfUpdate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected self-update check result")
	}
	if result.Comparable {
		t.Fatal("did not expect comparable result for dev")
	}
	if !result.HasUpdate {
		t.Fatal("expected self-update path to remain allowed for dev")
	}
}

func TestCheckForAimSelfUpdateUsesReleaseListWhenIncludingPrereleases(t *testing.T) {
	updater := fakeSelfUpdater{
		fetchLatestReleaseTag: func(_ context.Context, releaseURL string) (string, error) {
			if releaseURL != "https://api.github.com/repos/test/repo/releases?per_page=100" {
				t.Fatalf("release URL = %q", releaseURL)
			}
			return "v0.13.0-rc.1", nil
		},
	}
	service := NewService(Service{RepoSlug: "test/repo", SelfUpdater: updater})

	result, err := service.Check(context.Background(), "0.12.5", true)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result == nil || result.LatestVersion != "v0.13.0-rc.1" || !result.HasUpdate {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckForAimSelfUpdateRejectsBadGitHubStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	t.Cleanup(func() { selfUpdateHTTPClient = originalHTTPClient })
	selfUpdateHTTPClient = server.Client()
	service := NewService(Service{SelfUpdater: testSelfUpdater{}, LatestReleaseURL: func(string) string { return server.URL }})

	_, err := service.Check(context.Background(), "0.12.4", false)
	if err == nil {
		t.Fatal("expected latest release request error")
	}
	if !strings.Contains(err.Error(), "latest release request failed with status 502 Bad Gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelfUpdateInstallsAndReturnsInstalledVersion(t *testing.T) {
	called := false
	updater := fakeSelfUpdater{
		runInstallerScript: func(_ context.Context, scriptURL string, _ func() (string, error), env map[string]string) error {
			called = true
			if scriptURL != "https://raw.githubusercontent.com/test/repo/main/scripts/install.sh" {
				t.Fatalf("unexpected script URL %q", scriptURL)
			}
			if env["AIM_VERSION"] != "v0.12.5" {
				t.Fatalf("AIM_VERSION = %q, want %q", env["AIM_VERSION"], "v0.12.5")
			}
			return nil
		},
		resolveInstalledPath: func() (string, error) { return "/opt/bin/aim", nil },
		readInstalledVersion: func(ctx context.Context, binaryPath string) (string, error) {
			if ctx == nil {
				t.Fatal("expected non-nil context")
			}
			if binaryPath != "/opt/bin/aim" {
				t.Fatalf("unexpected binary path %q", binaryPath)
			}
			return "0.12.5", nil
		},
	}
	service := NewService(Service{RepoSlug: "test/repo", SelfUpdater: updater})

	result, err := service.SelfUpdate(context.Background(), InstallerSelfUpdateRequest{CurrentVersion: "0.12.4", TargetVersion: "v0.12.5"})
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if !called {
		t.Fatal("expected installer script runner to be called")
	}
	if result == nil {
		t.Fatal("expected self-update result")
	}
	if result.PreviousVersion != "0.12.4" {
		t.Fatalf("PreviousVersion = %q, want %q", result.PreviousVersion, "0.12.4")
	}
	if result.InstalledVersion != "0.12.5" {
		t.Fatalf("InstalledVersion = %q, want %q", result.InstalledVersion, "0.12.5")
	}
}

func TestSelfUpdateHandlesNilContext(t *testing.T) {
	updater := fakeSelfUpdater{}
	updater.runInstallerScript = func(ctx context.Context, scriptURL string, _ func() (string, error), _ map[string]string) error {
		if ctx == nil {
			t.Fatal("expected non-nil context")
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("unexpected context error: %v", err)
		}
		if scriptURL == "" {
			t.Fatal("expected installer script URL")
		}
		return nil
	}
	updater.resolveInstalledPath = func() (string, error) {
		return "/tmp/aim", nil
	}
	updater.readInstalledVersion = func(ctx context.Context, binaryPath string) (string, error) {
		if ctx == nil {
			t.Fatal("expected non-nil context")
		}
		if binaryPath == "" {
			t.Fatal("expected binary path")
		}
		return "0.12.5", nil
	}

	service := NewService(Service{SelfUpdater: updater})
	var nilContext context.Context
	result, err := service.SelfUpdate(nilContext, InstallerSelfUpdateRequest{CurrentVersion: "0.12.4"})
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if result == nil || result.InstalledVersion != "0.12.5" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSelfUpdateFallsBackWhenVersionProbeFails(t *testing.T) {
	updater := fakeSelfUpdater{
		runInstallerScript:   func(context.Context, string, func() (string, error), map[string]string) error { return nil },
		resolveInstalledPath: func() (string, error) { return "/tmp/aim", nil },
		readInstalledVersion: func(context.Context, string) (string, error) {
			return "", fmt.Errorf("version probe failed")
		},
	}
	service := NewService(Service{SelfUpdater: updater})

	result, err := service.SelfUpdate(context.Background(), InstallerSelfUpdateRequest{CurrentVersion: "0.12.4"})
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected self-update result")
	}
	if result.PreviousVersion != "0.12.4" {
		t.Fatalf("PreviousVersion = %q, want %q", result.PreviousVersion, "0.12.4")
	}
	if result.InstalledVersion != "" {
		t.Fatalf("InstalledVersion = %q, want empty string", result.InstalledVersion)
	}
}

func TestSelfUpdatePropagatesInstallerFailure(t *testing.T) {
	updater := fakeSelfUpdater{runInstallerScript: func(context.Context, string, func() (string, error), map[string]string) error {
		return fmt.Errorf("installer failed")
	}}
	service := NewService(Service{SelfUpdater: updater})

	result, err := service.SelfUpdate(context.Background(), InstallerSelfUpdateRequest{CurrentVersion: "0.12.4"})
	if err == nil {
		t.Fatal("expected installer failure")
	}
	if !strings.Contains(err.Error(), "installer failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
}

func TestReadInstalledAimVersionTrimsWhitespace(t *testing.T) {
	originalRunVersionCommand := selfUpdateRunVersionCommand
	t.Cleanup(func() { selfUpdateRunVersionCommand = originalRunVersionCommand })
	selfUpdateRunVersionCommand = func(context.Context, string) (string, error) { return "0.12.5\n", nil }

	version, err := (testSelfUpdater{}).ReadInstalledVersion(context.Background(), "/tmp/aim")
	if err != nil {
		t.Fatalf("readInstalledAimVersion returned error: %v", err)
	}
	if version != "0.12.5" {
		t.Fatalf("version = %q, want %q", version, "0.12.5")
	}
}

func TestRunInstallerScriptRejectsBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalHTTPClient
	})
	selfUpdateHTTPClient = server.Client()

	err := (testSelfUpdater{}).RunInstallerScript(context.Background(), server.URL, func() (string, error) { return t.TempDir(), nil }, nil)
	if err == nil {
		t.Fatal("expected download status error")
	}
	if !strings.Contains(err.Error(), "installer script download failed with status 502 Bad Gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInstallerScriptExecutesDownloadedScript(t *testing.T) {
	tempDir := setupSelfUpdatePathsForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	originalShellCommand := selfUpdateShellCommand
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalHTTPClient
		selfUpdateShellCommand = originalShellCommand
	})
	selfUpdateHTTPClient = server.Client()
	selfUpdateShellCommand = "/bin/sh"

	if err := (testSelfUpdater{}).RunInstallerScript(context.Background(), server.URL, func() (string, error) { return tempDir, nil }, nil); err != nil {
		t.Fatalf("selfUpdateRunInstallerScript returned error: %v", err)
	}
}

func TestRunInstallerScriptSurfacesFailureOutput(t *testing.T) {
	tempDir := setupSelfUpdatePathsForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#!/bin/sh\necho failure-output >&2\nexit 7\n"))
	}))
	defer server.Close()

	originalHTTPClient := selfUpdateHTTPClient
	originalShellCommand := selfUpdateShellCommand
	t.Cleanup(func() {
		selfUpdateHTTPClient = originalHTTPClient
		selfUpdateShellCommand = originalShellCommand
	})
	selfUpdateHTTPClient = server.Client()
	selfUpdateShellCommand = "/bin/sh"

	err := (testSelfUpdater{}).RunInstallerScript(context.Background(), server.URL, func() (string, error) { return tempDir, nil }, nil)
	if err == nil {
		t.Fatal("expected execution failure")
	}
	if !strings.Contains(err.Error(), "self-update via installer failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "failure-output") {
		t.Fatalf("expected installer output in error: %v", err)
	}
}

func setupSelfUpdatePathsForTest(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "cache", "tmp")
}
