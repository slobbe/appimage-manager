package domain

import (
	"path/filepath"
	"strings"
)

func ShouldRemoveStaleInstalledIcon(oldPath, newPath, appID, appDir string, apps map[string]*App) bool {
	oldPath = filepath.Clean(strings.TrimSpace(oldPath))
	newPath = filepath.Clean(strings.TrimSpace(newPath))
	appID = strings.TrimSpace(appID)
	appDir = filepath.Clean(strings.TrimSpace(appDir))

	if oldPath == "." || oldPath == "" || oldPath == newPath {
		return false
	}
	if appDir != "." && appDir != "" && (oldPath == appDir || strings.HasPrefix(oldPath, appDir+string(filepath.Separator))) {
		return false
	}

	for _, app := range apps {
		if app == nil || strings.TrimSpace(app.ID) == appID {
			continue
		}
		if filepath.Clean(strings.TrimSpace(app.IconPath)) == oldPath {
			return false
		}
	}

	return true
}
