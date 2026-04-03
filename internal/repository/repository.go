package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type DB struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Apps          map[string]*models.App `json:"apps"`
}

type CheckMetadataUpdate struct {
	ID            string
	Checked       bool
	Available     bool
	Latest        string
	LastCheckedAt string
}

func LoadDB(path string) (*DB, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &DB{SchemaVersion: 1, Apps: map[string]*models.App{}}, nil
		}
		return nil, err
	}
	var db DB
	if err := json.Unmarshal(b, &db); err != nil {
		return nil, err
	}
	if db.Apps == nil {
		db.Apps = map[string]*models.App{}
	}
	if db.SchemaVersion == 0 {
		db.SchemaVersion = 1
	}
	if err := validateDB(&db); err != nil {
		return nil, err
	}
	return &db, nil
}

func validateDB(db *DB) error {
	if db == nil {
		return fmt.Errorf("database cannot be empty")
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
	case models.SourceLocalFile, models.SourceDirectURL, models.SourceGitHubRelease, models.SourceGitLabRelease:
		return true
	default:
		return false
	}
}

func isSupportedUpdateKind(kind models.UpdateKind) bool {
	switch kind {
	case models.UpdateNone, models.UpdateZsync, models.UpdateGitHubRelease, models.UpdateGitLabRelease:
		return true
	default:
		return false
	}
}

func SaveDB(path string, db *DB) error {
	tmp := path + ".tmp"
	b, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func AddApp(appData *models.App, overwrite bool) error {
	if appData == nil {
		return fmt.Errorf("app data cannot be empty")
	}

	if len(config.DbSrc) < 1 {
		return fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	key := appData.ID
	if len(key) < 1 {
		return fmt.Errorf("invalid app slug")
	}

	_, exists := db.Apps[key]
	if exists && !overwrite {
		return fmt.Errorf("%s already exists in database", key)
	} else {
		db.Apps[key] = appData
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	return nil
}

func AddAppsBatch(apps []*models.App, overwrite bool) error {
	if len(apps) == 0 {
		return nil
	}

	if len(config.DbSrc) < 1 {
		return fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	for _, appData := range apps {
		if appData == nil {
			return fmt.Errorf("app data cannot be empty")
		}

		key := strings.TrimSpace(appData.ID)
		if key == "" {
			return fmt.Errorf("invalid app slug")
		}

		_, exists := db.Apps[key]
		if exists && !overwrite {
			return fmt.Errorf("%s already exists in database", key)
		}

		db.Apps[key] = appData
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	return nil
}

func UpdateApp(appData *models.App) error {
	if appData == nil {
		return fmt.Errorf("app data cannot be empty")
	}

	if len(config.DbSrc) < 1 {
		return fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	key := appData.ID
	if len(key) < 1 {
		return fmt.Errorf("invalid app slug")
	}

	_, exists := db.Apps[key]
	if !exists {
		return fmt.Errorf("%s does not exists in database", key)
	}

	db.Apps[key] = appData

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	return nil
}

func RemoveApp(key string) error {
	if len(key) < 1 {
		return fmt.Errorf("invalid app slug")
	}

	if len(config.DbSrc) < 1 {
		return fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	_, exists := db.Apps[key]
	if !exists {
		return fmt.Errorf("%s does not exists in database", key)
	}

	delete(db.Apps, key)

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	return nil
}

func GetApp(key string) (*models.App, error) {
	if len(key) < 1 {
		return nil, fmt.Errorf("invalid app slug")
	}

	if len(config.DbSrc) < 1 {
		return nil, fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return nil, err
	}

	_, exists := db.Apps[key]
	if !exists {
		return nil, fmt.Errorf("%s does not exists in database", key)
	}

	appData := db.Apps[key]

	return appData, nil
}

func GetAllApps() (map[string]*models.App, error) {
	if len(config.DbSrc) < 1 {
		return nil, fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return nil, err
	}

	return db.Apps, nil
}

func UpdateCheckMetadataBatch(updates []CheckMetadataUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	if len(config.DbSrc) < 1 {
		return fmt.Errorf("database source cannot be empty")
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return err
	}

	for _, update := range updates {
		key := strings.TrimSpace(update.ID)
		if key == "" {
			return fmt.Errorf("invalid app slug")
		}

		appData, exists := db.Apps[key]
		if !exists {
			return fmt.Errorf("%s does not exists in database", key)
		}

		if update.Checked {
			appData.UpdateAvailable = update.Available
			appData.LatestVersion = strings.TrimSpace(update.Latest)
		}

		appData.LastCheckedAt = strings.TrimSpace(update.LastCheckedAt)
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return err
	}

	return nil
}
