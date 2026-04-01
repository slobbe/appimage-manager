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

type Paths struct {
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

	ApplyPaths(resolvePaths(home, os.Getenv))
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

func CurrentPaths() Paths {
	return Paths{
		AimDir:       AimDir,
		DesktopDir:   DesktopDir,
		ConfigDir:    ConfigDir,
		TempDir:      TempDir,
		DbSrc:        DbSrc,
		IconThemeDir: IconThemeDir,
		PixmapsDir:   PixmapsDir,
	}
}

func ApplyPaths(paths Paths) {
	AimDir = paths.AimDir
	DesktopDir = paths.DesktopDir
	ConfigDir = paths.ConfigDir
	TempDir = paths.TempDir
	DbSrc = paths.DbSrc
	IconThemeDir = paths.IconThemeDir
	PixmapsDir = paths.PixmapsDir
}

func ResolvePathsFromStateRoot(root string) Paths {
	base := filepath.Clean(strings.TrimSpace(root))
	return Paths{
		AimDir:       filepath.Join(base, "data", "aim"),
		DesktopDir:   filepath.Join(base, "data", "applications"),
		ConfigDir:    filepath.Join(base, "config", "aim"),
		TempDir:      filepath.Join(base, "cache", "aim", "tmp"),
		DbSrc:        filepath.Join(base, "state", "aim", "apps.json"),
		IconThemeDir: filepath.Join(base, "data", "icons", "hicolor"),
		PixmapsDir:   filepath.Join(base, "data", "pixmaps"),
	}
}

func resolvePaths(home string, getenv func(string) string) Paths {
	dataHome := resolveXDGBaseDir(getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	configHome := resolveXDGBaseDir(getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
	stateHome := resolveXDGBaseDir(getenv("XDG_STATE_HOME"), filepath.Join(home, ".local", "state"))
	cacheHome := resolveXDGBaseDir(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))

	return Paths{
		AimDir:       filepath.Join(dataHome, "aim"),
		DesktopDir:   filepath.Join(dataHome, "applications"),
		ConfigDir:    filepath.Join(configHome, "aim"),
		TempDir:      filepath.Join(cacheHome, "aim", "tmp"),
		DbSrc:        filepath.Join(stateHome, "aim", "apps.json"),
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
