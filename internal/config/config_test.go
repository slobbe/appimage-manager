package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathsDefaults(t *testing.T) {
	home := "/home/alice"

	paths := resolvePaths(home, func(string) string { return "" })

	if paths.AimDir != filepath.Join(home, ".local", "share", "aim") {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.DesktopDir != filepath.Join(home, ".local", "share", "applications") {
		t.Fatalf("DesktopDir = %q", paths.DesktopDir)
	}
	if paths.TempDir != filepath.Join(home, ".cache", "aim", "tmp") {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.ConfigDir != filepath.Join(home, ".config", "aim") {
		t.Fatalf("ConfigDir = %q", paths.ConfigDir)
	}
	if paths.DbSrc != filepath.Join(home, ".local", "state", "aim", "apps.json") {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
	if paths.IconThemeDir != filepath.Join(home, ".local", "share", "icons", "hicolor") {
		t.Fatalf("IconThemeDir = %q", paths.IconThemeDir)
	}
	if paths.PixmapsDir != filepath.Join(home, ".local", "share", "pixmaps") {
		t.Fatalf("PixmapsDir = %q", paths.PixmapsDir)
	}
}

func TestResolvePathsXDGOverrides(t *testing.T) {
	home := "/home/alice"
	env := map[string]string{
		"XDG_DATA_HOME":   "/xdg/data",
		"XDG_CONFIG_HOME": "/xdg/config",
		"XDG_CACHE_HOME":  "/xdg/cache",
		"XDG_STATE_HOME":  "/xdg/state",
	}

	paths := resolvePaths(home, func(key string) string {
		return env[key]
	})

	if paths.AimDir != "/xdg/data/aim" {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.DesktopDir != "/xdg/data/applications" {
		t.Fatalf("DesktopDir = %q", paths.DesktopDir)
	}
	if paths.TempDir != "/xdg/cache/aim/tmp" {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.ConfigDir != "/xdg/config/aim" {
		t.Fatalf("ConfigDir = %q", paths.ConfigDir)
	}
	if paths.DbSrc != "/xdg/state/aim/apps.json" {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
	if paths.IconThemeDir != "/xdg/data/icons/hicolor" {
		t.Fatalf("IconThemeDir = %q", paths.IconThemeDir)
	}
	if paths.PixmapsDir != "/xdg/data/pixmaps" {
		t.Fatalf("PixmapsDir = %q", paths.PixmapsDir)
	}
}

func TestResolvePathsIgnoresRelativeXDGPaths(t *testing.T) {
	home := "/home/alice"
	env := map[string]string{
		"XDG_DATA_HOME":   "relative/data",
		"XDG_CONFIG_HOME": "relative/config",
		"XDG_CACHE_HOME":  "relative/cache",
		"XDG_STATE_HOME":  "relative/state",
	}

	paths := resolvePaths(home, func(key string) string {
		return env[key]
	})

	if paths.AimDir != filepath.Join(home, ".local", "share", "aim") {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.TempDir != filepath.Join(home, ".cache", "aim", "tmp") {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.ConfigDir != filepath.Join(home, ".config", "aim") {
		t.Fatalf("ConfigDir = %q", paths.ConfigDir)
	}
	if paths.DbSrc != filepath.Join(home, ".local", "state", "aim", "apps.json") {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
	if paths.IconThemeDir != filepath.Join(home, ".local", "share", "icons", "hicolor") {
		t.Fatalf("IconThemeDir = %q", paths.IconThemeDir)
	}
	if paths.PixmapsDir != filepath.Join(home, ".local", "share", "pixmaps") {
		t.Fatalf("PixmapsDir = %q", paths.PixmapsDir)
	}
}

func TestResolvePathsFromStateRoot(t *testing.T) {
	paths := ResolvePathsFromStateRoot("/tmp/aim-root")

	if paths.AimDir != "/tmp/aim-root/data/aim" {
		t.Fatalf("AimDir = %q", paths.AimDir)
	}
	if paths.DesktopDir != "/tmp/aim-root/data/applications" {
		t.Fatalf("DesktopDir = %q", paths.DesktopDir)
	}
	if paths.ConfigDir != "/tmp/aim-root/config/aim" {
		t.Fatalf("ConfigDir = %q", paths.ConfigDir)
	}
	if paths.TempDir != "/tmp/aim-root/cache/aim/tmp" {
		t.Fatalf("TempDir = %q", paths.TempDir)
	}
	if paths.DbSrc != "/tmp/aim-root/state/aim/apps.json" {
		t.Fatalf("DbSrc = %q", paths.DbSrc)
	}
	if paths.IconThemeDir != "/tmp/aim-root/data/icons/hicolor" {
		t.Fatalf("IconThemeDir = %q", paths.IconThemeDir)
	}
	if paths.PixmapsDir != "/tmp/aim-root/data/pixmaps" {
		t.Fatalf("PixmapsDir = %q", paths.PixmapsDir)
	}
}

func TestApplyPathsAndCurrentPaths(t *testing.T) {
	original := CurrentPaths()
	t.Cleanup(func() {
		ApplyPaths(original)
	})

	updated := Paths{
		AimDir:       "/tmp/alt/data/aim",
		DesktopDir:   "/tmp/alt/data/applications",
		ConfigDir:    "/tmp/alt/config/aim",
		TempDir:      "/tmp/alt/cache/aim/tmp",
		DbSrc:        "/tmp/alt/state/aim/apps.json",
		IconThemeDir: "/tmp/alt/data/icons/hicolor",
		PixmapsDir:   "/tmp/alt/data/pixmaps",
	}

	ApplyPaths(updated)

	if got := CurrentPaths(); got != updated {
		t.Fatalf("CurrentPaths() = %#v", got)
	}
}

func TestResolvePathsFromRelativeStateRootNormalization(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}

	paths := ResolvePathsFromStateRoot(filepath.Clean(filepath.Join(wd, "..", filepath.Base(wd))))
	if !filepath.IsAbs(paths.AimDir) {
		t.Fatalf("AimDir should be absolute, got %q", paths.AimDir)
	}
}
