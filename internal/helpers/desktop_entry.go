package util

import (
	"os"
	"strconv"
	"strings"
)

func RewriteDesktopEntryFile(path, execPath, iconValue string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	inDesktopEntryGroup := false
	inDesktopActionGroup := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntryGroup = trimmed == "[Desktop Entry]"
			inDesktopActionGroup = strings.HasPrefix(trimmed, "[Desktop Action ") && strings.HasSuffix(trimmed, "]")
			continue
		}

		if !inDesktopEntryGroup && !inDesktopActionGroup {
			continue
		}

		if strings.HasPrefix(trimmed, "Exec=") {
			lines[i] = rewriteDesktopExecLine(trimmed, execPath)
		}

		if inDesktopEntryGroup && iconValue != "" && strings.HasPrefix(trimmed, "Icon=") {
			lines[i] = "Icon=" + iconValue
		}
	}

	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}

	fileInfo, statErr := os.Stat(path)
	perm := os.FileMode(0o644)
	if statErr == nil {
		perm = fileInfo.Mode().Perm() & 0o666
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), perm)
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
