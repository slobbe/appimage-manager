package domain

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDesktopEntryAppInfoFallsBackToVersionFromFilename(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "0ad-0.28.0-x86_64.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=0 A.D.",
		"X-AppImage-Version=n/a",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "0ad")
	if appInfo.Version != "0.28.0" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "0.28.0")
	}
}

func TestParseDesktopEntryAppInfoUsesUnknownWhenVersionUnavailable(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "my-app.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=My App",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "my-app")
	if appInfo.Version != "unknown" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "unknown")
	}
}

func TestParseDesktopEntryAppInfoFallsBackToVersionBeforeStagedFilenameSuffix(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "handy_0.8.3_amd64-265e144b8aba0ca4.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=Handy",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "handy")
	if appInfo.Version != "0.8.3" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "0.8.3")
	}
}

func TestParseDesktopEntryAppInfoNormalizesStagedMetadataVersion(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "handy.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=Handy",
		"X-AppImage-Version=handy_0.8.3_amd64-265e144b8aba0ca4",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "handy")
	if appInfo.Version != "0.8.3" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "0.8.3")
	}
}

func TestParseDesktopEntryAppInfoNormalizesMetadataVersion(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "standard-notes.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=Standard Notes",
		"X-AppImage-Version=@standardnotes/desktop@3.201.19",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "standard-notes")
	if appInfo.Version != "3.201.19" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "3.201.19")
	}
}

func TestParseDesktopEntryAppInfoPrefersDesktopStemForID(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "t3-code-desktop.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=T3 Code (Alpha)",
		"X-AppImage-Version=0.0.14",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "t3-code-desktop")
	if appInfo.DesktopStem != "t3-code-desktop" {
		t.Fatalf("appInfo.DesktopStem = %q, want %q", appInfo.DesktopStem, "t3-code-desktop")
	}
	if appInfo.ID != "t3-code-desktop" {
		t.Fatalf("appInfo.ID = %q, want %q", appInfo.ID, "t3-code-desktop")
	}
}

func TestParseDesktopEntryAppInfoFallsBackToSlugifiedNameWhenDesktopStemInvalid(t *testing.T) {
	desktopPath := filepath.Join(t.TempDir(), "---.desktop")
	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=My App",
		"X-AppImage-Version=1.0.0",
	}, "\n") + "\n"

	appInfo := ParseDesktopEntryAppInfo(desktopPath, content, "")
	if appInfo.DesktopStem != "" {
		t.Fatalf("appInfo.DesktopStem = %q, want empty", appInfo.DesktopStem)
	}
	if appInfo.ID != "my-app" {
		t.Fatalf("appInfo.ID = %q, want %q", appInfo.ID, "my-app")
	}
}
