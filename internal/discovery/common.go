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

func scoreSearchCandidate(query, name, summary string, popularity int) int {
	query = strings.ToLower(strings.TrimSpace(query))
	name = strings.ToLower(strings.TrimSpace(name))
	summary = strings.ToLower(strings.TrimSpace(summary))

	score := 0
	if query != "" && name == query {
		score += 1000
	}
	if query != "" && strings.Contains(name, query) {
		score += 300
	}

	for _, token := range strings.Fields(query) {
		if strings.Contains(name, token) {
			score += 120
		}
		if strings.Contains(summary, token) {
			score += 40
		}
	}

	if popularity > 0 {
		score += popularity
	}

	return score
}
