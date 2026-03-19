package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckForAimUpgradeReturnsUpdateWhenLatestIsNewer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(latestReleaseResponse{TagName: "v0.12.5"})
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	originalLatestReleaseURL := upgradeLatestReleaseURL
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
		upgradeLatestReleaseURL = originalLatestReleaseURL
	})
	upgradeHTTPClient = server.Client()
	upgradeLatestReleaseURL = func(string) string {
		return server.URL + "/releases/latest"
	}

	result, err := CheckForAimUpgrade(context.Background(), "0.12.4")
	if err != nil {
		t.Fatalf("CheckForAimUpgrade returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected upgrade check result")
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

func TestCheckForAimUpgradeReturnsNoUpdateWhenVersionsMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(latestReleaseResponse{TagName: "v0.12.5"})
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	originalLatestReleaseURL := upgradeLatestReleaseURL
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
		upgradeLatestReleaseURL = originalLatestReleaseURL
	})
	upgradeHTTPClient = server.Client()
	upgradeLatestReleaseURL = func(string) string {
		return server.URL
	}

	result, err := CheckForAimUpgrade(context.Background(), "0.12.5")
	if err != nil {
		t.Fatalf("CheckForAimUpgrade returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected upgrade check result")
	}
	if !result.Comparable {
		t.Fatal("expected comparable result")
	}
	if result.HasUpdate {
		t.Fatal("did not expect update to be available")
	}
}

func TestCheckForAimUpgradeTreatsDevAsNonComparable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(latestReleaseResponse{TagName: "v0.12.5"})
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	originalLatestReleaseURL := upgradeLatestReleaseURL
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
		upgradeLatestReleaseURL = originalLatestReleaseURL
	})
	upgradeHTTPClient = server.Client()
	upgradeLatestReleaseURL = func(string) string {
		return server.URL
	}

	result, err := CheckForAimUpgrade(context.Background(), "dev")
	if err != nil {
		t.Fatalf("CheckForAimUpgrade returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected upgrade check result")
	}
	if result.Comparable {
		t.Fatal("did not expect comparable result for dev")
	}
	if !result.HasUpdate {
		t.Fatal("expected upgrade path to remain allowed for dev")
	}
}

func TestCheckForAimUpgradeRejectsBadGitHubStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	originalLatestReleaseURL := upgradeLatestReleaseURL
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
		upgradeLatestReleaseURL = originalLatestReleaseURL
	})
	upgradeHTTPClient = server.Client()
	upgradeLatestReleaseURL = func(string) string {
		return server.URL
	}

	_, err := CheckForAimUpgrade(context.Background(), "0.12.4")
	if err == nil {
		t.Fatal("expected latest release request error")
	}
	if !strings.Contains(err.Error(), "latest release request failed with status 502 Bad Gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradeViaInstallerReturnsInstalledVersion(t *testing.T) {
	originalRepoSlug := upgradeRepoSlug
	originalInstallScriptURL := upgradeInstallScriptURL
	originalRunInstallerScript := upgradeRunInstallerScript
	originalExecutablePath := upgradeExecutablePath
	originalEvalSymlinks := upgradeEvalSymlinks
	originalRunVersionCommand := upgradeRunVersionCommand
	t.Cleanup(func() {
		upgradeRepoSlug = originalRepoSlug
		upgradeInstallScriptURL = originalInstallScriptURL
		upgradeRunInstallerScript = originalRunInstallerScript
		upgradeExecutablePath = originalExecutablePath
		upgradeEvalSymlinks = originalEvalSymlinks
		upgradeRunVersionCommand = originalRunVersionCommand
	})

	upgradeRepoSlug = "test/repo"
	upgradeInstallScriptURL = func(repoSlug string) string {
		if repoSlug != "test/repo" {
			t.Fatalf("unexpected repo slug %q", repoSlug)
		}
		return "https://raw.githubusercontent.com/test/repo/main/scripts/install.sh"
	}

	called := false
	upgradeRunInstallerScript = func(_ context.Context, scriptURL string) error {
		called = true
		if scriptURL != "https://raw.githubusercontent.com/test/repo/main/scripts/install.sh" {
			t.Fatalf("unexpected script URL %q", scriptURL)
		}
		return nil
	}
	upgradeExecutablePath = func() (string, error) {
		return "/tmp/aim", nil
	}
	upgradeEvalSymlinks = func(path string) (string, error) {
		if path != "/tmp/aim" {
			t.Fatalf("unexpected executable path %q", path)
		}
		return "/opt/bin/aim", nil
	}
	upgradeRunVersionCommand = func(ctx context.Context, binaryPath string) (string, error) {
		if ctx == nil {
			t.Fatal("expected non-nil context")
		}
		if binaryPath != "/opt/bin/aim" {
			t.Fatalf("unexpected binary path %q", binaryPath)
		}
		return "0.12.5\n", nil
	}

	result, err := UpgradeViaInstaller(context.Background(), "0.12.4")
	if err != nil {
		t.Fatalf("UpgradeViaInstaller returned error: %v", err)
	}
	if !called {
		t.Fatal("expected installer script runner to be called")
	}
	if result == nil {
		t.Fatal("expected upgrade result")
	}
	if result.PreviousVersion != "0.12.4" {
		t.Fatalf("PreviousVersion = %q, want %q", result.PreviousVersion, "0.12.4")
	}
	if result.InstalledVersion != "0.12.5" {
		t.Fatalf("InstalledVersion = %q, want %q", result.InstalledVersion, "0.12.5")
	}
}

