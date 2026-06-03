package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestReleaseTagFromJSONSelectsFirstNonDraftRelease(t *testing.T) {
	tag, err := latestReleaseTagFromJSON([]byte(`[
		{"tag_name":"v0.14.0-rc.1","draft":true,"prerelease":true},
		{"tag_name":"v0.13.0-rc.1","draft":false,"prerelease":true},
		{"tag_name":"v0.12.5","draft":false,"prerelease":false}
	]`))
	if err != nil {
		t.Fatalf("latestReleaseTagFromJSON returned error: %v", err)
	}
	if tag != "v0.13.0-rc.1" {
		t.Fatalf("tag = %q, want %q", tag, "v0.13.0-rc.1")
	}
}

func TestRunInstallerScriptRejectsBadStatusBeforeResolvingTempDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	calledTempDir := false
	err := (Client{HTTPClient: server.Client()}).RunInstallerScript(context.Background(), server.URL, func() (string, error) {
		calledTempDir = true
		return t.TempDir(), nil
	}, nil)
	if err == nil {
		t.Fatal("expected download status error")
	}
	if calledTempDir {
		t.Fatal("expected temp dir not to be resolved for failed download")
	}
	if !strings.Contains(err.Error(), "installer script download failed with status 502 Bad Gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInstallerScriptPassesEnvironment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#!/bin/sh\ntest \"$AIM_VERSION\" = \"v0.13.0-rc.1\"\n"))
	}))
	defer server.Close()

	if err := (Client{
		HTTPClient:   server.Client(),
		ShellCommand: "/bin/sh",
	}).RunInstallerScript(context.Background(), server.URL, func() (string, error) {
		return filepath.Join(t.TempDir(), "tmp"), nil
	}, map[string]string{"AIM_VERSION": "v0.13.0-rc.1"}); err != nil {
		t.Fatalf("RunInstallerScript returned error: %v", err)
	}
}

func TestRunInstallerScriptSurfacesFailureOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("#!/bin/sh\necho failure-output >&2\nexit 7\n"))
	}))
	defer server.Close()

	err := (Client{
		HTTPClient:   server.Client(),
		ShellCommand: "/bin/sh",
	}).RunInstallerScript(context.Background(), server.URL, func() (string, error) {
		return filepath.Join(t.TempDir(), "tmp"), nil
	}, nil)
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
