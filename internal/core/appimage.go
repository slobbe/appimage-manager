package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
)

type AppInfo struct {
	Name    string
	ID      string
	Version string
}

type ExtractionData struct {
	Dir              string
	ExecPath         string
	DesktopEntryPath string
	IconPath         string
}

func ExtractAppImage(ctx context.Context, src string) (*ExtractionData, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("failed to access source file: %w", err)
	}
	if srcInfo.IsDir() {
		return nil, fmt.Errorf("source path is a directory, not a file: %s", src)
	}

	srcFileExt := filepath.Ext(src)
	srcFileName := strings.TrimSuffix(filepath.Base(src), srcFileExt)

	if !util.HasExtension(src, ".AppImage") {
		return nil, fmt.Errorf("source file must be an .AppImage: %s", srcFileExt)
	}

	if src, err = util.MakeAbsolute(src); err != nil {
		return nil, fmt.Errorf("failed to make source path absolute: %w", err)
	}

	if err := util.MakeExecutable(src); err != nil {
		return nil, fmt.Errorf("failed to make executable: %w", err)
	}

	tmpDir := config.TempDir + "-" + srcFileName
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create temporary directory %s: %w", tmpDir, err)
	}

	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	extractCmd := exec.Command(src, "--appimage-extract")
	extractCmd.Dir = tmpDir

	out, err := extractCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w\nOutput: %s", err, string(out))
	}

	root := filepath.Join(tmpDir, "squashfs-root")
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("extraction verification failed: squashfs-root not found")
	}

	tempDesktopSrc, err := LocateDesktopFile(root)
	tempIconSrc, err := LocateIcon(root)

	extractDir := filepath.Join(config.AimDir, "."+srcFileName)
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create temporary extracttion directory %s: %w", extractDir, err)
	}

	execSrc := filepath.Join(extractDir, srcFileName+".AppImage")
	if _, err = util.Copy(src, execSrc); err != nil {
		return nil, err
	}

	desktopSrc := filepath.Join(extractDir, srcFileName+".desktop")
	if _, err := util.Move(tempDesktopSrc, desktopSrc); err != nil {
		return nil, err
	}

	iconSrc := filepath.Join(extractDir, srcFileName+filepath.Ext(tempIconSrc))
	if _, err := util.Move(tempIconSrc, iconSrc); err != nil {
		return nil, err
	}

	extractionData := &ExtractionData{
		Dir:              extractDir,
		ExecPath:         execSrc,
		DesktopEntryPath: desktopSrc,
		IconPath:         iconSrc,
	}

	return extractionData, nil
}

func UpdateDesktopEntry(ctx context.Context, src string, execSrc string, iconSrc string) error {
	if !util.HasExtension(src, ".desktop") {
		return fmt.Errorf("source file must be a .desktop file")
	}

	if execSrc == "" || !util.HasExtension(execSrc, ".AppImage") {
		return fmt.Errorf("exec source file must be a .AppImage file")
	}

	if iconSrc == "" {
		return fmt.Errorf("icon source file cannot be empty")
	}

	content, err := util.ReadFileContents(src)
	if err != nil {
		return err
	}

	inDesktopEntryGroup := false
	inDesktopActionGroup := false
	lines := strings.Split(content, "\n")
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

		// handle Exec= lines - preserve arguments after command
		if strings.HasPrefix(trimmed, "Exec=") {
			lines[i] = rewriteExecLine(trimmed, execSrc)
		}

		// handle Icon= lines
		if inDesktopEntryGroup && strings.HasPrefix(trimmed, "Icon=") {
			lines[i] = "Icon=" + iconSrc
		}
	}

	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}

	info, statErr := os.Stat(src)
	var perm os.FileMode = 0o644
	if statErr == nil {
		perm = info.Mode().Perm() & 0o666
	}

	if err := os.WriteFile(src, []byte(strings.Join(lines, "\n")), perm); err != nil {
		return fmt.Errorf("failed to write desktop file: %w", err)
	}

	return nil
}

func rewriteExecLine(execLine, execSrc string) string {
	value := strings.TrimPrefix(execLine, "Exec=")
	value = strings.TrimSpace(value)

	_, args := splitDesktopExec(value)
	return "Exec=" + quoteDesktopExecArg(execSrc) + args
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

func GetAppInfo(ctx context.Context, desktopSrc string) (*AppInfo, error) {
	if !util.HasExtension(desktopSrc, ".desktop") {
		return nil, fmt.Errorf("source file must be a .desktop file")
	}

	content, err := util.ReadFileContents(desktopSrc)
	if err != nil {
		return nil, err
	}

	appInfo := AppInfo{}
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
				appInfo.Version = value
			}
		}
	}

	if appInfo.Name == "" {
		appInfo.Name = strings.TrimSuffix(filepath.Base(desktopSrc), filepath.Ext(desktopSrc))
	}
	if appInfo.Version == "" {
		appInfo.Version = "n/a"
	}

	appInfo.ID = util.Slugify(appInfo.Name)

	return &appInfo, nil
}

func LocateDesktopFile(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("directory cannot be empty")
	}

	// Ensure directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", dir)
	}

	// Find all .desktop files
	desktopGlob, err := filepath.Glob(filepath.Join(dir, "*.desktop"))
	if err != nil {
		return "", fmt.Errorf("glob pattern error: %w", err)
	}

	if len(desktopGlob) == 0 {
		return "", fmt.Errorf("no .desktop file found in: %s", dir)
	}

	// If multiple files found, prefer one matching directory name
	if len(desktopGlob) > 1 {
		dirName := filepath.Base(dir)
		for _, candidate := range desktopGlob {
			candidateName := strings.TrimSuffix(filepath.Base(candidate), ".desktop")
			if candidateName == dirName {
				return candidate, nil
			}
		}
		// Also prefer "AppName.desktop" pattern
		for _, candidate := range desktopGlob {
			if strings.HasPrefix(filepath.Base(candidate), dirName) {
				return candidate, nil
			}
		}
	}

	return desktopGlob[0], nil
}

func LocateIcon(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("directory cannot be empty")
	}

	// Ensure directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", dir)
	}

	// Icon search order: SVG (vector, best quality) → PNG → ICO → XPM
	extensions := []string{".svg", ".png", ".ico", ".xpm"}

	var candidates []string
	for _, ext := range extensions {
		pattern := filepath.Join(dir, "*"+ext)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return "", fmt.Errorf("glob pattern error for %s: %w", ext, err)
		}
		candidates = append(candidates, matches...)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no icon file found in: %s", dir)
	}

	// Try all candidates, resolving symlinks
	var lastErr error
	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			lastErr = err
			continue
		}

		// Verify the resolved file exists
		if _, err := os.Stat(resolved); err == nil {
			return resolved, nil
		}
		lastErr = fmt.Errorf("icon target does not exist: %s", resolved)
	}

	if lastErr != nil {
		return "", fmt.Errorf("no valid icon found: %w", lastErr)
	}

	return "", fmt.Errorf("no icon found in: %s", dir)
}