func TestUpgradeViaInstallerHandlesNilContext(t *testing.T) {
	originalRunInstallerScript := upgradeRunInstallerScript
	originalExecutablePath := upgradeExecutablePath
	originalRunVersionCommand := upgradeRunVersionCommand
	t.Cleanup(func() {
		upgradeRunInstallerScript = originalRunInstallerScript
		upgradeExecutablePath = originalExecutablePath
		upgradeRunVersionCommand = originalRunVersionCommand
	})

	upgradeRunInstallerScript = func(ctx context.Context, scriptURL string) error {
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
	upgradeExecutablePath = func() (string, error) {
		return "/tmp/aim", nil
	}
	upgradeRunVersionCommand = func(ctx context.Context, binaryPath string) (string, error) {
		if ctx == nil {
			t.Fatal("expected non-nil context")
		}
		if binaryPath == "" {
			t.Fatal("expected binary path")
		}
		return "0.12.5", nil
	}

	result, err := UpgradeViaInstaller(nil, "0.12.4")
	if err != nil {
		t.Fatalf("UpgradeViaInstaller returned error: %v", err)
	}
	if result == nil || result.InstalledVersion != "0.12.5" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestUpgradeViaInstallerFallsBackWhenVersionProbeFails(t *testing.T) {
	originalRunInstallerScript := upgradeRunInstallerScript
	originalExecutablePath := upgradeExecutablePath
	originalRunVersionCommand := upgradeRunVersionCommand
	t.Cleanup(func() {
		upgradeRunInstallerScript = originalRunInstallerScript
		upgradeExecutablePath = originalExecutablePath
		upgradeRunVersionCommand = originalRunVersionCommand
	})

	upgradeRunInstallerScript = func(context.Context, string) error {
		return nil
	}
	upgradeExecutablePath = func() (string, error) {
		return "/tmp/aim", nil
	}
	upgradeRunVersionCommand = func(context.Context, string) (string, error) {
		return "", fmt.Errorf("version probe failed")
	}

	result, err := UpgradeViaInstaller(context.Background(), "0.12.4")
	if err != nil {
		t.Fatalf("UpgradeViaInstaller returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected upgrade result")
	}
	if result.PreviousVersion != "0.12.4" {
		t.Fatalf("PreviousVersion = %q, want %q", result.PreviousVersion, "0.12.4")
	}
	if result.InstalledVersion != "" {
		t.Fatalf("InstalledVersion = %q, want empty string", result.InstalledVersion)
	}
}

func TestUpgradeViaInstallerPropagatesInstallerFailure(t *testing.T) {
	originalRunInstallerScript := upgradeRunInstallerScript
	t.Cleanup(func() {
		upgradeRunInstallerScript = originalRunInstallerScript
	})

	upgradeRunInstallerScript = func(context.Context, string) error {
		return fmt.Errorf("installer failed")
	}

	result, err := UpgradeViaInstaller(context.Background(), "0.12.4")
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
	originalRunVersionCommand := upgradeRunVersionCommand
	t.Cleanup(func() {
		upgradeRunVersionCommand = originalRunVersionCommand
	})

	upgradeRunVersionCommand = func(context.Context, string) (string, error) {
		return "0.12.5\n", nil
	}

	version, err := readInstalledAimVersion(context.Background(), "/tmp/aim")
	if err != nil {
		t.Fatalf("readInstalledAimVersion returned error: %v", err)
	}
	if version != "0.12.5" {
		t.Fatalf("version = %q, want %q", version, "0.12.5")
	}
}

func TestCompareUpgradeVersions(t *testing.T) {
	tests := []struct {
		name    string
		left    string
		right   string
		expect  int
		wantErr bool
	}{
		{name: "newer", left: "0.12.5", right: "0.12.4", expect: 1},
		{name: "same", left: "0.12.5", right: "0.12.5", expect: 0},
		{name: "older", left: "0.12.4", right: "0.12.5", expect: -1},
		{name: "invalid", left: "dev", right: "0.12.5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compareUpgradeVersions(tt.left, tt.right)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("compareUpgradeVersions returned error: %v", err)
			}
			if got != tt.expect {
				t.Fatalf("compareUpgradeVersions(%q, %q) = %d, want %d", tt.left, tt.right, got, tt.expect)
			}
		})
	}
}

func TestRunInstallerScriptRejectsBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
	})
	upgradeHTTPClient = server.Client()

	err := runInstallerScript(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected download status error")
	}
	if !strings.Contains(err.Error(), "installer script download failed with status 502 Bad Gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInstallerScriptExecutesDownloadedScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	originalShellCommand := upgradeShellCommand
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
		upgradeShellCommand = originalShellCommand
	})
	upgradeHTTPClient = server.Client()
	upgradeShellCommand = "/bin/sh"

	if err := runInstallerScript(context.Background(), server.URL); err != nil {
		t.Fatalf("runInstallerScript returned error: %v", err)
	}
}

func TestRunInstallerScriptSurfacesFailureOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#!/bin/sh\necho failure-output >&2\nexit 7\n"))
	}))
	defer server.Close()

	originalHTTPClient := upgradeHTTPClient
	originalShellCommand := upgradeShellCommand
	t.Cleanup(func() {
		upgradeHTTPClient = originalHTTPClient
		upgradeShellCommand = originalShellCommand
	})
	upgradeHTTPClient = server.Client()
	upgradeShellCommand = "/bin/sh"

	err := runInstallerScript(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected execution failure")
	}
	if !strings.Contains(err.Error(), "upgrade via installer failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "failure-output") {
		t.Fatalf("expected installer output in error: %v", err)
	}
}
