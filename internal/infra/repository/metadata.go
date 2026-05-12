package repo

import (
	"fmt"
	"strings"
)

type CheckMetadataUpdate struct {
	ID            string
	Checked       bool
	Available     bool
	Latest        string
	LastCheckedAt string
}

func (s *Store) UpdateCheckMetadataBatch(updates []CheckMetadataUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	db, err := s.load()
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

	return s.save(db)
}
