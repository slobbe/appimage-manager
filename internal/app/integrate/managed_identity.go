package integrate

import models "github.com/slobbe/appimage-manager/internal/domain"

func ResolveManagedAppID(appName, upstreamID, hashSeed string, incoming *models.App) (string, *models.App, error) {
	store, err := requireStore()
	if err != nil {
		return "", nil, err
	}

	return resolveManagedAppID(store, appName, upstreamID, hashSeed, incoming)
}

func resolveManagedAppID(store AppStore, appName, upstreamID, hashSeed string, incoming *models.App) (string, *models.App, error) {
	allApps, err := store.GetAllApps()
	if err != nil {
		return "", nil, err
	}
	return models.ResolveManagedAppIdentity(appName, upstreamID, hashSeed, incoming, allApps)
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

	return models.EquivalentManagedApp(incoming, allApps, incoming.ID), nil
}
