package repo

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: strings.TrimSpace(path)}
}

func (s *Store) requirePath() (string, error) {
	if s == nil || s.path == "" {
		return "", fmt.Errorf("database source cannot be empty")
	}
	return s.path, nil
}

func (s *Store) load() (*db, error) {
	path, err := s.requirePath()
	if err != nil {
		return nil, err
	}
	return loadDB(path)
}

func (s *Store) save(data *db) error {
	path, err := s.requirePath()
	if err != nil {
		return err
	}
	return saveDB(path, data)
}

func (s *Store) AddApp(appData *models.App, overwrite bool) error {
	if appData == nil {
		return fmt.Errorf("app data cannot be empty")
	}

	db, err := s.load()
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
	}

	db.Apps[key] = appData

	return s.save(db)
}

func (s *Store) AddAppsBatch(apps []*models.App, overwrite bool) error {
	if len(apps) == 0 {
		return nil
	}

	db, err := s.load()
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

	return s.save(db)
}

func (s *Store) UpdateApp(appData *models.App) error {
	if appData == nil {
		return fmt.Errorf("app data cannot be empty")
	}

	db, err := s.load()
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

	return s.save(db)
}

func (s *Store) RemoveApp(key string) error {
	if len(key) < 1 {
		return fmt.Errorf("invalid app slug")
	}

	db, err := s.load()
	if err != nil {
		return err
	}

	_, exists := db.Apps[key]
	if !exists {
		return fmt.Errorf("%s does not exists in database", key)
	}

	delete(db.Apps, key)

	return s.save(db)
}

func (s *Store) GetApp(key string) (*models.App, error) {
	if len(key) < 1 {
		return nil, fmt.Errorf("invalid app slug")
	}

	db, err := s.load()
	if err != nil {
		return nil, err
	}

	appData, exists := db.Apps[key]
	if !exists {
		return nil, fmt.Errorf("%s does not exists in database", key)
	}

	return appData, nil
}

func (s *Store) GetAllApps() (map[string]*models.App, error) {
	db, err := s.load()
	if err != nil {
		return nil, err
	}

	apps := make(map[string]*models.App, len(db.Apps))
	for key, app := range db.Apps {
		apps[key] = app
	}

	return apps, nil
}
