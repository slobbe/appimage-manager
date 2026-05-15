package domain

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

func ResolveManagedAppIdentity(appName, upstreamID, hashSeed string, incoming *App, existing map[string]*App) (string, *App, error) {
	candidates := ManagedIDCandidates(appName, upstreamID, hashSeed)
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("managed app id cannot be empty")
	}

	equivalentApp := EquivalentManagedApp(incoming, existing, "")
	for _, candidate := range candidates {
		current := existing[candidate]
		if current == nil {
			if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != candidate {
				return candidate, equivalentApp, nil
			}
			return candidate, nil, nil
		}
		if AppsShareManagedIdentity(current, incoming) {
			return candidate, nil, nil
		}
	}

	fallback := candidates[len(candidates)-1]
	if equivalentApp != nil && strings.TrimSpace(equivalentApp.ID) != fallback {
		return fallback, equivalentApp, nil
	}
	return fallback, nil, nil
}

func EquivalentManagedApp(incoming *App, existing map[string]*App, excludeID string) *App {
	if incoming == nil {
		return nil
	}

	excludeID = strings.TrimSpace(excludeID)
	var match *App
	for _, app := range existing {
		if app == nil {
			continue
		}
		id := strings.TrimSpace(app.ID)
		if id == "" || id == excludeID {
			continue
		}
		if !AppsShareManagedIdentity(app, incoming) {
			continue
		}
		if match != nil && strings.TrimSpace(match.ID) != id {
			return nil
		}
		match = app
	}
	return match
}

func ManagedIDCandidates(appName, upstreamID, hashSeed string) []string {
	upstreamID = strings.TrimSpace(upstreamID)
	base := Slugify(appName)
	if base == "" {
		base = upstreamID
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return nil
	}

	candidates := []string{base}
	if upstreamID != "" {
		withUpstream := base + "-" + upstreamID
		if !containsString(candidates, withUpstream) {
			candidates = append(candidates, withUpstream)
		}

		withHash := withUpstream + "-" + ShortIdentityHash(upstreamID, hashSeed)
		if !containsString(candidates, withHash) {
			candidates = append(candidates, withHash)
		}
	}

	return candidates
}

func ShortIdentityHash(parts ...string) string {
	seed := strings.Join(parts, "\x00")
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])[:6]
}

func AppsShareManagedIdentity(a, b *App) bool {
	if a == nil || b == nil {
		return false
	}

	if UpdateSourcesEqual(a.Update, b.Update) {
		return true
	}

	return SourcesEqual(&a.Source, &b.Source)
}

func UpdateSourcesEqual(a, b *UpdateSource) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case UpdateNone:
		return false
	case UpdateGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	case UpdateZsync:
		return a.Zsync != nil && b.Zsync != nil &&
			strings.TrimSpace(a.Zsync.UpdateInfo) == strings.TrimSpace(b.Zsync.UpdateInfo) &&
			strings.TrimSpace(a.Zsync.Transport) == strings.TrimSpace(b.Zsync.Transport)
	default:
		return false
	}
}

func SourcesEqual(a, b *Source) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}

	switch a.Kind {
	case SourceLocalFile:
		return a.LocalFile != nil && b.LocalFile != nil &&
			normalizeLocalSourcePath(a.LocalFile.OriginalPath) != "" &&
			normalizeLocalSourcePath(a.LocalFile.OriginalPath) == normalizeLocalSourcePath(b.LocalFile.OriginalPath)
	case SourceDirectURL:
		return a.DirectURL != nil && b.DirectURL != nil &&
			strings.TrimSpace(a.DirectURL.URL) != "" &&
			strings.TrimSpace(a.DirectURL.URL) == strings.TrimSpace(b.DirectURL.URL)
	case SourceGitHubRelease:
		return a.GitHubRelease != nil && b.GitHubRelease != nil &&
			strings.TrimSpace(a.GitHubRelease.Repo) == strings.TrimSpace(b.GitHubRelease.Repo) &&
			strings.TrimSpace(a.GitHubRelease.Asset) == strings.TrimSpace(b.GitHubRelease.Asset)
	default:
		return false
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func normalizeLocalSourcePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return cleanLocalSourcePath(trimmed)
}

func cleanLocalSourcePath(path string) string {
	normalized := strings.ReplaceAll(path, "\\", "/")
	absolute := strings.HasPrefix(normalized, "/")
	parts := strings.Split(normalized, "/")
	stack := make([]string, 0, len(parts))

	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(stack) > 0 && stack[len(stack)-1] != ".." {
				stack = stack[:len(stack)-1]
				continue
			}
			if !absolute {
				stack = append(stack, part)
			}
		default:
			stack = append(stack, part)
		}
	}

	cleaned := strings.Join(stack, "/")
	if absolute {
		if cleaned == "" {
			return "/"
		}
		return "/" + cleaned
	}
	if cleaned == "" {
		return "."
	}
	return cleaned
}
