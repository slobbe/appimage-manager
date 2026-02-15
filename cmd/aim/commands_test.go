package main

import (
	"path/filepath"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
	models "github.com/slobbe/appimage-manager/internal/types"
)

func TestIdentifyInputType(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "apps.json")

	originalDbSrc := config.DbSrc
	config.DbSrc = dbPath
	t.Cleanup(func() {
		config.DbSrc = originalDbSrc
	})

	db := &repo.DB{
		SchemaVersion: 1,
		Apps: map[string]*models.App{
			"integrated": {
				ID:               "integrated",
				DesktopEntryLink: "/tmp/integrated.desktop",
			},
			"unlinked": {
				ID:               "unlinked",
				DesktopEntryLink: "",
			},
		},
	}

	if err := repo.SaveDB(dbPath, db); err != nil {
		t.Fatalf("failed to write test DB: %v", err)
	}

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "local appimage path", input: "/tmp/MyApp.AppImage", expect: InputTypeAppImage},
		{name: "integrated id", input: "integrated", expect: InputTypeIntegrated},
		{name: "unlinked id", input: "unlinked", expect: InputTypeUnlinked},
		{name: "unknown id", input: "missing", expect: InputTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identifyInputType(tt.input)
			if got != tt.expect {
				t.Fatalf("identifyInputType(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestUpdateDownloadFilename(t *testing.T) {
	tests := []struct {
		name      string
		assetName string
		url       string
		expect    string
	}{
		{name: "uses AppImage asset name", assetName: "MyApp-x86_64.AppImage", url: "https://example.com/file", expect: "MyApp-x86_64.AppImage"},
		{name: "adds extension when missing", assetName: "MyApp", url: "https://example.com/file", expect: "MyApp.AppImage"},
		{name: "falls back to URL basename", assetName: "", url: "https://example.com/download/MyApp.AppImage", expect: "MyApp.AppImage"},
		{name: "falls back to default filename", assetName: "", url: "", expect: "update.AppImage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateDownloadFilename(tt.assetName, tt.url)
			if got != tt.expect {
				t.Fatalf("updateDownloadFilename(%q, %q) = %q, want %q", tt.assetName, tt.url, got, tt.expect)
			}
		})
	}
}
