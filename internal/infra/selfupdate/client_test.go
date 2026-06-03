package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInstallerScriptRejectsBadStatusBeforeResolvingTempDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	calledTempDir := false
	err := (Client{HTTPClient: server.Client()}).RunInstallerScript(context.Background(), server.URL, func() (string, error) {
		calledTempDir = true
		return t.TempDir(), nil
	})
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
	})
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
