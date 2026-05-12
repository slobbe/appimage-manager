package repo

import (
	"encoding/json"
	"os"
	"path/filepath"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type DB struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Apps          map[string]*models.App `json:"apps"`
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
	if err := validateDB(&db); err != nil {
		return nil, err
	}
	return &db, nil
}

func SaveDB(path string, db *DB) error {
	b, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false

	return syncDir(dir)
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	return dir.Sync()
}
