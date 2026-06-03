package integrate

import (
	"fmt"
	"path/filepath"
	"strings"
)

func (service Service) InstallDesktopIcon(iconID, iconSrc string) (string, string, error) {
	iconID = strings.TrimSpace(iconID)
	if iconID == "" {
		return "", "", fmt.Errorf("icon id cannot be empty")
	}

	iconSrc = strings.TrimSpace(iconSrc)
	if iconSrc == "" {
		return "", "", fmt.Errorf("icon source cannot be empty")
	}

	ext := strings.ToLower(filepath.Ext(iconSrc))
	if ext == "" {
		return "", "", fmt.Errorf("icon file extension is required")
	}

	destDir := service.iconInstallDir(ext)
	destName := iconID + ext

	destPath := filepath.Join(destDir, destName)
	desktopIconValue := destPath

	if filepath.Clean(iconSrc) == filepath.Clean(destPath) {
		return destPath, desktopIconValue, nil
	}

	filesystem, err := service.requireFilesystem()
	if err != nil {
		return "", "", err
	}
	if err := filesystem.EnsureDir(destDir); err != nil {
		return "", "", err
	}

	if _, err := filesystem.Move(iconSrc, destPath); err != nil {
		return "", "", err
	}

	return destPath, desktopIconValue, nil
}

func (service Service) iconInstallDir(ext string) string {
	paths, err := service.requirePaths()
	if err != nil {
		return ""
	}
	if ext == ".svg" {
		return filepath.Join(paths.IconThemeDir, "scalable", "apps")
	}

	return filepath.Join(paths.IconThemeDir, "256x256", "apps")
}
