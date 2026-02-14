package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slobbe/appimage-manager/internal/config"
)

func TestInstallDesktopIconPNGUsesThemeName(t *testing.T) {
	tmp := t.TempDir()
	setupIconDirsForTest(t, tmp)

	src := filepath.Join(tmp, "source.png")
	if err := os.WriteFile(src, []byte("icon"), 0o644); err != nil {
		t.Fatal(err)
	}

	installedPath, iconValue, err := InstallDesktopIcon("my-app", src)
	if err != nil {
		t.Fatalf("InstallDesktopIcon returned error: %v", err)
	}

	expected := filepath.Join(config.IconThemeDir, "256x256", "apps", "my-app.png")
	if installedPath != expected {
		t.Fatalf("installedPath = %q, want %q", installedPath, expected)
	}
	if iconValue != "my-app" {
		t.Fatalf("iconValue = %q, want %q", iconValue, "my-app")
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected installed icon missing: %v", err)
	}
}

func TestInstallDesktopIconSVGUsesScalableTheme(t *testing.T) {
	tmp := t.TempDir()
	setupIconDirsForTest(t, tmp)

	src := filepath.Join(tmp, "source.svg")
	if err := os.WriteFile(src, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	installedPath, iconValue, err := InstallDesktopIcon("my-app", src)
	if err != nil {
		t.Fatalf("InstallDesktopIcon returned error: %v", err)
	}

	expected := filepath.Join(config.IconThemeDir, "scalable", "apps", "my-app.svg")
	if installedPath != expected {
		t.Fatalf("installedPath = %q, want %q", installedPath, expected)
	}
	if iconValue != "my-app" {
		t.Fatalf("iconValue = %q, want %q", iconValue, "my-app")
	}
}

func TestInstallDesktopIconUnsupportedExtUsesAbsolutePath(t *testing.T) {
	tmp := t.TempDir()
	setupIconDirsForTest(t, tmp)

	src := filepath.Join(tmp, "source.ico")
	if err := os.WriteFile(src, []byte("icon"), 0o644); err != nil {
		t.Fatal(err)
	}

	installedPath, iconValue, err := InstallDesktopIcon("my-app", src)
	if err != nil {
		t.Fatalf("InstallDesktopIcon returned error: %v", err)
	}

	expected := filepath.Join(config.PixmapsDir, "my-app.ico")
	if installedPath != expected {
		t.Fatalf("installedPath = %q, want %q", installedPath, expected)
	}
	if iconValue != expected {
		t.Fatalf("iconValue = %q, want %q", iconValue, expected)
	}
}

func TestInstallDesktopIconErrors(t *testing.T) {
	tmp := t.TempDir()
	setupIconDirsForTest(t, tmp)

	if _, _, err := InstallDesktopIcon("", "/tmp/icon.png"); err == nil {
		t.Fatal("expected error for empty app id")
	}
	if _, _, err := InstallDesktopIcon("my-app", ""); err == nil {
		t.Fatal("expected error for empty icon source")
	}
}

func setupIconDirsForTest(t *testing.T, tmp string) {
	t.Helper()

	originalThemeDir := config.IconThemeDir
	originalPixmapsDir := config.PixmapsDir
	t.Cleanup(func() {
		config.IconThemeDir = originalThemeDir
		config.PixmapsDir = originalPixmapsDir
	})

	config.IconThemeDir = filepath.Join(tmp, "icons", "hicolor")
	config.PixmapsDir = filepath.Join(tmp, "pixmaps")
}
