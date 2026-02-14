package config

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	AimDir       string
	DesktopDir   string
	ConfigDir    string
	TempDir      string
	DbSrc        string
	IconThemeDir string
	PixmapsDir   string
)

type resolvedPaths struct {
	AimDir       string
	DesktopDir   string
	ConfigDir    string
	TempDir      string
	DbSrc        string
	IconThemeDir string
	PixmapsDir   string
}

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("failed to get home directory: " + err.Error())
	}

	paths := resolvePaths(home, os.Getenv)
	AimDir = paths.AimDir
	DesktopDir = paths.DesktopDir
	ConfigDir = paths.ConfigDir
	TempDir = paths.TempDir
	DbSrc = paths.DbSrc
	IconThemeDir = paths.IconThemeDir
	PixmapsDir = paths.PixmapsDir
}

func EnsureDirsExist() error {
	dirs := []string{AimDir, DesktopDir, ConfigDir, TempDir, filepath.Dir(DbSrc), IconThemeDir, PixmapsDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func resolvePaths(home string, getenv func(string) string) resolvedPaths {
	dataHome := resolveXDGBaseDir(getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	configHome := resolveXDGBaseDir(getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
	stateHome := resolveXDGBaseDir(getenv("XDG_STATE_HOME"), filepath.Join(home, ".local", "state"))
	cacheHome := resolveXDGBaseDir(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))

	return resolvedPaths{
		AimDir:       filepath.Join(dataHome, "appimage-manager"),
		DesktopDir:   filepath.Join(dataHome, "applications"),
		ConfigDir:    filepath.Join(configHome, "appimage-manager"),
		TempDir:      filepath.Join(cacheHome, "appimage-manager", "tmp"),
		DbSrc:        filepath.Join(stateHome, "appimage-manager", "apps.json"),
		IconThemeDir: filepath.Join(dataHome, "icons", "hicolor"),
		PixmapsDir:   filepath.Join(dataHome, "pixmaps"),
	}
}

func resolveXDGBaseDir(envValue, fallback string) string {
	value := strings.TrimSpace(envValue)
	if value != "" && filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return fallback
}
