package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	models "github.com/slobbe/appimage-manager/internal/types"
)

func zsyncTestSource(url string) *models.UpdateSource {
	return &models.UpdateSource{
		Kind: models.UpdateZsync,
		Zsync: &models.ZsyncUpdateSource{
			UpdateInfo: "zsync|" + url,
			Transport:  "zsync",
		},
	}
}

func validZsyncMetadata(filename, sha1 string) string {
	return strings.Join([]string{
		"SHA-1: " + sha1,
		"Filename: " + filename,
		"",
		"payload ignored",
	}, "\n")
}

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
		_, _ = w.Write([]byte(validZsyncMetadata(
			"helium-v0.10.6.1-x86_64.AppImage",
			strings.Repeat("b", 40),
		)))
	}))
	defer server.Close()

	update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/helium.AppImage.zsync"), strings.Repeat("a", 40))
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

func TestZsyncUpdateCheckRejectsFailedHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/app.AppImage.zsync"), strings.Repeat("a", 40))
	if err == nil {
		t.Fatal("expected status error")
	}
	if update != nil {
		t.Fatalf("update = %#v, want nil", update)
	}
	if !strings.Contains(err.Error(), "status 500 Internal Server Error") {
		t.Fatalf("error = %q, want status substring", err.Error())
	}
}

func TestZsyncUpdateCheckUsesConfiguredHTTPClientTimeout(t *testing.T) {
	originalClient := sharedHTTPClient
	sharedHTTPClient = NewHTTPClient(20 * time.Millisecond)
	t.Cleanup(func() {
		sharedHTTPClient = originalClient
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(validZsyncMetadata("app.AppImage", strings.Repeat("b", 40))))
	}))
	defer server.Close()

	start := time.Now()
	update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/app.AppImage.zsync"), strings.Repeat("a", 40))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if update != nil {
		t.Fatalf("update = %#v, want nil", update)
	}
	if elapsed >= time.Second {
		t.Fatalf("elapsed = %s, want less than 1s", elapsed)
	}
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %q, want timeout substring", err.Error())
	}
}

func TestZsyncUpdateCheckRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing sha1",
			body: strings.Join([]string{
				"Filename: app.AppImage",
				"",
			}, "\n"),
		},
		{
			name: "missing filename",
			body: strings.Join([]string{
				"SHA-1: " + strings.Repeat("b", 40),
				"",
			}, "\n"),
		},
		{
			name: "unrelated headers",
			body: strings.Join([]string{
				"Version: 1.0.0",
				"",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/app.AppImage.zsync"), strings.Repeat("a", 40))
			if err == nil {
				t.Fatal("expected malformed metadata error")
			}
			if update != nil {
				t.Fatalf("update = %#v, want nil", update)
			}
			if !strings.Contains(err.Error(), "invalid zsync metadata") {
				t.Fatalf("error = %q, want invalid metadata substring", err.Error())
			}
		})
	}
}

func TestZsyncUpdateCheckRejectsRedirectStatusWhenClientDoesNotFollow(t *testing.T) {
	originalClient := sharedHTTPClient
	sharedHTTPClient = NewHTTPClient(30 * time.Second)
	sharedHTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	t.Cleanup(func() {
		sharedHTTPClient = originalClient
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app.AppImage.zsync", http.StatusFound)
	}))
	defer server.Close()

	update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/redirect.AppImage.zsync"), strings.Repeat("a", 40))
	if err == nil {
		t.Fatal("expected redirect status error")
	}
	if update != nil {
		t.Fatalf("update = %#v, want nil", update)
	}
	if !strings.Contains(err.Error(), "status 302 Found") {
		t.Fatalf("error = %q, want redirect status substring", err.Error())
	}
}

func TestZsyncUpdateCheckRejectsPartialMetadataResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SHA-1: " + strings.Repeat("b", 40) + "\n"))
	}))
	defer server.Close()

	update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/app.AppImage.zsync"), strings.Repeat("a", 40))
	if err == nil {
		t.Fatal("expected partial metadata error")
	}
	if update != nil {
		t.Fatalf("update = %#v, want nil", update)
	}
	if !strings.Contains(err.Error(), "invalid zsync metadata") {
		t.Fatalf("error = %q, want invalid metadata substring", err.Error())
	}
}

func TestZsyncUpdateCheckRejectsOversizedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", zsyncMetadataMaxBytes+1)))
	}))
	defer server.Close()

	update, err := ZsyncUpdateCheck(zsyncTestSource(server.URL+"/app.AppImage.zsync"), strings.Repeat("a", 40))
	if err == nil {
		t.Fatal("expected oversized metadata error")
	}
	if update != nil {
		t.Fatalf("update = %#v, want nil", update)
	}
	if !strings.Contains(err.Error(), "zsync metadata exceeds") {
		t.Fatalf("error = %q, want oversized metadata substring", err.Error())
	}
}
