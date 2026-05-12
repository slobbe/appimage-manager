package util

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func RewriteDesktopEntryFile(path, execPath, iconValue string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	rewritten := make([]string, 0, len(lines))
	inDesktopEntryGroup := false
	inDesktopActionGroup := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntryGroup = trimmed == "[Desktop Entry]"
			inDesktopActionGroup = strings.HasPrefix(trimmed, "[Desktop Action ") && strings.HasSuffix(trimmed, "]")
			rewritten = append(rewritten, line)
			continue
		}

		if !inDesktopEntryGroup && !inDesktopActionGroup {
			rewritten = append(rewritten, line)
			continue
		}

		if inDesktopEntryGroup {
			key, value, ok := splitDesktopEntryKeyValue(trimmed)
			if ok && shouldDropDesktopKey(key, value) {
				continue
			}
		}

		switch {
		case strings.HasPrefix(trimmed, "Exec="):
			line = rewriteDesktopExecLine(trimmed, execPath)
		case inDesktopEntryGroup && iconValue != "" && strings.HasPrefix(trimmed, "Icon="):
			line = "Icon=" + iconValue
		}

		rewritten = append(rewritten, line)
	}

	if len(rewritten) > 0 && rewritten[len(rewritten)-1] != "" {
		rewritten = append(rewritten, "")
	}

	fileInfo, statErr := os.Stat(path)
	perm := os.FileMode(0o644)
	if statErr == nil {
		perm = fileInfo.Mode().Perm() & 0o666
	}

	return os.WriteFile(path, []byte(strings.Join(rewritten, "\n")), perm)
}

func rewriteDesktopExecLine(execLine, execPath string) string {
	value := strings.TrimPrefix(execLine, "Exec=")
	value = strings.TrimSpace(value)
	_, args := splitDesktopExec(value)
	return "Exec=" + quoteDesktopExecArg(execPath) + args
}

func splitDesktopExec(value string) (string, string) {
	if value == "" {
		return "", ""
	}

	if value[0] == '"' {
		escaped := false
		for i := 1; i < len(value); i++ {
			if escaped {
				escaped = false
				continue
			}
			if value[i] == '\\' {
				escaped = true
				continue
			}
			if value[i] == '"' {
				return value[:i+1], value[i+1:]
			}
		}
		return value, ""
	}

	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		return value[:idx], value[idx:]
	}

	return value, ""
}

func quoteDesktopExecArg(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t\n\r\"") {
		return strconv.Quote(value)
	}
	return value
}

func splitDesktopEntryKeyValue(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}

	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

func shouldDropDesktopKey(key, value string) bool {
	switch key {
	case "DBusActivatable":
		return true
	case "TryExec":
		return shouldDropTryExec(value)
	default:
		return false
	}
}

func shouldDropTryExec(value string) bool {
	if !filepath.IsAbs(value) {
		return false
	}

	_, err := os.Stat(value)
	return errors.Is(err, os.ErrNotExist)
}
