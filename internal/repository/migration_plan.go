package repo

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type MigrationPlan struct {
	TargetID            string   `json:"target_id,omitempty"`
	CurrentDBExists     bool     `json:"current_db_exists"`
	LegacySources       []string `json:"legacy_sources,omitempty"`
	ConfigRoots         []string `json:"config_roots,omitempty"`
	PlannedAppIDs       []string `json:"planned_app_ids,omitempty"`
	CanonicalDBSource   string   `json:"canonical_db_source,omitempty"`
	WouldChangeAnything bool     `json:"would_change_anything"`
}

func PlanMigrationToCurrentPaths(id string) (*MigrationPlan, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	states, err := discoverMigrationSources(home)
	if err != nil {
		return nil, err
	}

	currentDBExists, err := fileExists(config.DbSrc)
	if err != nil {
		return nil, err
	}

	orderedStates := orderedLegacyStates(states)
	plan := &MigrationPlan{
		TargetID:        strings.TrimSpace(id),
		CurrentDBExists: currentDBExists,
	}

	for _, state := range orderedStates {
		plan.LegacySources = append(plan.LegacySources, state.Source.Name)
		if state.ConfigExists && strings.TrimSpace(state.Source.ConfigDir) != "" {
			plan.ConfigRoots = append(plan.ConfigRoots, state.Source.ConfigDir)
		}
	}

	if canonical := chooseCanonicalLegacyDB(orderedStates); canonical != nil {
		plan.CanonicalDBSource = canonical.Source.Name
	}

	if currentDBExists {
		db, err := LoadDB(config.DbSrc)
		if err != nil {
			return nil, err
		}
		if plan.TargetID == "" {
			plan.PlannedAppIDs = sortedAppIDs(db.Apps)
		} else if _, ok := db.Apps[plan.TargetID]; ok {
			plan.PlannedAppIDs = []string{plan.TargetID}
		}
	}

	if !currentDBExists && plan.CanonicalDBSource != "" {
		for _, state := range orderedStates {
			if state.Source.Name != plan.CanonicalDBSource {
				continue
			}
			db, err := LoadDB(state.Source.DBPath)
			if err != nil {
				return nil, err
			}
			if plan.TargetID == "" {
				plan.PlannedAppIDs = sortedAppIDs(db.Apps)
			} else if _, ok := db.Apps[plan.TargetID]; ok {
				plan.PlannedAppIDs = []string{plan.TargetID}
			}
			break
		}
	}

	if plan.TargetID != "" && len(plan.PlannedAppIDs) == 0 && currentDBExists {
		return nil, fmt.Errorf("%s does not exists in database", plan.TargetID)
	}

	plan.WouldChangeAnything = len(plan.LegacySources) > 0 || len(plan.ConfigRoots) > 0
	if currentDBExists && len(plan.PlannedAppIDs) > 0 {
		plan.WouldChangeAnything = true
	}
	if !currentDBExists && plan.CanonicalDBSource != "" && len(plan.PlannedAppIDs) > 0 {
		plan.WouldChangeAnything = true
	}

	return plan, nil
}

func sortedAppIDs(apps map[string]*models.App) []string {
	ids := make([]string, 0, len(apps))
	for id := range apps {
		ids = append(ids, strings.TrimSpace(id))
	}
	sort.Strings(ids)
	return ids
}
