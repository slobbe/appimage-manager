package discovery

import (
	"strings"
	"time"
)

const (
	defaultAssetPattern = "*.AppImage"
	coreHTTPTimeout     = 30 * time.Second
)

func normalizeAssetPattern(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultAssetPattern
	}
	return trimmed
}

func versionForDisplay(normalized, fallback string) string {
	if strings.TrimSpace(normalized) != "" {
		return strings.TrimSpace(normalized)
	}
	return strings.TrimSpace(fallback)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
