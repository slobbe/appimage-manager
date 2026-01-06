package config

import (
	"os"
	"path/filepath"
)

var (
	AimDir     string
	DesktopDir string
	TempDir    string

	DbSrc string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("failed to get home directory: " + err.Error())
	}

	AimDir = filepath.Join(home, ".appimage-manager")
	DesktopDir = filepath.Join(home, ".local/share/applications")

	DbSrc = filepath.Join(AimDir, "apps.json")
}

func EnsureDirsExist() error {
	dirs := []string{AimDir, DesktopDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
