package core

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	util "github.com/slobbe/appimage-manager/internal/helpers"
)

type AppInfo struct {
	Name    string
	Version string
}

func ExtractAppImage(appImageSrc, outDir string) error {
	if err := util.MakeExecutable(appImageSrc); err != nil {
		return err
	}
	cmd := exec.Command(appImageSrc, "--appimage-extract")
	cmd.Dir = filepath.Dir(outDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract failed: %w\n%s", err, out)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(outDir), "squashfs-root"), outDir); err != nil {
		return err
	}

	return nil
}

func LocateDesktop(dir string) (string, error) {
	desktopGlob, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
	if len(desktopGlob) == 0 {
		return "", fmt.Errorf("no desktop file found inside AppImage")
	}
	return desktopGlob[0], nil
}

func LocateIcon(dir string) (string, error) {
	iconGlob, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	if len(iconGlob) == 0 {
		iconGlob, _ = filepath.Glob(filepath.Join(dir, "*.svg"))
	}

	if len(iconGlob) > 0 {
		iconSrc := iconGlob[0]
		real, err := filepath.EvalSymlinks(iconSrc)
		if err != nil {
			return "", err
		}

		return real, nil
	} else {
		return "", fmt.Errorf("no icon found inside AppImage")
	}
}

func UpdateDesktop(desktopSrc string, execCmd string, iconSrc string) error {
	content, err := os.ReadFile(desktopSrc)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "Exec=") {
			lines[i] = "Exec=" + execCmd
		}
		if strings.HasPrefix(l, "Icon=") {
			lines[i] = "Icon=" + iconSrc
		}
	}

	return os.WriteFile(desktopSrc, []byte(strings.Join(lines, "\n")), 0644)
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
