package integrate

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

func ResolveManagedAppID(appName, upstreamID, hashSeed string, incoming *models.App) (string, *models.App, error) {
	store, err := requireStore()
	if err != nil {
		return "", nil, err
	}

	return resolveManagedAppID(store, appName, upstreamID, hashSeed, incoming)
}

func resolveManagedAppID(store AppStore, appName, upstreamID, hashSeed string, incoming *models.App) (string, *models.App, error) {
	candidates := models.ManagedIDCandidates(appName, upstreamID, hashSeed)
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("managed app id cannot be empty")
	}

	allApps, err := store.GetAllApps()
	if err != nil {
		return "", nil, err
	}

	var equivalentApp *models.App
	for _, existing := range allApps {
		if existing == nil || strings.TrimSpace(existing.ID) == "" {
			continue
		}
		if models.AppsShareManagedIdentity(existing, incoming) {
			if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != strings.TrimSpace(existing.ID) {
				equivalentApp = nil
				break
			}
			equivalentApp = existing
		}
	}

	for _, candidate := range candidates {
		existing := allApps[candidate]
		if existing == nil {
			if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != candidate {
				return candidate, equivalentApp, nil
			}
			return candidate, nil, nil
		}
		if models.AppsShareManagedIdentity(existing, incoming) {
			return candidate, nil, nil
		}
	}

	fallback := candidates[len(candidates)-1]
	if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != fallback {
		return fallback, equivalentApp, nil
	}
	return fallback, nil, nil
}

func FindEquivalentManagedApp(incoming *models.App) (*models.App, error) {
	if incoming == nil {
		return nil, nil
	}

	store, err := requireStore()
	if err != nil {
		return nil, err
	}

	allApps, err := store.GetAllApps()
	if err != nil {
		return nil, err
	}

	var match *models.App
	for _, existing := range allApps {
		if existing == nil {
			continue
		}
		if strings.TrimSpace(existing.ID) == "" || strings.TrimSpace(existing.ID) == strings.TrimSpace(incoming.ID) {
			continue
		}
		if !models.AppsShareManagedIdentity(existing, incoming) {
			continue
		}
		if match != nil {
			return nil, nil
		}
		match = existing
	}

	return match, nil
}
