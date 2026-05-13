package filesystem

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type DesktopEntryCandidate struct {
	Path string
	Stem string
}

func LocateDesktopEntry(root string) (*DesktopEntryCandidate, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("directory cannot be empty")
	}
	if err := RequireDir(root); err != nil {
		return nil, err
	}

	desktopFiles, err := filepath.Glob(filepath.Join(root, "*.desktop"))
	if err != nil {
		return nil, fmt.Errorf("glob pattern error: %w", err)
	}

	if len(desktopFiles) == 0 {
		desktopFiles, err = findDesktopFilesRecursive(root)
		if err != nil {
			return nil, err
		}
		if len(desktopFiles) == 0 {
			return nil, fmt.Errorf("no .desktop file found in: %s", root)
		}
	}

	path := selectPreferredDesktopFile(root, desktopFiles)
	resolved, err := ResolveRegularFile(path, "desktop file")
	if err != nil {
		return nil, err
	}

	return &DesktopEntryCandidate{
		Path: resolved,
		Stem: sanitizeDesktopStem(desktopStemFromPath(path)),
	}, nil
}

func LocateIcon(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("directory cannot be empty")
	}
	if err := RequireDir(root); err != nil {
		return "", err
	}

	extensions := []string{".svg", ".png", ".ico", ".xpm"}
	var candidates []string
	for _, ext := range extensions {
		pattern := filepath.Join(root, "*"+ext)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("glob pattern error for %s: %w", ext, err)
		}
		candidates = append(candidates, matches...)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no icon file found in: %s", root)
	}

	var lastErr error
	for _, candidate := range candidates {
		resolved, err := ResolveRegularFile(candidate, "icon file")
		if err != nil {
			lastErr = err
			continue
		}
		return resolved, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("no valid icon found: %w", lastErr)
	}

	return "", fmt.Errorf("no icon found in: %s", root)
}

func findDesktopFilesRecursive(root string) ([]string, error) {
	var desktopFiles []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		if strings.EqualFold(filepath.Ext(d.Name()), ".desktop") {
			desktopFiles = append(desktopFiles, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search .desktop files recursively: %w", err)
	}

	return desktopFiles, nil
}

func selectPreferredDesktopFile(root string, candidates []string) string {
	if len(candidates) == 1 {
		return candidates[0]
	}

	sort.Strings(candidates)

	dirName := filepath.Base(root)
	for _, candidate := range candidates {
		candidateName := strings.TrimSuffix(filepath.Base(candidate), ".desktop")
		if candidateName == dirName {
			return candidate
		}
	}

	for _, candidate := range candidates {
		if strings.HasPrefix(filepath.Base(candidate), dirName) {
			return candidate
		}
	}

	for _, candidate := range candidates {
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			continue
		}

		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "usr/share/applications/") {
			return candidate
		}
	}

	return candidates[0]
}

func desktopStemFromPath(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return ""
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func sanitizeDesktopStem(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".desktop")
	value = strings.Trim(value, ".-_ ")
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '.', r == '_', r == '-', r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}
