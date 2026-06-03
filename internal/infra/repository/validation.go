package repo

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

func validateDB(db *db) error {
	if db == nil {
		return fmt.Errorf("database cannot be empty")
	}
	if db.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema version: %d", db.SchemaVersion)
	}
	if db.Apps == nil {
		return fmt.Errorf("apps collection cannot be empty")
	}

	for key, app := range db.Apps {
		if app == nil {
			return fmt.Errorf("app %q cannot be empty", strings.TrimSpace(key))
		}

		appID := strings.TrimSpace(app.ID)
		if appID == "" {
			appID = strings.TrimSpace(key)
		}

		if kind := strings.TrimSpace(string(app.Source.Kind)); kind != "" && !isSupportedSourceKind(app.Source.Kind) {
			return fmt.Errorf("unsupported source kind for %s: %q", appID, app.Source.Kind)
		}

		if app.Update != nil && !isSupportedUpdateKind(app.Update.Kind) {
			return fmt.Errorf("unsupported update kind for %s: %q", appID, app.Update.Kind)
		}
	}

	return nil
}

func isSupportedSourceKind(kind models.SourceKind) bool {
	switch kind {
	case models.SourceLocalFile, models.SourceDirectURL, models.SourceGitHubRelease:
		return true
	default:
		return false
	}
}

func isSupportedUpdateKind(kind models.UpdateKind) bool {
	switch kind {
	case models.UpdateNone, models.UpdateZsync, models.UpdateGitHubRelease:
		return true
	default:
		return false
	}
}
