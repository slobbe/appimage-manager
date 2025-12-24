package config

import (
	"os"
	"path/filepath"
)

var (
	AppName    = "AppImageManager"
	AppVersion = "0.1.0"

	AimDir     string
	DesktopDir string
	TempDir    string

	DbSrc         string
	UnlinkedDbSrc string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("failed to get home directory: " + err.Error())
	}

	AimDir = filepath.Join(home, ".local/share/appimage-manager")
	DesktopDir = filepath.Join(home, ".local/share/applications")

	TempDir = filepath.Join(AimDir, ".tmp")

	DbSrc = filepath.Join(AimDir, "apps.json")
	UnlinkedDbSrc = filepath.Join(AimDir, "unlinked.json")
}
