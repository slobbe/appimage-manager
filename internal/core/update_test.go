package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestParseUpdateInfoStringZsync(t *testing.T) {
	info := "zsync|https://example.com/MyApp.AppImage.zsync"

	got, err := parseUpdateInfoString(info)
	if err != nil {
		t.Fatalf("parseUpdateInfoString returned error: %v", err)
	}

	if got.Kind != models.UpdateZsync {
		t.Fatalf("Kind = %q, want %q", got.Kind, models.UpdateZsync)
	}
	if got.Transport != "zsync" {
		t.Fatalf("Transport = %q, want %q", got.Transport, "zsync")
	}
	if got.UpdateUrl != "https://example.com/MyApp.AppImage.zsync" {
		t.Fatalf("UpdateUrl = %q, want %q", got.UpdateUrl, "https://example.com/MyApp.AppImage.zsync")
	}
	if got.UpdateInfo != info {
		t.Fatalf("UpdateInfo = %q, want %q", got.UpdateInfo, info)
	}
}

func TestParseUpdateInfoStringGitHubReleasesZsync(t *testing.T) {
	info := "gh-releases-zsync|owner|repo|v1.2.3|*-x86_64.AppImage.zsync"

	got, err := parseUpdateInfoString(info)
	if err != nil {
		t.Fatalf("parseUpdateInfoString returned error: %v", err)
	}

	if got.Kind != models.UpdateZsync {
		t.Fatalf("Kind = %q, want %q", got.Kind, models.UpdateZsync)
	}
	if got.Transport != "gh-releases" {
		t.Fatalf("Transport = %q, want %q", got.Transport, "gh-releases")
	}

	expectURL := "https://github.com/owner/repo/releases/download/v1.2.3/v1.2.3-x86_64.AppImage.zsync"
	if got.UpdateUrl != expectURL {
		t.Fatalf("UpdateUrl = %q, want %q", got.UpdateUrl, expectURL)
	}
	if got.UpdateInfo != info {
		t.Fatalf("UpdateInfo = %q, want %q", got.UpdateInfo, info)
	}
}

func TestParseUpdateInfoStringErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		errLike string
	}{
		{name: "empty", input: "", errLike: "empty update info"},
		{name: "invalid zsync format", input: "zsync", errLike: "invalid update info"},
		{name: "invalid gh-releases format", input: "gh-releases-zsync|owner|repo|v1.2.3", errLike: "invalid update info"},
		{name: "unsupported kind", input: "other|value", errLike: "unsupported update info kind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseUpdateInfoString(tt.input)
			if err == nil {
				t.Fatalf("parseUpdateInfoString(%q) expected error", tt.input)
			}
			if !strings.Contains(err.Error(), tt.errLike) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.errLike)
			}
		})
	}
}

func TestZsyncUpdateCheckDerivesNormalizedVersionFromFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Join([]string{
			"SHA-1: " + strings.Repeat("b", 40),
			"Filename: helium-v0.10.6.1-x86_64.AppImage",
			"",
			"payload ignored",
		}, "\n")))
	}))
	defer server.Close()

	update, err := ZsyncUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateZsync,
		Zsync: &models.ZsyncUpdateSource{
			UpdateInfo: "zsync|" + server.URL + "/helium.AppImage.zsync",
			Transport:  "zsync",
		},
	}, strings.Repeat("a", 40))
	if err != nil {
		t.Fatalf("ZsyncUpdateCheck returned error: %v", err)
	}

	if update.RemoteFilename != "helium-v0.10.6.1-x86_64.AppImage" {
		t.Fatalf("RemoteFilename = %q", update.RemoteFilename)
	}
	if update.NormalizedVersion != "0.10.6.1" {
		t.Fatalf("NormalizedVersion = %q, want %q", update.NormalizedVersion, "0.10.6.1")
	}
}
