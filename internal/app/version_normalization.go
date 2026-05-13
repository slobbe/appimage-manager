package app

import models "github.com/slobbe/appimage-manager/internal/domain"

// NormalizeComparableVersion extracts the comparable version token from decorated
// AppImage metadata and release tags.
func NormalizeComparableVersion(raw string) string {
	return models.NormalizeComparableVersion(raw)
}

func normalizeComparableVersion(raw string) string {
	return models.NormalizeComparableVersion(raw)
}
