package core

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	util "github.com/slobbe/appimage-manager/internal/helpers"
)

type AppInfo struct {
	Name    string
	Version string
}

func ExtractAppImage(appImageSrc, outDir string) error {
	// Validate inputs
	if appImageSrc == "" {
		return fmt.Errorf("appimage source path cannot be empty")
	}
	if outDir == "" {
		return fmt.Errorf("output directory cannot be empty")
	}

	// Check source file exists and is accessible
	info, err := os.Stat(appImageSrc)
	if err != nil {
		return fmt.Errorf("failed to access source file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("source path is a directory, not a file: %s", appImageSrc)
	}

	// Ensure output directory parent exists
	outDirParent := filepath.Dir(outDir)
	if err := os.MkdirAll(outDirParent, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Remove existing output directory if present
	if _, err := os.Stat(outDir); err == nil {
		if err := os.RemoveAll(outDir); err != nil {
			return fmt.Errorf("failed to remove existing output directory: %w", err)
		}
	}

	// Ensure source file is executable
	if err := util.MakeExecutable(appImageSrc); err != nil {
		return fmt.Errorf("failed to make executable: %w", err)
	}

	// Try extraction with Type 2 syntax first (supports both types via --appimage-help)
	// Some AppImages use different extraction methods
	extractCmd := exec.Command(appImageSrc, "--appimage-extract")
	extractCmd.Dir = outDirParent

	out, err := extractCmd.CombinedOutput()
	if err != nil {
		// Try Type 1 syntax as fallback
		extractCmd = exec.Command(appImageSrc, "--appimage-extract-and-run")
		out, err = extractCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("extraction failed: %w\nOutput: %s", err, string(out))
		}
	}

	// Verify extraction created the expected directory
	squashfsPath := filepath.Join(outDirParent, "squashfs-root")
	if _, err := os.Stat(squashfsPath); err != nil {
		return fmt.Errorf("extraction verification failed: squashfs-root not found")
	}

	// Rename extraction directory to desired output path
	if err := os.Rename(squashfsPath, outDir); err != nil {
		// Clean up partial extraction on rename failure
		os.RemoveAll(squashfsPath)
		return fmt.Errorf("failed to rename extraction directory: %w", err)
	}

	return nil
}

func UpdateDesktopFile(desktopSrc string, execCmd string, iconSrc string) error {
	// Validate inputs
	if desktopSrc == "" {
		return fmt.Errorf("desktop file path cannot be empty")
	}
	if execCmd == "" {
		return fmt.Errorf("exec command cannot be empty")
	}
	if iconSrc == "" {
		return fmt.Errorf("icon path cannot be empty")
	}
	
	
	// Read file content
	content, err := os.ReadFile(desktopSrc)
	if err != nil {
		return fmt.Errorf("failed to read desktop file: %w", err)
	}
	
	// Verify it's a valid UTF-8 string
	if !utf8.Valid(content) {
		return fmt.Errorf("desktop file is not valid UTF-8")
	}
	
	lines := strings.Split(string(content), "\n")
	inDesktopEntryGroup := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Detect group headers
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntryGroup = trimmed == "[Desktop Entry]"
			continue
		}

		// Only modify lines within [Desktop Entry] group
		if !inDesktopEntryGroup {
			continue
		}
		
		// Handle Exec= lines - preserve arguments after command
		if strings.HasPrefix(trimmed, "Exec=") {
			// Extract existing arguments after executable (e.g., %f, %U, %c)
			existingArgs := ""
			if idx := strings.Index(trimmed, " "); idx != -1 {
				existingArgs = trimmed[idx:] // Keep space and arguments
			}
			lines[i] = "Exec=" + execCmd + existingArgs
		}
		
		// Handle Icon= lines - handle different value types per spec
		if strings.HasPrefix(trimmed, "Icon=") {
			lines[i] = "Icon=" + iconSrc
		}
	}
	
	// Ensure file ends with newline
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	
	// Write back with consistent permissions (preserve existing if possible)
	info, statErr := os.Stat(desktopSrc)
	var perm os.FileMode = 0o644
	if statErr == nil {
		perm = info.Mode().Perm() & 0o666 // Preserve permissions except executable bit
	}

	if err := os.WriteFile(desktopSrc, []byte(strings.Join(lines, "\n")), perm); err != nil {
		return fmt.Errorf("failed to write desktop file: %w", err)
	}

	return nil
}

func CreateDesktopFile(desktopSrc, name, execCmd, iconSrc, comment string) error {
	// Validate required fields
	if desktopSrc == "" || name == "" || execCmd == "" || iconSrc == "" {
		return fmt.Errorf("desktopSrc, name, execCmd, and iconSrc are required")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(desktopSrc), 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Build desktop file content
	var buf bytes.Buffer
	buf.WriteString("[Desktop Entry]\n")
	buf.WriteString("Type=Application\n")
	buf.WriteString("Name=" + name + "\n")
	buf.WriteString("Exec=" + execCmd + "\n")
	buf.WriteString("Icon=" + iconSrc + "\n")
	if comment != "" {
		buf.WriteString("Comment=" + comment + "\n")
	}
	buf.WriteString("Terminal=false\n")
	buf.WriteString("Categories=Utility;\n")

	if err := os.WriteFile(desktopSrc, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write desktop file: %w", err)
	}

	return nil
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


func ExtractAppInfo(desktopPath string) (*AppInfo, error) {
	file, err := os.Open(desktopPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	info := &AppInfo{}
	scanner := bufio.NewScanner(file)
	inDesktopEntry := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check if we entered [Desktop Entry] section
		if line == "[Desktop Entry]" {
			inDesktopEntry = true
			continue
		}

		// Stop if we hit another section
		if len(line) > 0 && line[0] == '[' && line != "[Desktop Entry]" {
			break
		}

		if !inDesktopEntry {
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Name":
			info.Name = value
		case "X-AppImage-Version":
			info.Version = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return info, nil
}
