package core

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestManifestUpdateCheckFlatPayload(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"v1.2.0","url":"https://example.com/MyApp.AppImage","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	}))
	defer server.Close()

	originalClient := manifestHTTPClient
	t.Cleanup(func() {
		manifestHTTPClient = originalClient
	})
	manifestHTTPClient = server.Client()

	update, err := ManifestUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateManifest,
		Manifest: &models.ManifestUpdateSource{
			URL: server.URL,
		},
	}, "v1.1.0", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatalf("ManifestUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if !update.Available {
		t.Fatal("expected update to be available")
	}
	if update.Version != "v1.2.0" {
		t.Fatalf("Version = %q", update.Version)
	}
}

func TestManifestUpdateCheckArchAssets(t *testing.T) {
	arch := runtime.GOARCH
	jsonBody := `{"version":"v2.0.0","assets":{"` + arch + `":{"url":"https://example.com/MyApp-` + arch + `.AppImage","sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}}}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer server.Close()

	originalClient := manifestHTTPClient
	t.Cleanup(func() {
		manifestHTTPClient = originalClient
	})
	manifestHTTPClient = server.Client()

	update, err := ManifestUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateManifest,
		Manifest: &models.ManifestUpdateSource{
			URL: server.URL,
		},
	}, "v2.0.0", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatalf("ManifestUpdateCheck returned error: %v", err)
	}
	if update == nil {
		t.Fatal("expected update response")
	}
	if update.Available {
		t.Fatal("expected no update when sha256 matches")
	}
}

func TestManifestUpdateCheckRequiresSHA256(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"v1.0.0","url":"https://example.com/MyApp.AppImage"}`))
	}))
	defer server.Close()

	originalClient := manifestHTTPClient
	t.Cleanup(func() {
		manifestHTTPClient = originalClient
	})
	manifestHTTPClient = server.Client()

	_, err := ManifestUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateManifest,
		Manifest: &models.ManifestUpdateSource{
			URL: server.URL,
		},
	}, "v1.0.0", "")
	if err == nil {
		t.Fatal("expected error when manifest sha256 is missing")
	}
}
