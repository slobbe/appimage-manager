package util

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func DesktopStemFromPath(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" {
		return ""
	}

	return strings.TrimSuffix(base, filepath.Ext(base))
}

func SanitizeDesktopStem(stem string) string {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(stem))

	prevSeparator := false
	for _, r := range stem {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			b.WriteRune(r)
			prevSeparator = false
		case r == '.', r == '-', r == '_':
			if prevSeparator {
				continue
			}
			b.WriteRune(r)
			prevSeparator = true
		case unicode.IsSpace(r):
			if prevSeparator {
				continue
			}
			b.WriteRune('-')
			prevSeparator = true
		}
	}

	return strings.Trim(b.String(), "._-")
}

func ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName string) (string, error) {
	preferredName = strings.TrimSpace(preferredName)
	fallbackName = strings.TrimSpace(fallbackName)

	candidates := make([]string, 0, 2)
	if preferredName != "" {
		candidates = append(candidates, filepath.Join(desktopDir, preferredName))
	}
	if fallbackName != "" && fallbackName != preferredName {
		candidates = append(candidates, filepath.Join(desktopDir, fallbackName))
	}

	for _, candidate := range candidates {
		owned, exists, err := DesktopLinkOwnedBy(candidate, src)
		if err != nil {
			return "", err
		}
		if owned || !exists {
			return candidate, nil
		}
	}

	return "", os.ErrExist
}

func DesktopLinkOwnedBy(linkPath, src string) (bool, bool, error) {
	linkPath = strings.TrimSpace(linkPath)
	src = strings.TrimSpace(src)
	if linkPath == "" || src == "" {
		return false, false, nil
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return false, true, nil
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		return false, true, err
	}

	if target == src {
		return true, true, nil
	}

	resolvedTarget, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, true, nil
		}
		return false, true, err
	}

	resolvedSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		if os.IsNotExist(err) {
			resolvedSrc = filepath.Clean(src)
		} else {
			return false, true, err
		}
	}

	return filepath.Clean(resolvedTarget) == filepath.Clean(resolvedSrc), true, nil
}
