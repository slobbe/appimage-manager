package config

import (
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
