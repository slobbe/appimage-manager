package domain

import "strings"

type AppInfo struct {
	Name        string
	ID          string
	DesktopStem string
	Version     string
}

func ParseDesktopEntryAppInfo(desktopPath, content, desktopStem string) *AppInfo {
	appInfo := AppInfo{
		DesktopStem: strings.TrimSpace(desktopStem),
	}

	inDesktopEntry := false
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntry = trimmed == "[Desktop Entry]"
			continue
		}

		if inDesktopEntry && strings.HasPrefix(trimmed, "[") {
			break
		}

		if !inDesktopEntry {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if strings.Contains(key, "[") {
			continue
		}

		switch key {
		case "Name":
			if appInfo.Name == "" {
				appInfo.Name = value
			}
		case "X-AppImage-Version":
			if appInfo.Version == "" {
				appInfo.Version = NormalizeComparableVersion(value)
			}
		}
	}

	if appInfo.Name == "" {
		appInfo.Name = trimFileExtension(fileBase(desktopPath))
	}
	if appInfo.Version == "" {
		appInfo.Version = AppVersionFromFilename(desktopPath)
	}
	if appInfo.Version == "" {
		appInfo.Version = "unknown"
	}

	appInfo.ID = appInfo.DesktopStem
	if appInfo.ID == "" {
		appInfo.ID = Slugify(appInfo.Name)
	}

	return &appInfo
}

func AppVersionFromFilename(path string) string {
	base := trimFileExtension(fileBase(path))
	if base == "" {
		return ""
	}

	return NormalizeComparableVersion(base)
}

func fileBase(path string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if normalized == "" {
		return ""
	}

	index := strings.LastIndex(normalized, "/")
	if index >= 0 {
		return normalized[index+1:]
	}
	return normalized
}

func trimFileExtension(name string) string {
	index := strings.LastIndex(name, ".")
	if index <= 0 {
		return name
	}
	return name[:index]
}
