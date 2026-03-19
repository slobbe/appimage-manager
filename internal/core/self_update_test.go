package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpgradeViaInstallerUsesPublishedScriptURL(t *testing.T) {
	originalRepoSlug := upgradeRepoSlug
	originalInstallScriptURL := upgradeInstallScriptURL
	originalRunInstallerScript := upgradeRunInstallerScript
	t.Cleanup(func() {
		upgradeRepoSlug = originalRepoSlug
		upgradeInstallScriptURL = originalInstallScriptURL
		upgradeRunInstallerScript = originalRunInstallerScript
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

	if err := UpgradeViaInstaller(context.Background()); err != nil {
		t.Fatalf("UpgradeViaInstaller returned error: %v", err)
	}
	if !called {
		t.Fatal("expected installer script runner to be called")
	}
}

func TestUpgradeViaInstallerHandlesNilContext(t *testing.T) {
	originalRunInstallerScript := upgradeRunInstallerScript
	t.Cleanup(func() {
		upgradeRunInstallerScript = originalRunInstallerScript
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

	if err := UpgradeViaInstaller(nil); err != nil {
		t.Fatalf("UpgradeViaInstaller returned error: %v", err)
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
