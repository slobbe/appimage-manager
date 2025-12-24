package core

import (
	"encoding/json"
	"os"
	"time"
)

type DB struct {
	Version int             `json:"version"`
	Apps    map[string]*App `json:"apps"`
}

type App struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	AppImage    string `json:"appimage"`
	Desktop     string `json:"desktop"`
	DesktopLink string `json:"desktop_link"`
	Icon        string `json:"icon"`
	AddedAt     string `json:"added_at"`
	SHA256      string `json:"sha256"`
}

func LoadDB(path string) (*DB, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &DB{Version: 1, Apps: map[string]*App{}}, nil
		}
		return nil, err
	}
	var db DB
	if err := json.Unmarshal(b, &db); err != nil {
		return nil, err
	}
	if db.Apps == nil {
		db.Apps = map[string]*App{}
	}
	if db.Version == 0 {
		db.Version = 1
	}
	return &db, nil
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

func NowISO() string { return time.Now().UTC().Format(time.RFC3339) }
