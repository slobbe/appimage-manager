package config

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	AimDir     string
	DesktopDir string

	TempDir string
	DbSrc   string
)

type resolvedPaths struct {
	AimDir     string
	DesktopDir string
	TempDir    string
	DbSrc      string
}

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("failed to get home directory: " + err.Error())
	}

	paths := resolvePaths(home, os.Getenv)
	AimDir = paths.AimDir
	DesktopDir = paths.DesktopDir
	TempDir = paths.TempDir
	DbSrc = paths.DbSrc
}

func EnsureDirsExist() error {
	dirs := []string{AimDir, DesktopDir, TempDir, filepath.Dir(DbSrc)}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func resolvePaths(home string, getenv func(string) string) resolvedPaths {
	dataHome := resolveXDGBaseDir(getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	stateHome := resolveXDGBaseDir(getenv("XDG_STATE_HOME"), filepath.Join(home, ".local", "state"))
	cacheHome := resolveXDGBaseDir(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))

	return resolvedPaths{
		AimDir:     filepath.Join(dataHome, "appimage-manager"),
		DesktopDir: filepath.Join(dataHome, "applications"),
		TempDir:    filepath.Join(cacheHome, "appimage-manager", "tmp"),
		DbSrc:      filepath.Join(stateHome, "appimage-manager", "apps.json"),
	}
}

func resolveXDGBaseDir(envValue, fallback string) string {
	value := strings.TrimSpace(envValue)
	if value != "" && filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return fallback
}
